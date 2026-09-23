package server_handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"github.com/gin-gonic/gin"
)

type fakeOverview struct {
	ov  *whatsmeow_service.InstanceOverview
	err error
}

func (f fakeOverview) GetInstanceOverview(string) (*whatsmeow_service.InstanceOverview, error) {
	return f.ov, f.err
}

func TestInstanceOverviewHandlerReturnsProviderData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &serverHandler{overview: fakeOverview{ov: &whatsmeow_service.InstanceOverview{
		Connected:     true,
		ProfileName:   "Numero CLARO",
		ProfilePicURL: "https://pps.whatsapp.net/x.jpg",
		ContactsCount: 346,
	}}}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "instanceId", Value: "abc"}}
	h.InstanceOverview(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", w.Code, w.Body.String())
	}
	var body struct {
		Data whatsmeow_service.InstanceOverview `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Data.ContactsCount != 346 || !body.Data.Connected || body.Data.ProfilePicURL == "" {
		t.Fatalf("unexpected payload: %+v", body.Data)
	}
}

func TestInstanceOverviewHandlerValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Missing instanceId -> 400.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	(&serverHandler{}).InstanceOverview(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing id: status = %d, want 400", w.Code)
	}

	// No provider wired -> 503.
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "instanceId", Value: "abc"}}
	(&serverHandler{}).InstanceOverview(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("no provider: status = %d, want 503", w.Code)
	}
}

func TestStatsIncludesVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &serverHandler{version: "1.2.3-fork", startTime: time.Now()}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.Stats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body struct {
		System map[string]any `json:"system"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.System["version"] != "1.2.3-fork" {
		t.Fatalf("system.version = %v, want 1.2.3-fork", body.System["version"])
	}
}
