package instance_handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
)

// stubInstanceService records the HMAC calls. The embedded interface satisfies
// the rest of InstanceService.
type stubInstanceService struct {
	instance_service.InstanceService
	setKey    string
	setCalls  int
	setErr    error
	cleared   bool
	status    *instance_service.HmacConfigStatus
	statusErr error
}

func (s *stubInstanceService) SetWebhookHmacKey(_ string, key string) (*instance_service.HmacConfigStatus, error) {
	s.setKey = key
	s.setCalls++
	if s.setErr != nil {
		return nil, s.setErr
	}
	return &instance_service.HmacConfigStatus{Configured: true, GlobalFallback: true}, nil
}

func (s *stubInstanceService) ClearWebhookHmacKey(string) error {
	s.cleared = true
	return nil
}

func (s *stubInstanceService) WebhookHmacStatus(string) (*instance_service.HmacConfigStatus, error) {
	if s.statusErr != nil {
		return nil, s.statusErr
	}
	if s.status == nil {
		return &instance_service.HmacConfigStatus{}, nil
	}
	return s.status, nil
}

func newHmacRouter(svc instance_service.InstanceService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("instance", &instance_model.Instance{Id: "inst-1", Token: "tok"})
	})
	h := NewInstanceHandler(svc, &config.Config{})
	r.POST("/instance/hmac", h.SetHmac)
	r.GET("/instance/hmac", h.GetHmac)
	r.DELETE("/instance/hmac", h.DeleteHmac)
	return r
}

func doHmac(t *testing.T, r *gin.Engine, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, "/instance/hmac", reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestSetHmacStoresProvidedKey(t *testing.T) {
	svc := &stubInstanceService{}
	r := newHmacRouter(svc)
	key := strings.Repeat("k", 40)

	w := doHmac(t, r, http.MethodPost, `{"hmacKey":"`+key+`"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	if svc.setKey != key || svc.setCalls != 1 {
		t.Fatalf("setKey = %q (%d calls)", svc.setKey, svc.setCalls)
	}

	var body struct {
		Data HmacKeyResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Data.Configured || !body.Data.GlobalFallback {
		t.Fatalf("response = %+v", body.Data)
	}
	if body.Data.GeneratedKey != "" {
		t.Fatal("an explicitly provided key must not be echoed back")
	}
}

func TestSetHmacGeneratesAndReturnsKeyOnce(t *testing.T) {
	svc := &stubInstanceService{}
	r := newHmacRouter(svc)

	w := doHmac(t, r, http.MethodPost, `{"generate":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}

	var body struct {
		Data HmacKeyResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Data.GeneratedKey) != 64 {
		t.Fatalf("generated key length = %d, want 64 hex chars", len(body.Data.GeneratedKey))
	}
	if svc.setKey != body.Data.GeneratedKey {
		t.Fatalf("service received %q, response returned %q", svc.setKey, body.Data.GeneratedKey)
	}
}

func TestSetHmacRequiresKeyOrGenerate(t *testing.T) {
	svc := &stubInstanceService{}
	r := newHmacRouter(svc)

	w := doHmac(t, r, http.MethodPost, `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if svc.setCalls != 0 {
		t.Fatal("service must not be called without a key")
	}
}

func TestGetHmacReportsStatus(t *testing.T) {
	svc := &stubInstanceService{status: &instance_service.HmacConfigStatus{Configured: true}}
	r := newHmacRouter(svc)

	w := doHmac(t, r, http.MethodGet, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"configured":true`) {
		t.Fatalf("body = %s", w.Body.String())
	}
	// The key itself must never be serialized.
	if strings.Contains(w.Body.String(), "hmacKey") || strings.Contains(w.Body.String(), "hmac_key") {
		t.Fatalf("status response leaked a key field: %s", w.Body.String())
	}
}

func TestDeleteHmacClearsKey(t *testing.T) {
	svc := &stubInstanceService{}
	r := newHmacRouter(svc)

	w := doHmac(t, r, http.MethodDelete, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !svc.cleared {
		t.Fatal("DeleteHmac did not clear the key")
	}
}
