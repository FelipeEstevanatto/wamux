package auth_middleware

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
	"github.com/gin-gonic/gin"
)

// fakeInstanceService embeds the full interface so only the auth lookup needs a
// real implementation; the rest would panic if the code under test called them.
type fakeInstanceService struct {
	instance_service.InstanceService
	instance *instance_model.Instance
	err      error
}

func (f *fakeInstanceService) GetInstanceByToken(string) (*instance_model.Instance, error) {
	return f.instance, f.err
}

func newTestRouter(mw ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/x", append(mw, func(c *gin.Context) { c.Status(http.StatusOK) })...)
	return r
}

func doJSON(r *gin.Engine, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// --- Auth / AuthAdmin / RequireAdminKey ---

func TestAuthRejectsMissingAndUnknownToken(t *testing.T) {
	svc := &fakeInstanceService{err: http.ErrNoLocation} // any non-nil error
	m := NewMiddleware(&config.Config{GlobalApiKey: "admin"}, svc)

	for _, tc := range []struct {
		name  string
		token string
	}{
		{"missing", ""},
		{"unknown", "nope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRouter(m.Auth)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/x", nil)
			if tc.token != "" {
				req.Header.Set("apikey", tc.token)
			}
			r.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", w.Code)
			}
		})
	}
}

func TestAuthSetsInstanceAndContinues(t *testing.T) {
	inst := &instance_model.Instance{Id: "inst-1", Name: "one"}
	m := NewMiddleware(&config.Config{}, &fakeInstanceService{instance: inst})

	var seen *instance_model.Instance
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/x", m.Auth, func(c *gin.Context) {
		if v, ok := c.Get("instance"); ok {
			seen, _ = v.(*instance_model.Instance)
		}
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("apikey", "valid")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if seen == nil || seen.Id != "inst-1" {
		t.Fatalf("instance not propagated to handler: %+v", seen)
	}
}

