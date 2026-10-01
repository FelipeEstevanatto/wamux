package httpguard

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLimiterAllowsUpToLimitThenBlocks(t *testing.T) {
	l := NewLimiter(3, time.Minute)
	base := time.Unix(0, 0)
	l.nowFunc = func() time.Time { return base }

	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("k"); !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	ok, retry := l.Allow("k")
	if ok {
		t.Fatalf("4th request should be blocked")
	}
	if retry <= 0 || retry > time.Minute {
		t.Fatalf("retry-after = %v, want within (0, 1m]", retry)
	}
}

func TestLimiterWindowResets(t *testing.T) {
	l := NewLimiter(1, time.Minute)
	now := time.Unix(0, 0)
	l.nowFunc = func() time.Time { return now }

	if ok, _ := l.Allow("k"); !ok {
		t.Fatalf("first should pass")
	}
	if ok, _ := l.Allow("k"); ok {
		t.Fatalf("second should be blocked")
	}

	now = now.Add(time.Minute)
	if ok, _ := l.Allow("k"); !ok {
		t.Fatalf("after the window it should pass again")
	}
}

func TestLimiterKeysAreIndependent(t *testing.T) {
	l := NewLimiter(1, time.Minute)
	l.nowFunc = func() time.Time { return time.Unix(0, 0) }

	if ok, _ := l.Allow("a"); !ok {
		t.Fatalf("a first should pass")
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatalf("b should not be throttled by a")
	}
}

func TestNilLimiterAllowsEverything(t *testing.T) {
	l := NewLimiter(0, time.Minute)
	if l != nil {
		t.Fatalf("limit 0 should yield a nil limiter")
	}
	if ok, _ := l.Allow("k"); !ok {
		t.Fatalf("nil limiter should allow")
	}
}

func TestMiddlewareReturns429(t *testing.T) {
	gin.SetMode(gin.TestMode)
	l := NewLimiter(1, time.Minute)
	l.nowFunc = func() time.Time { return time.Unix(0, 0) }

	r := gin.New()
	r.Use(Middleware(l))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	do := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("apikey", "tok")
		r.ServeHTTP(w, req)
		return w
	}

	if w := do(); w.Code != 200 {
		t.Fatalf("first = %d, want 200", w.Code)
	}
	w := do()
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatalf("429 should carry Retry-After")
	}
}

func TestClientKeyPrefersToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("apikey", "abc")
	if got := ClientKey(c); got != "abc" {
		t.Fatalf("ClientKey = %q, want the token", got)
	}
}
