package calling

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cloud_service "github.com/felipeestevanatto/wamux/pkg/cloud"
	cloud_model "github.com/felipeestevanatto/wamux/pkg/cloud/model"
	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newCallService(t *testing.T, fn http.HandlerFunc) (*Service, *instance_model.Instance, *httptest.Server) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/calls.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&cloud_model.App{}, &cloud_model.Credentials{}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(fn)
	cfg := &config.Config{
		DataEncryptionKey:    []byte("01234567890123456789012345678901"),
		CloudAPIGraphBaseURL: srv.URL, CloudAPIGraphVersion: "v27.0",
	}
	cloudSvc := cloud_service.NewService(cloud_service.NewRepository(db), cloud_service.NewClient(cfg, nil), cfg, nil, nil, nil, nil, nil)
	if err := cloudSvc.SaveApp("app-1", "secret", "verify", "v27.0"); err != nil {
		t.Fatal(err)
	}
	inst := &instance_model.Instance{Id: "11111111-1111-1111-1111-111111111111", Name: "calls"}
	if err := cloudSvc.SaveCredentials(inst.Id, cloud_service.CredentialsInput{
		AppID: "app-1", WABAID: "waba-1", PhoneNumberID: "phone-1", AccessToken: "token-1",
	}); err != nil {
		t.Fatal(err)
	}
	callSvc := New(cloudSvc)
	if err := callSvc.Migrate(); err != nil {
		t.Fatal(err)
	}
	return callSvc, inst, srv
}

func TestCallingControlPlane(t *testing.T) {
	var action map[string]any
	callSvc, inst, srv := newCallService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/calls"):
			_, _ = w.Write([]byte(`{"call_id":"call-1","status":"ringing"}`))
		case strings.HasSuffix(r.URL.Path, "/actions"):
			_ = json.NewDecoder(r.Body).Decode(&action)
			_, _ = w.Write([]byte(`{"success":true}`))
		case strings.HasSuffix(r.URL.Path, "/call_permissions"):
			_, _ = w.Write([]byte(`{"allow":true}`))
		case strings.HasSuffix(r.URL.Path, "/call_settings"):
			_, _ = w.Write([]byte(`{"enabled":true}`))
		case strings.HasSuffix(r.URL.Path, "/call-1"):
			_, _ = w.Write([]byte(`{"id":"call-1","status":"connected"}`))
		default:
			t.Logf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()
	ctx := context.Background()
	if _, err := callSvc.Start(ctx, inst, map[string]any{"to": "5511999999999", "action": "connect"}); err != nil {
		t.Fatal(err)
	}
	if _, err := callSvc.Get(ctx, inst, "call-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := callSvc.Action(ctx, inst, "call-1", "accept", nil); err != nil {
		t.Fatal(err)
	}
	if action["action"] != "accept" {
		t.Fatalf("action=%+v", action)
	}
	if _, err := callSvc.Action(ctx, inst, "call-1", "media", nil); err == nil {
		t.Fatal("media action should be explicitly unsupported")
	}
	if _, err := callSvc.Permissions(ctx, inst); err != nil {
		t.Fatal(err)
	}
	if _, err := callSvc.Settings(ctx, inst); err != nil {
		t.Fatal(err)
	}
	callSvc.HandleCloudChange(inst.Id, "calls", []byte(`{"call_id":"call-1","event":"connect","status":"connected","direction":"inbound"}`))
	events, err := callSvc.ListEvents(inst.Id, "call-1", 10)
	if err != nil || len(events) != 1 || events[0].CallID != "call-1" {
		t.Fatalf("events=%+v err=%v", events, err)
	}
}
