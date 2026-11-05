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

// authFailingRouter answers 401 unless the apikey is "good".
func authFailingRouter(l *Limiter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(MiddlewareWithAdmin(l, nil, "admin-key"))
	r.GET("/x", func(c *gin.Context) {
		if key := c.GetHeader("apikey"); key != "good" && key != "admin-key" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Status(http.StatusOK)
	})
	return r
}

func requestWithKey(r *gin.Engine, key string) int {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	req.Header.Set("apikey", key)
	r.ServeHTTP(w, req)
	return w.Code
}

// Every guess is a new key with a fresh budget, so the per-key limit alone never
// stopped guessing: 10,000 guesses used to get through with no 429 at all.
func TestRotatingKeysAreStoppedPerIP(t *testing.T) {
	r := authFailingRouter(NewLimiter(600, time.Minute))

	reached := 0
	for i := 0; i < 500; i++ {
		switch requestWithKey(r, "guess-"+itoa(i+1)) {
		case http.StatusUnauthorized:
			reached++
		case http.StatusTooManyRequests:
		default:
			t.Fatal("unexpected status")
		}
	}
	if reached != authFailureLimit {
		t.Fatalf("%d guesses reached auth, want %d", reached, authFailureLimit)
	}
}

// Blocked guesses must not grow the limiter either.
func TestBlockedGuessesDoNotCreateBuckets(t *testing.T) {
	l := NewLimiter(600, time.Minute)
	r := authFailingRouter(l)
	for i := 0; i < 500; i++ {
		requestWithKey(r, "guess-"+itoa(i+1))
	}
	if n := len(l.hits); n != authFailureLimit {
		t.Fatalf("limiter holds %d keys, want %d", n, authFailureLimit)
	}
}

// A tenant already working from this IP keeps working while a neighbour on the
// same NAT guesses.
func TestWorkingKeySurvivesNeighbourGuessing(t *testing.T) {
	r := authFailingRouter(NewLimiter(600, time.Minute))

	if code := requestWithKey(r, "good"); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	for i := 0; i < 100; i++ {
		requestWithKey(r, "guess-"+itoa(i+1))
	}
	if code := requestWithKey(r, "good"); code != http.StatusOK {
		t.Fatalf("working key got %d during a neighbour's guessing", code)
	}
}

func TestAdminKeyIsNeverBlockedByFailures(t *testing.T) {
	r := authFailingRouter(NewLimiter(600, time.Minute))
	for i := 0; i < 100; i++ {
		requestWithKey(r, "guess-"+itoa(i+1))
	}
	if code := requestWithKey(r, "admin-key"); code != http.StatusOK {
		t.Fatalf("admin key got %d", code)
	}
}

// The block lifts with the window.
func TestAuthFailureBlockExpires(t *testing.T) {
	failures := NewLimiter(1, time.Minute)
	now := time.Now()
	failures.nowFunc = func() time.Time { return now }

	failures.Allow("ip")
	if blocked, _ := failures.exhausted("ip"); !blocked {
		t.Fatal("expected blocked")
	}
	now = now.Add(time.Minute)
	if blocked, _ := failures.exhausted("ip"); blocked {
		t.Fatal("expected the block to lift")
	}
}

// New keys must not scan the map: it holds the lock every request needs, and that
// scan is what took legitimate p50 from 0.33 ms to 100 ms under junk keys. Expired
// windows are still dropped once per window, so the map stays bounded.
func TestSweepRunsAtMostOncePerWindow(t *testing.T) {
	l := NewLimiter(10, time.Minute)
	now := time.Now()
	l.nowFunc = func() time.Time { return now }

	l.Allow("first") // t=0: sweeps an empty map and starts the clock
	now = now.Add(2 * time.Second)
	l.Allow("old") // t=2
	now = now.Add(59 * time.Second)
	l.Allow("trigger") // t=61: a sweep is due, and only "first" has expired
	if _, ok := l.hits["first"]; ok {
		t.Fatal("expired key survived a due sweep")
	}
	if _, ok := l.hits["old"]; !ok {
		t.Fatal("swept a key that had not expired")
	}

	now = now.Add(2 * time.Second)
	l.Allow("no-sweep") // t=63: "old" has expired, but the last sweep was 2s ago
	if _, ok := l.hits["old"]; !ok {
		t.Fatal("swept again inside the same window")
	}

	now = now.Add(time.Minute)
	l.Allow("later")
	if _, ok := l.hits["old"]; ok {
		t.Fatal("expired key survived the next due sweep")
	}
}
