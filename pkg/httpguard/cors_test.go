package httpguard

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func corsTestRouter(allowed []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORSMiddleware(allowed))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })
	return r
}

func doCORS(r *gin.Engine, origin, method string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/x", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestCORSExactOriginEchoedWithCredentials(t *testing.T) {
	r := corsTestRouter([]string{"https://app.example.com"})
	w := doCORS(r, "https://app.example.com", http.MethodGet)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("allow-origin = %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("credentials = %q", got)
	}
}

func TestCORSDisallowedOriginGetsNoHeaders(t *testing.T) {
	r := corsTestRouter([]string{"https://app.example.com"})
	w := doCORS(r, "https://evil.example.com", http.MethodGet)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("disallowed origin should get no allow-origin, got %q", got)
	}
}

func TestCORSEmptyAllowlistIsSameOriginOnly(t *testing.T) {
	r := corsTestRouter(nil)
	w := doCORS(r, "https://app.example.com", http.MethodGet)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("empty allowlist should not allow any cross-origin, got %q", got)
	}
}

func TestCORSWildcardNeverAllowsCredentials(t *testing.T) {
	r := corsTestRouter([]string{"*"})
	w := doCORS(r, "https://anything.example.com", http.MethodGet)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow-origin = %q, want *", got)
	}
	// The whole point: `*` + credentials is spec-invalid, so credentials must NOT
	// be advertised when reflecting a wildcard.
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("wildcard must not allow credentials, got %q", got)
	}
}

func TestCORSSubdomainWildcard(t *testing.T) {
	r := corsTestRouter([]string{"https://*.example.com"})

	if got := doCORS(r, "https://a.example.com", http.MethodGet).Header().Get("Access-Control-Allow-Origin"); got != "https://a.example.com" {
		t.Fatalf("subdomain should match, got %q", got)
	}
	if got := doCORS(r, "https://other.test", http.MethodGet).Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unrelated host should not match, got %q", got)
	}
}

func TestCORSPreflightAborts204(t *testing.T) {
	r := corsTestRouter([]string{"https://app.example.com"})
	w := doCORS(r, "https://app.example.com", http.MethodOptions)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight = %d, want 204", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatalf("preflight missing allow-methods")
	}
}
