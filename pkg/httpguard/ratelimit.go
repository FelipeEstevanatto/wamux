// Package httpguard holds HTTP-layer abuse protection: a per-client token-bucket
// rate limiter and the CORS configuration.
//
// # WHY THIS EXISTS
//
// Every route was reachable at an unlimited rate. A single client could brute
// force the `apikey` header, or a runaway integration could hammer POST /send/*
// and starve every other tenant on the box (shared DB pool, shared WhatsApp
// sockets). There was also no way to bound it: the auth middleware only checks
// the key, never how often it is presented.
//
// The limiter here is intentionally dependency-free (no golang.org/x/time) so it
// stays easy to reason about and test.
package httpguard

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Limiter is a fixed-window counter keyed by client identity.
//
// A token bucket is smoother, but a fixed window is enough to stop brute force
// and floods, and it is trivial to verify: a key gets at most `limit` requests
// per `window`, then 429s until the window rolls over.
type Limiter struct {
	mu      sync.Mutex
	hits    map[string]*bucket
	limit   int
	window  time.Duration
	nowFunc func() time.Time // injectable for tests
}

type bucket struct {
	count int
	start time.Time
}

// NewLimiter builds a limiter allowing `limit` requests per `window`. A
// non-positive limit disables limiting (NewLimiter returns nil), so callers can
// treat "nil limiter" as "no limiting".
func NewLimiter(limit int, window time.Duration) *Limiter {
	if limit <= 0 || window <= 0 {
		return nil
	}
	return &Limiter{
		hits:    make(map[string]*bucket),
		limit:   limit,
		window:  window,
		nowFunc: time.Now,
	}
}

// Allow reports whether the key may proceed, and how long to wait when it may
// not. It is safe for concurrent use.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.nowFunc()
	w, ok := l.hits[key]
	if !ok || now.Sub(w.start) >= l.window {
		l.hits[key] = &bucket{count: 1, start: now}
		l.sweepLocked(now)
		return true, 0
	}

	if w.count >= l.limit {
		return false, l.window - now.Sub(w.start)
	}
	w.count++
	return true, 0
}

// sweepLocked drops windows that have expired. It runs on each new window so the
// map cannot grow without bound as new clients appear; the cost is amortised
// because it only touches entries while iterating for a fresh key.
func (l *Limiter) sweepLocked(now time.Time) {
	for k, w := range l.hits {
		if now.Sub(w.start) >= l.window {
			delete(l.hits, k)
		}
	}
}

// ClientKey identifies the caller for limiting: the instance token when present,
// else the admin/global key, else the client IP. Keying by credential means two
// tenants behind one NAT do not throttle each other, while an unauthenticated
// flood still collapses to its source IP.
func ClientKey(c *gin.Context) string {
	if token := c.GetHeader("apikey"); token != "" {
		return token
	}
	return c.ClientIP()
}

// Middleware returns a Gin middleware enforcing the limiter. A nil limiter is a
// no-op passthrough. The response is a 429 with a Retry-After header so a
// well-behaved client backs off instead of hammering.
func Middleware(l *Limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if l == nil {
			c.Next()
			return
		}

		ok, retryAfter := l.Allow(ClientKey(c))
		if !ok {
			seconds := int(retryAfter.Seconds())
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", itoa(seconds))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
			})
			return
		}
		c.Next()
	}
}

// itoa avoids pulling strconv into the hot path signature; small and clear.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
