package httpguard

import (
	"net/http"
	"net/http/httptest"
	"testing"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

// sendRouter mounts the guard behind a fake auth that sets the instance.
func sendRouter(g *SendGuard, instanceID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("instance", &instance_model.Instance{Id: instanceID})
		c.Next()
	})
	r.Use(g.Middleware())
	r.POST("/send/text", func(c *gin.Context) { c.Status(200) })
	return r
}

func TestSendGuardConcurrencyCap(t *testing.T) {
	g := NewSendGuard(0, 1) // no rate limit, max 1 concurrent
	r := sendRouter(g, "inst-a")

	// First request enters and blocks inside the handler; the second must be
	// rejected with 429 before it reaches the handler.
	release := make(chan struct{})
	entered := make(chan struct{})
	r2 := gin.New()
	gin.SetMode(gin.TestMode)
	r2.Use(func(c *gin.Context) { c.Set("instance", &instance_model.Instance{Id: "inst-a"}); c.Next() })
	r2.Use(g.Middleware())
	r2.POST("/send/text", func(c *gin.Context) {
		close(entered)
		<-release
		c.Status(200)
	})

	go func() {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/send/text", nil)
		r2.ServeHTTP(w, req)
	}()

	<-entered // first is in flight

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/send/text", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second concurrent send = %d, want 429", w.Code)
	}

	close(release)
}

func TestSendGuardRateLimitPerInstance(t *testing.T) {
	g := NewSendGuard(2, 0) // 2/min, no concurrency cap
	r := sendRouter(g, "inst-b")

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/send/text", nil))
		if w.Code != 200 {
			t.Fatalf("request %d = %d, want 200", i+1, w.Code)
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/send/text", nil))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("third request = %d, want 429", w.Code)
	}
}

func TestSendGuardIsolatesInstances(t *testing.T) {
	g := NewSendGuard(1, 0)
	ra := sendRouter(g, "tenant-a")
	rb := sendRouter(g, "tenant-b")

	// tenant-a spends its budget.
	ra.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/send/text", nil))
	if w := httptest.NewRecorder(); func() int { ra.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/send/text", nil)); return w.Code }() != http.StatusTooManyRequests {
		t.Fatalf("tenant-a second request should be limited")
	}

	// tenant-b is unaffected.
	w := httptest.NewRecorder()
	rb.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/send/text", nil))
	if w.Code != 200 {
		t.Fatalf("tenant-b should not be throttled by tenant-a, got %d", w.Code)
	}
}

func TestSendGuardDisabled(t *testing.T) {
	g := NewSendGuard(0, 0)
	// A disabled guard is a no-op passthrough even under a flood.
	r := sendRouter(g, "inst-c")
	for i := 0; i < 100; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/send/text", nil))
		if w.Code != 200 {
			t.Fatalf("disabled guard blocked request %d (%d)", i, w.Code)
		}
	}
}
