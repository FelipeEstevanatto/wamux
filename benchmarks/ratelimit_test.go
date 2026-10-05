package benchmarks

import (
	"testing"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/httpguard"
)

// Scenario: the rate-limiter and queue substrate. apime can back both with
// in-process memory or Redis (selected by REDIS_ENABLED), so a fleet of API
// replicas can share the budget. WaMux is in-process only. These benchmarks
// measure the in-process cost against a real Redis round trip.
//
//	EVO_BENCH_REDIS_ADDR=127.0.0.1:6379 \
//	  go test -run=^$ -bench='Limiter|Queue' ./benchmarks/ -benchtime=2000x

const benchLimiterKey = "bench-instance"

func BenchmarkLimiterMemory(b *testing.B) {
	l := httpguard.NewLimiter(1_000_000, time.Minute)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if ok, _ := l.Allow(benchLimiterKey); !ok {
			b.Fatal("memory limiter denied")
		}
	}
}

func BenchmarkLimiterRedis(b *testing.B) {
	rc := dialBenchRedis(b)
	defer rc.close()
	rc.del(benchLimiterKey)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if ok, err := rc.incrAllow(benchLimiterKey, 1_000_000, time.Minute); err != nil || !ok {
			b.Fatalf("redis limiter: ok=%v err=%v", ok, err)
		}
	}
}

func BenchmarkQueueMemoryEnqueue(b *testing.B) {
	q := make(chan string, b.N+1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q <- "job"
	}
}

func BenchmarkQueueRedisEnqueue(b *testing.B) {
	rc := dialBenchRedis(b)
	defer rc.close()
	const key = "bench-queue"
	rc.del(key)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := rc.rpush(key, "job"); err != nil {
			b.Fatal(err)
		}
	}
}

// TestLimiterSemantics validates the fixed-window behaviour of both limiters:
// the first `limit` requests pass, the next is denied.
func TestLimiterSemantics(t *testing.T) {
	l := httpguard.NewLimiter(2, time.Minute)
	for i := 0; i < 2; i++ {
		if ok, _ := l.Allow("k"); !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if ok, _ := l.Allow("k"); ok {
		t.Fatal("third request should be denied")
	}

	rc := dialBenchRedis(t) // skips without EVO_BENCH_REDIS_ADDR
	defer rc.close()
	key := "bench-limiter-semantics"
	rc.del(key)
	for i := 0; i < 2; i++ {
		if ok, err := rc.incrAllow(key, 2, time.Minute); err != nil || !ok {
			t.Fatalf("redis request %d: ok=%v err=%v", i+1, ok, err)
		}
	}
	if ok, _ := rc.incrAllow(key, 2, time.Minute); ok {
		t.Fatal("redis third request should be denied")
	}
}

// TestQueueSemantics validates FIFO order for the Redis queue.
func TestQueueSemantics(t *testing.T) {
	rc := dialBenchRedis(t)
	defer rc.close()
	const key = "bench-queue-semantics"
	rc.del(key)
	for _, v := range []string{"a", "b", "c"} {
		if err := rc.rpush(key, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []string{"a", "b", "c"} {
		got, ok, err := rc.lpop(key)
		if err != nil || !ok || got != want {
			t.Fatalf("lpop: got %q ok=%v err=%v, want %q", got, ok, err, want)
		}
	}
}
