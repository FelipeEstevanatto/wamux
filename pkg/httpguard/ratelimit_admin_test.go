package httpguard

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// A health/scrape route must never be rate limited, however hard it is polled —
// otherwise a load balancer or Prometheus burst would make the service look
// "down" exactly when it is busiest.
func TestHealthRoutesAreExempt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	l := NewLimiter(1, time.Minute)
	l.nowFunc = func() time.Time { return time.Unix(0, 0) }

	r := gin.New()
	r.Use(MiddlewareWithAdmin(l, nil, "admin-key"))
	r.GET("/server/health", func(c *gin.Context) { c.Status(200) })

	for i := 0; i < 50; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/server/health", nil))
		if w.Code != 200 {
			t.Fatalf("health request %d = %d, want 200", i, w.Code)
		}
	}
}

// The admin key is one operator, not a tenant: a burst of admin traffic must not
// lock the operator out. It gets a separate, more generous bucket.
func TestAdminKeyHasSeparateBucket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	l := NewLimiter(2, time.Minute)
	l.nowFunc = func() time.Time { return time.Unix(0, 0) }

	r := gin.New()
	r.Use(MiddlewareWithAdmin(l, nil, "admin-key"))
	r.GET("/instance/all", func(c *gin.Context) { c.Status(200) })

	// Spend the instance-limit budget on the admin key.
	do := func() int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/instance/all", nil)
		req.Header.Set("apikey", "admin-key")
		r.ServeHTTP(w, req)
		return w.Code
	}

	// Admin limit is 10x the base (2 -> 20), so well over 2 requests succeed.
	for i := 0; i < 15; i++ {
		if code := do(); code != 200 {
			t.Fatalf("admin request %d = %d, want 200 (separate, generous bucket)", i, code)
		}
	}
}

// A tenant token is still limited by the normal bucket, and is independent of
// the admin bucket.
func TestTenantStillLimitedAndIndependentOfAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	l := NewLimiter(2, time.Minute)
	l.nowFunc = func() time.Time { return time.Unix(0, 0) }

	r := gin.New()
	r.Use(MiddlewareWithAdmin(l, nil, "admin-key"))
	r.GET("/send/text", func(c *gin.Context) { c.Status(200) })

	call := func(token string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/send/text", nil)
		req.Header.Set("apikey", token)
		r.ServeHTTP(w, req)
		return w.Code
	}

	if call("tenant-a") != 200 || call("tenant-a") != 200 {
		t.Fatalf("tenant-a first two should pass")
	}
	if call("tenant-a") != http.StatusTooManyRequests {
		t.Fatalf("tenant-a third should be limited")
	}
	// The admin bucket is untouched by tenant-a's exhaustion.
	if call("admin-key") != 200 {
		t.Fatalf("admin should be unaffected by tenant-a")
	}
}

func TestExplicitAdminLimiterRespected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	l := NewLimiter(100, time.Minute)
	admin := NewLimiter(1, time.Minute)
	l.nowFunc = func() time.Time { return time.Unix(0, 0) }
	admin.nowFunc = l.nowFunc

	r := gin.New()
	r.Use(MiddlewareWithAdmin(l, admin, "admin-key"))
	r.GET("/instance/all", func(c *gin.Context) { c.Status(200) })

	call := func() int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/instance/all", nil)
		req.Header.Set("apikey", "admin-key")
		r.ServeHTTP(w, req)
		return w.Code
	}
	if call() != 200 {
		t.Fatalf("first admin call should pass")
	}
	if call() != http.StatusTooManyRequests {
		t.Fatalf("second admin call should hit the explicit admin limiter")
	}
}
