package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	config "github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	instance_service "github.com/felipeestevanatto/wamux/pkg/instance/service"
	auth_middleware "github.com/felipeestevanatto/wamux/pkg/middleware"
	"github.com/gin-gonic/gin"
)

// stubInstanceService resolves exactly one token, so the auth middleware can be
// exercised in both the allowed and rejected cases. The embedded interface
// satisfies the rest of InstanceService.
type stubInstanceService struct {
	instance_service.InstanceService
	valid string
}

func (s *stubInstanceService) GetInstanceByToken(token string) (*instance_model.Instance, error) {
	if token == s.valid {
		return &instance_model.Instance{Id: "inst-1", Token: token}, nil
	}
	return nil, errorInvalidToken
}

var errorInvalidToken = errorString("invalid token")

type errorString string

func (e errorString) Error() string { return string(e) }

func testConfig() *config.Config {
	return &config.Config{
		GlobalApiKey:       "admin-secret",
		SwaggerEnabled:     false,
		RateLimitPerMinute: 0, // disable the limiter so assertions are about routing, not volume
	}
}

// routeTable returns the set of "METHOD /path" strings the engine exposes.
func routeTable(t *testing.T, eng *gin.Engine) map[string]struct{} {
	t.Helper()
	out := make(map[string]struct{})
	for _, r := range eng.Routes() {
		out[r.Method+" "+r.Path] = struct{}{}
	}
	return out
}

func TestAssignRoutesRegistersEveryEndpointGroup(t *testing.T) {
	cfg := testConfig()
	eng := NewTestRouter(cfg, auth_middleware.NewMiddleware(cfg, &stubInstanceService{valid: "instance-token"}))

	table := routeTable(t, eng)

	// Every group must have at least this canonical route. A group that fails to
	// mount (a typo in the path, a handler left off) shows up here.
	want := []string{
		"GET /",
		"GET /metrics",
		"GET /server/health",
		"GET /server/ok",
		"GET /server/stats",
		"POST /instance/create",
		"GET /instance/all",
		"POST /instance/connect",
		"GET /instance/qr",
		"POST /send/text",
		"POST /send/media",
		"POST /send/status/text",
		"POST /user/info",
		"GET /user/contacts",
		"POST /message/react",
		"POST /message/markread",
		"POST /chat/pin",
		"GET /chat/history",
		"GET /chat/history/search",
		"GET /chat/chats",
		"GET /chat/contacts",
		"GET /chat/senders",
		"GET /chat/media/:messageId",
		"GET /group/list",
		"POST /group/create",
		"POST /call/reject",
		"POST /community/create",
		"POST /label/chat",
		"POST /unlabel/chat",
		"POST /newsletter/create",
		"GET /polls/:pollMessageId/results",
		"POST /typebot",
		"GET /typebot/sessions",
	}
	for _, want_ := range want {
		if _, ok := table[want_]; !ok {
			t.Errorf("route not registered: %s", want_)
		}
	}
}

// Swagger is served only when enabled; the default (here, disabled) must not
// leak the public docs route.
func TestAssignRoutesSwaggerToggle(t *testing.T) {
	cfg := testConfig()
	mw := auth_middleware.NewMiddleware(cfg, &stubInstanceService{valid: "t"})

	off := NewTestRouter(cfg, mw)
	if _, ok := routeTable(t, off)["GET /swagger/*any"]; ok {
		t.Error("swagger route registered although SwaggerEnabled=false")
	}

	cfg.SwaggerEnabled = true
	on := NewTestRouter(cfg, mw)
	if _, ok := routeTable(t, on)["GET /swagger/*any"]; !ok {
		t.Error("swagger route missing although SwaggerEnabled=true")
	}
}

