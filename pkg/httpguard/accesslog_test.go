package httpguard

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// logged runs one request through AccessLog and returns what it wrote.
func logged(t *testing.T, mode, target string, status int) string {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var out bytes.Buffer
	previous := gin.DefaultWriter
	gin.DefaultWriter = &out
	defer func() { gin.DefaultWriter = previous }()

	r := gin.New()
	r.Use(AccessLog(mode))
	handler := func(c *gin.Context) { c.Status(status) }
	r.GET("/x", handler)
	r.GET("/ws", handler)
	r.GET("/server/ok", handler)

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
	return out.String()
}

func TestAccessLogModes(t *testing.T) {
	cases := []struct {
		name   string
		mode   string
		target string
		status int
		want   bool
	}{
		{"errors skips success", AccessLogErrors, "/x", 200, false},
		{"errors logs client error", AccessLogErrors, "/x", 401, true},
		{"errors logs server error", AccessLogErrors, "/x", 500, true},
		{"unknown mode means errors", "bogus", "/x", 200, false},
		{"all logs success", AccessLogAll, "/x", 200, true},
		{"429s are never logged", AccessLogAll, "/x", 429, false},
		{"health is never logged", AccessLogAll, "/server/ok", 200, false},
		{"off logs nothing", AccessLogOff, "/x", 500, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := logged(t, tc.mode, tc.target, tc.status) != ""; got != tc.want {
				t.Fatalf("logged = %v, want %v", got, tc.want)
			}
		})
	}
}

// The global API key travels in /ws's query string, so it must never be logged.
func TestAccessLogRedactsCredentials(t *testing.T) {
	out := logged(t, AccessLogAll, "/ws?token=super-secret&instanceId=x", 200)
	if strings.Contains(out, "super-secret") {
		t.Fatalf("credential in log: %s", out)
	}
	if !strings.Contains(out, "/ws?<redacted>") {
		t.Fatalf("expected a redacted path: %s", out)
	}
}

func TestAccessLogKeepsHarmlessQueries(t *testing.T) {
	if got := redactQuery("/chat/history?limit=50"); got != "/chat/history?limit=50" {
		t.Fatalf("got %q", got)
	}
}
