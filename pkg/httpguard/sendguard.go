package httpguard

import (
	"net/http"
	"sync"
	"time"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

// SendGuard bounds a single instance's send traffic so one tenant cannot starve
// the others.
//
// # WHY
//
// /send/* reaches WhatsApp through shared machinery: one process-wide
// persistence pool, one HTTP worker set, and — while sending — the instance's own
// socket. A single tenant looping POST /send/text or firing hundreds of parallel
// media uploads consumes all of it, and every other tenant on the box slows to a
// crawl. There was no per-instance ceiling of any kind.
//
// Two independent limits, because they stop different failure modes:
//   - a rate limit (requests/minute) stops a sustained flood;
//   - a concurrency cap (in-flight) stops a burst of slow requests from
//     occupying every worker, which a per-minute limit alone does not.
//
// Both are keyed by the instance id taken from the authenticated request, so the
// budget is per tenant, not per IP (many tenants share a NAT).
type SendGuard struct {
	limiter   *Limiter
	mu        sync.Mutex
	inFlight  map[string]int
	maxPerIns int
}

// NewSendGuard builds a guard. perMinute<=0 disables the rate limit;
// maxConcurrent<=0 disables the concurrency cap. A guard with both disabled is
// still returned so callers do not branch.
func NewSendGuard(perMinute, maxConcurrent int) *SendGuard {
	return &SendGuard{
		limiter:   NewLimiter(perMinute, time.Minute),
		inFlight:  make(map[string]int),
		maxPerIns: maxConcurrent,
	}
}

// enabled reports whether either limit is active.
func (g *SendGuard) enabled() bool {
	return g != nil && (g.limiter != nil || g.maxPerIns > 0)
}

// Middleware enforces the guard. It must run AFTER authentication, because it
// keys on the authenticated instance.
func (g *SendGuard) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !g.enabled() {
			c.Next()
			return
		}

		instanceID := instanceIDFromContext(c)
		if instanceID == "" {
			// No authenticated instance: nothing to scope to, let auth handle it.
			c.Next()
			return
		}

		// Rate limit first: the cheaper check, and the one that sheds load.
		if g.limiter != nil {
			if ok, retryAfter := g.limiter.Allow(instanceID); !ok {
				seconds := int(retryAfter.Seconds())
				if seconds < 1 {
					seconds = 1
				}
				c.Header("Retry-After", itoa(seconds))
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error": "send rate limit exceeded for this instance",
				})
				return
			}
		}

		// Concurrency cap.
		if g.maxPerIns > 0 {
			if !g.acquire(instanceID) {
				c.Header("Retry-After", "1")
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error": "too many concurrent sends for this instance",
				})
				return
			}
			defer g.release(instanceID)
		}

		c.Next()
	}
}

// acquire reserves one concurrency slot, reporting false when at the cap.
func (g *SendGuard) acquire(instanceID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight[instanceID] >= g.maxPerIns {
		return false
	}
	g.inFlight[instanceID]++
	return true
}

func (g *SendGuard) release(instanceID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight[instanceID] > 0 {
		g.inFlight[instanceID]--
	}
	if g.inFlight[instanceID] == 0 {
		delete(g.inFlight, instanceID)
	}
}

// InFlight reports the current in-flight send count for an instance (diagnostics).
func (g *SendGuard) InFlight(instanceID string) int {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.inFlight[instanceID]
}

// instanceIDFromContext reads the instance set by the auth middleware.
func instanceIDFromContext(c *gin.Context) string {
	v, ok := c.Get("instance")
	if !ok {
		return ""
	}
	if inst, ok := v.(*instance_model.Instance); ok && inst != nil {
		return inst.Id
	}
	return ""
}