func TestAuthAdmin(t *testing.T) {
	m := NewMiddleware(&config.Config{GlobalApiKey: "admin-key"}, &fakeInstanceService{})
	for _, tc := range []struct {
		name string
		key  string
		want int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"wrong", "other", http.StatusUnauthorized},
		{"correct", "admin-key", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRouter(m.AuthAdmin)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/x", nil)
			if tc.key != "" {
				req.Header.Set("apikey", tc.key)
			}
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestRequireAdminKey(t *testing.T) {
	r := newTestRouter(RequireAdminKey("global"))
	for _, tc := range []struct {
		name string
		key  string
		want int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"wrong", "nope", http.StatusUnauthorized},
		{"correct", "global", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/x", nil)
			if tc.key != "" {
				req.Header.Set("apikey", tc.key)
			}
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

// --- JID validation ---

// bodySink records the JSON body the handler actually received after the
// middleware's read/restore/rewrite cycle.
func bodySink(seen *map[string]any) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		_ = json.Unmarshal(body, seen)
		c.Status(http.StatusOK)
	}
}

func TestValidateJIDFieldsNormalizesAndRewritesBody(t *testing.T) {
	m := NewJIDValidationMiddleware()
	var seen map[string]any
	r := newTestRouter(m.ValidateJIDFields("number"), bodySink(&seen))

	w := doJSON(r, `{"number":"11999999999"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	if got := seen["number"]; got != "+11999999999@s.whatsapp.net" {
		t.Fatalf("number = %v, want normalized JID", got)
	}
}

func TestValidateJIDFieldsRejectsBadInput(t *testing.T) {
	m := NewJIDValidationMiddleware()
	r := newTestRouter(m.ValidateJIDFields("number"))

	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"invalid json", `{"number":`, http.StatusBadRequest},
		{"empty value", `{"number":""}`, http.StatusBadRequest},
		{"unparseable number", `{"number":"+++"}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if w := doJSON(r, tc.body); w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestValidateJIDFieldsPassesNonJSONThrough(t *testing.T) {
	m := NewJIDValidationMiddleware()
	r := newTestRouter(m.ValidateJIDFields("number"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("plain"))
	req.Header.Set("Content-Type", "text/plain")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("non-JSON body should pass through, got %d", w.Code)
	}
}

func TestValidateJIDFieldsRejectsOversizedBody(t *testing.T) {
	m := NewJIDValidationMiddleware()
	r := newTestRouter(m.ValidateJIDFields("number"))

	// One byte over the cap: readBody must convert it to a 413, not buffer it.
	w := doJSON(r, strings.Repeat("a", maxBodyBytes+1))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", w.Code)
	}
}

func TestValidateNumberField(t *testing.T) {
	m := NewJIDValidationMiddleware()

	t.Run("single string normalized", func(t *testing.T) {
		var seen map[string]any
		r := newTestRouter(m.ValidateNumberField(), bodySink(&seen))
		if w := doJSON(r, `{"number":"11999999999"}`); w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		if seen["number"] != "+11999999999@s.whatsapp.net" {
			t.Fatalf("number = %v", seen["number"])
		}
	})

	t.Run("array normalized", func(t *testing.T) {
		var seen map[string]any
		r := newTestRouter(m.ValidateNumberField(), bodySink(&seen))
		if w := doJSON(r, `{"number":["11999999999","11988888888"]}`); w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		arr, _ := seen["number"].([]any)
		if len(arr) != 2 || arr[0] != "+11999999999@s.whatsapp.net" {
			t.Fatalf("number = %v", seen["number"])
		}
	})

	for _, tc := range []struct {
		name string
		body string
	}{
		{"empty string", `{"number":""}`},
		{"empty array", `{"number":[]}`},
		{"empty array element", `{"number":[""]}`},
		{"wrong type", `{"number":42}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestRouter(m.ValidateNumberField())
			if w := doJSON(r, tc.body); w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", w.Code)
			}
		})
	}
}

func TestValidateNumberFieldWithFormatJidFalseSkipsNormalization(t *testing.T) {
	m := NewJIDValidationMiddleware()
	var seen map[string]any
	r := newTestRouter(m.ValidateNumberFieldWithFormatJid(), bodySink(&seen))

	// Not a valid phone number, but formatJid=false means "accept as received".
	w := doJSON(r, `{"number":"not-a-jid","formatJid":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if seen["number"] != "not-a-jid" {
		t.Fatalf("number = %v, want raw value preserved", seen["number"])
	}
}

func TestValidateMultipleNumbers(t *testing.T) {
	m := NewJIDValidationMiddleware()

	t.Run("normalizes members", func(t *testing.T) {
		var seen map[string]any
		r := newTestRouter(m.ValidateMultipleNumbers("numbers"), bodySink(&seen))
		if w := doJSON(r, `{"numbers":["11999999999","11988888888"]}`); w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		arr, _ := seen["numbers"].([]any)
		if len(arr) != 2 || arr[1] != "+11988888888@s.whatsapp.net" {
			t.Fatalf("numbers = %v", seen["numbers"])
		}
	})

	t.Run("rejects invalid member", func(t *testing.T) {
		r := newTestRouter(m.ValidateMultipleNumbers("numbers"))
		if w := doJSON(r, `{"numbers":["11999999999","+++"]}`); w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("absent field is untouched", func(t *testing.T) {
		r := newTestRouter(m.ValidateMultipleNumbers("numbers"))
		if w := doJSON(r, `{}`); w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})
}

func TestValidateContactFields(t *testing.T) {
	m := NewJIDValidationMiddleware()

	t.Run("valid number and vcard", func(t *testing.T) {
		var seen map[string]any
		r := newTestRouter(m.ValidateContactFields(), bodySink(&seen))
		body := `{"number":"11999999999","vcard":{"fullName":"A","phone":"11988888888"}}`
		if w := doJSON(r, body); w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		if seen["number"] != "+11999999999@s.whatsapp.net" {
			t.Fatalf("number = %v", seen["number"])
		}
	})

	t.Run("invalid vcard phone", func(t *testing.T) {
		r := newTestRouter(m.ValidateContactFields())
		if w := doJSON(r, `{"vcard":{"phone":"+++"}}`); w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})
}

func TestValidateJIDFieldsMultipartForm(t *testing.T) {
	m := NewJIDValidationMiddleware()

	call := func(number string) *httptest.ResponseRecorder {
		body := strings.NewReader("--b\r\nContent-Disposition: form-data; name=\"number\"\r\n\r\n" + number + "\r\n--b--\r\n")
		req := httptest.NewRequest(http.MethodPost, "/x", body)
		req.Header.Set("Content-Type", "multipart/form-data; boundary=b")
		w := httptest.NewRecorder()
		newTestRouter(m.ValidateJIDFields("number")).ServeHTTP(w, req)
		return w
	}

	if w := call("11999999999"); w.Code != http.StatusOK {
		t.Fatalf("valid multipart number: status = %d, want 200", w.Code)
	}
	if w := call(""); w.Code != http.StatusBadRequest {
		t.Fatalf("empty multipart number: status = %d, want 400", w.Code)
	}
}