// A protected route must reject a missing or wrong apikey with 401, and the
// admin route must reject an instance token (only the global key is accepted).
func TestAssignRoutesAuthBoundaries(t *testing.T) {
	cfg := testConfig()
	mw := auth_middleware.NewMiddleware(cfg, &stubInstanceService{valid: "instance-token"})
	eng := NewTestRouter(cfg, mw)

	// Admin route: no key -> 401, instance key -> 401 (reaches no handler, but
	// the middleware short-circuits before the nil stub would be called).
	for _, key := range []string{"", "instance-token"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/instance/all", nil)
		if key != "" {
			req.Header.Set("apikey", key)
		}
		eng.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("GET /instance/all with apikey=%q: status = %d, want 401", key, w.Code)
		}
	}

	// Instance route: no key -> 401.
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/send/text", strings.NewReader("{}")))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("POST /send/text without key: status = %d, want 401", w.Code)
	}
}

// A request to an unknown path is a NestJS-style JSON 404 with the
// "Cannot METHOD /path" message, not Gin's plain-text "404 page not found".
func TestAssignRoutesUnknownPathIs404(t *testing.T) {
	cfg := testConfig()
	eng := NewTestRouter(cfg, auth_middleware.NewMiddleware(cfg, &stubInstanceService{valid: "t"}))

	for _, tc := range []struct {
		method, path, wantMsg string
	}{
		{http.MethodGet, "/sdfsdf", "Cannot GET /sdfsdf"},
		{http.MethodPost, "/does-not-exist", "Cannot POST /does-not-exist"},
		{http.MethodGet, "/login", "Cannot GET /login"},
	} {
		w := httptest.NewRecorder()
		eng.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s %s: status = %d, want 404", tc.method, tc.path, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("%s %s: Content-Type = %q, want application/json", tc.method, tc.path, ct)
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s %s: body is not JSON: %v", tc.method, tc.path, err)
		}
		if body["status"] != float64(404) || body["error"] != "Not Found" {
			t.Errorf("%s %s: body = %v, want status 404 / error Not Found", tc.method, tc.path, body)
		}
		resp, _ := body["response"].(map[string]any)
		msgs, _ := resp["message"].([]any)
		if len(msgs) != 1 || msgs[0] != tc.wantMsg {
			t.Errorf("%s %s: response.message = %v, want [%q]", tc.method, tc.path, msgs, tc.wantMsg)
		}
	}
}

// HTML pages must be revalidated on every load, or a deploy stays invisible until
// the browser's guessed lifetime runs out.
func TestAssignRoutesHTMLPagesAreRevalidated(t *testing.T) {
	// The pages must exist: Go's 404 response strips Cache-Control.
	dir := t.TempDir()
	dist := filepath.Join(dir, "manager", "dist")
	if err := os.MkdirAll(dist, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<html></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	cfg := testConfig()
	eng := NewTestRouter(cfg, auth_middleware.NewMiddleware(cfg, &stubInstanceService{valid: "t"}))

	for _, path := range []string{"/manager", "/manager/messages"} {
		w := httptest.NewRecorder()
		eng.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200", path, w.Code)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("GET %s: Cache-Control = %q, want no-cache", path, got)
		}
	}
}

// The dashboard moved into the manager, and old links must not break.
func TestAssignRoutesDashboardRedirectsToTheManager(t *testing.T) {
	cfg := testConfig()
	eng := NewTestRouter(cfg, auth_middleware.NewMiddleware(cfg, &stubInstanceService{valid: "t"}))

	for _, path := range []string{"/dashboard", "/dashboard?embed=1&theme=dark"} {
		w := httptest.NewRecorder()
		eng.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusFound || w.Header().Get("Location") != "/manager" {
			t.Errorf("GET %s: %d to %q", path, w.Code, w.Header().Get("Location"))
		}
	}
}

// no-cache must keep conditional requests working, so an unchanged page costs a
// 304 and not the whole file.
func TestServePageAnswersConditionalRequests(t *testing.T) {
	file := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(file, []byte("<html></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}

	eng := gin.New()
	eng.GET("/page", servePage(file))

	req := httptest.NewRequest(http.MethodGet, "/page", nil)
	req.Header.Set("If-Modified-Since", info.ModTime().UTC().Add(time.Second).Format(http.TimeFormat))
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)
	if w.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", got)
	}
}
