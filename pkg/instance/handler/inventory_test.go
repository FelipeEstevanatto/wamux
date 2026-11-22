package instance_handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	instance_service "github.com/felipeestevanatto/wamux/pkg/instance/service"
	whatsmeow_service "github.com/felipeestevanatto/wamux/pkg/whatsmeow/service"
	"github.com/gin-gonic/gin"
)

type inventoryService struct {
	instance_service.InstanceService
	list []*instance_model.Instance
}

func (s inventoryService) GetAll() ([]*instance_model.Instance, error) { return s.list, nil }

func (s inventoryService) InstanceHealth(string) *whatsmeow_service.InstanceHealth {
	return nil
}

func TestInventoryDoesNotExposeSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jid := "5511999999999@s.whatsapp.net"
	h := &instanceHandler{instanceService: inventoryService{list: []*instance_model.Instance{{
		Id: "i-1", Name: "One", Token: "secret-token", Jid: jid,
		Connected: true, AutoReconnect: true, CreatedAt: time.Now(),
	}}}}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.Inventory(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body)
	}
	body := w.Body.String()
	for _, forbidden := range []string{"secret-token", jid, "\"token\"", "\"jid\""} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("inventory leaked %q: %s", forbidden, body)
		}
	}

	var envelope struct {
		Data inventory `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.ContractVersion == "" || envelope.Data.RuntimeID == "" {
		t.Fatalf("missing contract/runtime id: %+v", envelope.Data)
	}
	if len(envelope.Data.Instances) != 1 {
		t.Fatalf("instances = %+v", envelope.Data.Instances)
	}
	item := envelope.Data.Instances[0]
	if item.OwnerFingerprint == "" || item.OwnerFingerprint == jid || item.ConnectionStatus != "connected" {
		t.Fatalf("item = %+v", item)
	}
}
