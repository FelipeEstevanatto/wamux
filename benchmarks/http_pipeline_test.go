package benchmarks

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/config"
	"github.com/felipeestevanatto/wamux/pkg/httpguard"
	instance_handler "github.com/felipeestevanatto/wamux/pkg/instance/handler"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	auth_middleware "github.com/felipeestevanatto/wamux/pkg/middleware"
	send_handler "github.com/felipeestevanatto/wamux/pkg/sendMessage/handler"
	"github.com/gin-gonic/gin"
)

// Scenario: the full HTTP request pipeline for the hot endpoints, wired with the
// REAL middleware and handlers (auth, JID validation, per-instance send guard,
// JSON bind, response encode) and only the outbound WhatsApp call stubbed.
//
// This isolates the per-request cost WaMux controls. The endpoint is not the
// bottleneck for a send: the WhatsApp server round trip is, which the
// *WhatsAppRTT benchmark models explicitly.

const (
	benchInstanceID    = "bench-instance"
	benchInstanceToken = "bench-instance-token"
	benchAdminKey      = "bench-admin-key"
)

var (
	textBody  = []byte(`{"number":"5511999999999","text":"hello from the benchmark harness"}`)
	mediaBody = []byte(`{"number":"5511999999999","type":"image","url":"https://example.com/photo.jpg"}`)
)

type benchHTTP struct {
	engine *gin.Engine
}

func newBenchHTTP(sendLatency time.Duration, ratePerMinute, maxConcurrent int) *benchHTTP {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		GlobalApiKey:           benchAdminKey,
		SendRateLimitPerMinute: ratePerMinute,
		SendMaxConcurrent:      maxConcurrent,
	}
	inst := &instance_model.Instance{Id: benchInstanceID, Name: "bench", Token: benchInstanceToken}
	instSvc := &fakeInstanceService{instances: map[string]*instance_model.Instance{
		benchInstanceToken: inst,
	}}
	mw := auth_middleware.NewMiddleware(cfg, instSvc)
	jid := auth_middleware.NewJIDValidationMiddleware()
	guard := httpguard.NewSendGuard(cfg.SendRateLimitPerMinute, cfg.SendMaxConcurrent)
	sendH := send_handler.NewSendHandler(&fakeSendService{latency: sendLatency})
	instH := instance_handler.NewInstanceHandler(instSvc, cfg)

	eng := gin.New()
	eng.GET("/server/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	send := eng.Group("/send")
	send.Use(mw.Auth, guard.Middleware())
	send.POST("/text", jid.ValidateNumberFieldWithFormatJid(), sendH.SendText)
	send.POST("/media", jid.ValidateNumberFieldWithFormatJid(), sendH.SendMedia)

	admin := eng.Group("/instance")
	admin.Use(mw.AuthAdmin)
	admin.GET("/all", instH.All)

	return &benchHTTP{engine: eng}
}

func (s *benchHTTP) serve(req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.engine.ServeHTTP(w, req)
	return w
}

// benchJSON drives one route repeatedly. The request object is reused and only
// its body reset, so the measurement is server-side work, not client allocs.
func benchJSON(b *testing.B, s *benchHTTP, method, path, token string, body []byte) {
	b.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("apikey", token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
		w.Body.Reset()
		s.engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("%s %s: status %d body %s", method, path, w.Code, w.Body.String())
		}
	}
}

func BenchmarkSendTextEndpoint(b *testing.B) {
	benchJSON(b, newBenchHTTP(0, 0, 0), http.MethodPost, "/send/text", benchInstanceToken, textBody)
}

func BenchmarkSendMediaEndpoint(b *testing.B) {
	benchJSON(b, newBenchHTTP(0, 0, 0), http.MethodPost, "/send/media", benchInstanceToken, mediaBody)
}

func BenchmarkInstanceListEndpoint(b *testing.B) {
	benchJSON(b, newBenchHTTP(0, 0, 0), http.MethodGet, "/instance/all", benchAdminKey, nil)
}

func BenchmarkHealthEndpoint(b *testing.B) {
	benchJSON(b, newBenchHTTP(0, 0, 0), http.MethodGet, "/server/health", "", nil)
}

// BenchmarkSendTextEndpointWhatsAppRTT models a 20ms WhatsApp server round trip.
// The delta against BenchmarkSendTextEndpoint is the part of send latency WaMux
// cannot remove by optimizing its own request path.
func BenchmarkSendTextEndpointWhatsAppRTT(b *testing.B) {
	benchJSON(b, newBenchHTTP(20*time.Millisecond, 0, 0), http.MethodPost, "/send/text", benchInstanceToken, textBody)
}

func do(t *testing.T, s *benchHTTP, method, path, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if token != "" {
		req.Header.Set("apikey", token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return s.serve(req)
}

// TestHTTPPipeline validates every branch the endpoint benchmarks measure, so a
// green benchmark always corresponds to a correct pipeline.
func TestHTTPPipeline(t *testing.T) {
	s := newBenchHTTP(0, 0, 0)

	if w := do(t, s, http.MethodGet, "/server/health", "", nil); w.Code != http.StatusOK {
		t.Fatalf("health: want 200, got %d", w.Code)
	}

	if w := do(t, s, http.MethodPost, "/send/text", benchInstanceToken, textBody); w.Code != http.StatusOK {
		t.Fatalf("valid send: want 200, got %d body %s", w.Code, w.Body.String())
	} else if !bytes.Contains(w.Body.Bytes(), []byte(`"message":"success"`)) {
		t.Fatalf("valid send: unexpected body %s", w.Body.String())
	}

	if w := do(t, s, http.MethodPost, "/send/text", "", textBody); w.Code != http.StatusUnauthorized {
		t.Fatalf("missing apikey: want 401, got %d", w.Code)
	}
	if w := do(t, s, http.MethodPost, "/send/text", "wrong-token", textBody); w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown token: want 401, got %d", w.Code)
	}
	if w := do(t, s, http.MethodPost, "/send/text", benchInstanceToken, []byte(`{"text":"no number"}`)); w.Code != http.StatusBadRequest {
		t.Fatalf("missing number: want 400, got %d", w.Code)
	}
	if w := do(t, s, http.MethodPost, "/send/text", benchInstanceToken, []byte(`{"number":"5511999999999"}`)); w.Code != http.StatusBadRequest {
		t.Fatalf("missing text: want 400, got %d", w.Code)
	}

	if w := do(t, s, http.MethodPost, "/send/media", benchInstanceToken, mediaBody); w.Code != http.StatusOK {
		t.Fatalf("valid media: want 200, got %d body %s", w.Code, w.Body.String())
	}

	if w := do(t, s, http.MethodGet, "/instance/all", benchAdminKey, nil); w.Code != http.StatusOK {
		t.Fatalf("admin list: want 200, got %d", w.Code)
	}
	if w := do(t, s, http.MethodGet, "/instance/all", benchInstanceToken, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("admin list with instance token: want 401, got %d", w.Code)
	}
}

// TestSendGuardRateLimit validates that the per-instance guard the send
// benchmarks run under actually sheds load when configured.
func TestSendGuardRateLimit(t *testing.T) {
	s := newBenchHTTP(0, 2, 0)
	codes := make([]int, 0, 3)
	for i := 0; i < 3; i++ {
		codes = append(codes, do(t, s, http.MethodPost, "/send/text", benchInstanceToken, textBody).Code)
	}
	if codes[0] != http.StatusOK || codes[1] != http.StatusOK || codes[2] != http.StatusTooManyRequests {
		t.Fatalf("expected [200 200 429], got %v", codes)
	}
}
