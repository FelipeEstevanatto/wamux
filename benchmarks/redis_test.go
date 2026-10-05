package benchmarks

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Minimal RESP client. apime can back its rate limiter, queue and (optionally)
// idempotency store with Redis; WaMux does not use Redis at all. Rather than add
// go-redis to WaMux's production dependency graph for a benchmark, this speaks
// just enough RESP to compare in-process state against a real Redis round trip.
//
// Gated on EVO_BENCH_REDIS_ADDR so CI without Redis still passes.

type redisConn struct {
	c net.Conn
	r *bufio.Reader
}

func dialBenchRedis(tb testing.TB) *redisConn {
	tb.Helper()
	addr := strings.TrimSpace(os.Getenv("EVO_BENCH_REDIS_ADDR"))
	if addr == "" {
		tb.Skip("set EVO_BENCH_REDIS_ADDR (host:port) to run the Redis comparisons")
	}
	c, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		tb.Fatalf("dial redis %s: %v", addr, err)
	}
	return &redisConn{c: c, r: bufio.NewReader(c)}
}

func (rc *redisConn) close() { _ = rc.c.Close() }

func (rc *redisConn) cmd(args ...string) (interface{}, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := io.WriteString(rc.c, b.String()); err != nil {
		return nil, err
	}
	return rc.readReply()
}

func (rc *redisConn) readReply() (interface{}, error) {
	line, err := rc.r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return nil, fmt.Errorf("redis: empty reply")
	}
	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return nil, fmt.Errorf("redis: %s", line[1:])
	case ':':
		return strconv.ParseInt(line[1:], 10, 64)
	case '$':
		n, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, nil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(rc.r, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	default:
		return nil, fmt.Errorf("redis: unexpected reply %q", line)
	}
}

// incrAllow is a fixed-window limiter implemented the way a Redis-backed one is:
// INCR then PEXPIRE on first hit.
func (rc *redisConn) incrAllow(key string, limit int, window time.Duration) (bool, error) {
	n, err := rc.cmd("INCR", key)
	if err != nil {
		return false, err
	}
	count, ok := n.(int64)
	if !ok {
		return false, fmt.Errorf("redis: INCR returned %T", n)
	}
	if count == 1 {
		if _, err := rc.cmd("PEXPIRE", key, strconv.FormatInt(window.Milliseconds(), 10)); err != nil {
			return false, err
		}
	}
	return count <= int64(limit), nil
}

// setNX stores a value only if absent, with a TTL (idempotency first-call).
func (rc *redisConn) setNX(key, val string, ttl time.Duration) (bool, error) {
	r, err := rc.cmd("SET", key, val, "NX", "PX", strconv.FormatInt(ttl.Milliseconds(), 10))
	if err != nil {
		return false, err
	}
	return r == "OK", nil
}

func (rc *redisConn) get(key string) (string, bool, error) {
	r, err := rc.cmd("GET", key)
	if err != nil {
		return "", false, err
	}
	if r == nil {
		return "", false, nil
	}
	s, ok := r.(string)
	return s, ok, nil
}

func (rc *redisConn) rpush(key, val string) error {
	_, err := rc.cmd("RPUSH", key, val)
	return err
}

func (rc *redisConn) lpop(key string) (string, bool, error) {
	r, err := rc.cmd("LPOP", key)
	if err != nil {
		return "", false, err
	}
	if r == nil {
		return "", false, nil
	}
	s, ok := r.(string)
	return s, ok, nil
}

func (rc *redisConn) del(keys ...string) {
	_, _ = rc.cmd(append([]string{"DEL"}, keys...)...)
}
