package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

// A request to an unknown path is a 404, proving the table is exact.
func TestAssignRoutesUnknownPathIs404(t *testing.T) {
	cfg := testConfig()
	eng := NewTestRouter(cfg, auth_middleware.NewMiddleware(cfg, &stubInstanceService{valid: "t"}))

	w := httptest.NewRecorder()
	eng.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/does-not-exist", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
