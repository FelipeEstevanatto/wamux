package instance_handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

// serve runs a handler as the tenant "own" against path id. The service is nil on
// purpose: a request that is refused must never reach it.
func serveAdvancedSettings(method, pathID string, handler func(*instanceHandler) gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("instance", &instance_model.Instance{Id: "own"})
		c.Next()
	})
	r.Handle(method, "/instance/:instanceId/advanced-settings", handler(&instanceHandler{}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/instance/"+pathID+"/advanced-settings", strings.NewReader(`{"rejectCall":true}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// A valid token proves ownership of its own instance only. Any tenant used to be
// able to read and rewrite another's settings by putting its id in the path.
func TestAdvancedSettingsRefuseAnotherInstance(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		handler func(*instanceHandler) gin.HandlerFunc
	}{
		{"get", http.MethodGet, func(h *instanceHandler) gin.HandlerFunc { return h.GetAdvancedSettings }},
		{"update", http.MethodPut, func(h *instanceHandler) gin.HandlerFunc { return h.UpdateAdvancedSettings }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if w := serveAdvancedSettings(tc.method, "someone-else", tc.handler); w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", w.Code)
			}
		})
	}
}

func TestAuthorizedInstanceParamAcceptsOwnInstance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("instance", &instance_model.Instance{Id: "own"})
	c.Params = gin.Params{{Key: "instanceId", Value: "own"}}

	if id, ok := authorizedInstanceParam(c); !ok || id != "own" {
		t.Fatalf("got %q, %v", id, ok)
	}
}
