package flows

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/felipeestevanatto/wamux/pkg/channel"
	cloud_service "github.com/felipeestevanatto/wamux/pkg/cloud"
	cloud_model "github.com/felipeestevanatto/wamux/pkg/cloud/model"
	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newFlowService(t *testing.T, handlerFn http.HandlerFunc) (*Service, *cloud_service.Service, *instance_model.Instance, *httptest.Server) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/flows.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&cloud_model.App{}, &cloud_model.Credentials{}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handlerFn)
	cfg := &config.Config{
		DataEncryptionKey:    []byte("01234567890123456789012345678901"),
		CloudAPIGraphBaseURL: srv.URL, CloudAPIGraphVersion: "v27.0",
	}
	cloudSvc := cloud_service.NewService(cloud_service.NewRepository(db), cloud_service.NewClient(cfg, nil), cfg, nil, nil, nil, nil, nil)
	if err := cloudSvc.SaveApp("app-1", "secret 1", "verify", "v27.0"); err != nil {
		t.Fatal(err)
	}
	inst := &instance_model.Instance{Id: "11111111-1111-1111-1111-111111111111", Name: "flows"}
	noWindow := false
	if err := cloudSvc.SaveCredentials(inst.Id, cloud_service.CredentialsInput{
		AppID: "app-1", WABAID: "waba-1", PhoneNumberID: "phone-1",
		AccessToken: "token-1", EnforceWindow: &noWindow,
	}); err != nil {
		t.Fatal(err)
	}
	flowSvc := New(cloudSvc)
	if err := flowSvc.Migrate(); err != nil {
		t.Fatal(err)
	}
	return flowSvc, cloudSvc, inst, srv
}

func TestFlowsLifecycleAndSend(t *testing.T) {
	var paths []string
	var sent map[string]any
	flowSvc, _, inst, srv := newFlowService(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/flows") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"data":[{"id":"flow-1","name":"Order","status":"DRAFT"}],"paging":{"cursors":{"after":""}}}`))
		case strings.HasSuffix(r.URL.Path, "/flows") && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"flow-2","name":"Order","status":"DRAFT"}`))
		case strings.HasSuffix(r.URL.Path, "/publish"):
			_, _ = w.Write([]byte(`{"success":true}`))
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_ = json.NewDecoder(r.Body).Decode(&sent)
			_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.flow"}]}`))
		case strings.HasSuffix(r.URL.Path, "/flow-1"):
			_, _ = w.Write([]byte(`{"id":"flow-1","name":"Order","status":"DRAFT"}`))
		case strings.HasSuffix(r.URL.Path, "/assets"):
			_, _ = w.Write([]byte(`{"data":[{"name":"flow.json"}]}`))
		default:
			t.Logf("unexpected: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()
	items, err := flowSvc.List(context.Background(), inst)
	if err != nil || len(items) != 1 || items[0].ID != "flow-1" {
		t.Fatalf("list = %+v err=%v", items, err)
	}
	created, err := flowSvc.Create(context.Background(), inst, map[string]any{"name": "Order", "categories": []string{"OTHER"}})
	if err != nil || created.ID != "flow-2" {
		t.Fatalf("create = %+v err=%v", created, err)
	}
	if _, err := flowSvc.Publish(context.Background(), inst, "flow-2"); err != nil {
		t.Fatal(err)
	}
	if _, err := flowSvc.Send(context.Background(), inst, "flow-1", channel.FlowMessage{
		To: "5511999999999", Body: "Open flow", FlowCTA: "Open", FlowAction: "navigate",
		FlowActionPayload: map[string]any{"screen": "WELCOME"},
	}); err != nil {
		t.Fatal(err)
	}
	if sent == nil {
		t.Fatalf("no send body; paths=%v", paths)
	}
	interactive, _ := sent["interactive"].(map[string]any)
	if interactive["type"] != "flow" {
		t.Fatalf("payload = %+v", sent)
	}
	if len(paths) < 4 {
		t.Fatalf("paths = %v", paths)
	}
}

func TestFlowCallbackIsSignatureVerified(t *testing.T) {
	gin.SetMode(gin.TestMode)
	flowSvc, _, inst, srv := newFlowService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()
	h := NewHandler(flowSvc)
	eng := gin.New()
	h.RegisterPublicRoutes(eng)
	body := []byte(`{"flow_id":"flow-1","event":"ENDPOINT"}`)
	mac := hmac.New(sha256.New, []byte("secret 1"))
	mac.Write(body)
	req := httptest.NewRequest(http.MethodPost, "/webhook/cloud/flows/"+inst.Id, strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	events, err := flowSvc.ListEvents(inst.Id, "flow-1", 10)
	if err != nil || len(events) != 1 || !events[0].SignatureOK {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	// Unsigned/incorrect signature must be rejected.
	req = httptest.NewRequest(http.MethodPost, "/webhook/cloud/flows/"+inst.Id, strings.NewReader(string(body)))
	w = httptest.NewRecorder()
	eng.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned status=%d", w.Code)
	}
}
