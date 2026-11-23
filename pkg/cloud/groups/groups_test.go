package groups

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

func newGroupService(t *testing.T, fn http.HandlerFunc) (*Service, *cloud_service.Service, *instance_model.Instance, *httptest.Server) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/groups.db"), &gorm.Config{})
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
	inst := &instance_model.Instance{Id: "11111111-1111-1111-1111-111111111111", Name: "groups"}
	if err := cloudSvc.SaveCredentials(inst.Id, cloud_service.CredentialsInput{
		AppID: "app-1", WABAID: "waba-1", PhoneNumberID: "phone-1", AccessToken: "token-1",
	}); err != nil {
		t.Fatal(err)
	}
	groupSvc := New(cloudSvc)
	if err := groupSvc.Migrate(); err != nil {
		t.Fatal(err)
	}
	return groupSvc, cloudSvc, inst, srv
}

func TestGroupsLifecycle(t *testing.T) {
	var sent map[string]any
	groupSvc, _, inst, srv := newGroupService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/groups") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"data":[{"id":"group-1","subject":"Support"}]}`))
		case strings.HasSuffix(r.URL.Path, "/groups") && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"group-2","subject":"Sales"}`))
		case strings.HasSuffix(r.URL.Path, "/group-1"):
			_, _ = w.Write([]byte(`{"id":"group-1","subject":"Support"}`))
		case strings.HasSuffix(r.URL.Path, "/participants"):
			_, _ = w.Write([]byte(`{"data":[{"id":"participant-1"}]}`))
		case strings.HasSuffix(r.URL.Path, "/invite_link"):
			_, _ = w.Write([]byte(`{"invite_link":"https://chat.whatsapp.com/abc"}`))
		case strings.HasSuffix(r.URL.Path, "/join_requests"):
			_, _ = w.Write([]byte(`{"data":[{"id":"request-1"}]}`))
		case strings.HasSuffix(r.URL.Path, "/join_requests/request-1"):
			_, _ = w.Write([]byte(`{"success":true}`))
		case strings.HasSuffix(r.URL.Path, "/settings"):
			_, _ = w.Write([]byte(`{"success":true}`))
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_ = json.NewDecoder(r.Body).Decode(&sent)
			_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.group"}]}`))
		default:
			t.Logf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()
	items, err := groupSvc.List(context.Background(), inst)
	if err != nil || len(items) != 1 || items[0].ID != "group-1" {
		t.Fatalf("list=%+v err=%v", items, err)
	}
	if _, err := groupSvc.Create(context.Background(), inst, "Sales", "team", []string{"5511"}); err != nil {
		t.Fatal(err)
	}
	if _, err := groupSvc.AddParticipants(context.Background(), inst, "group-1", []string{"5512"}); err != nil {
		t.Fatal(err)
	}
	if _, err := groupSvc.InviteLink(context.Background(), inst, "group-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := groupSvc.JoinRequests(context.Background(), inst, "group-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := groupSvc.DecideJoinRequest(context.Background(), inst, "group-1", "request-1", "approve"); err != nil {
		t.Fatal(err)
	}
	if _, err := groupSvc.UpdateSettings(context.Background(), inst, "group-1", map[string]any{"join_approval_mode": "auto_approve"}); err != nil {
		t.Fatal(err)
	}
	if _, err := groupSvc.Send(context.Background(), inst, "group-1", map[string]any{"type": "text", "text": map[string]any{"body": "hello"}}); err != nil {
		t.Fatal(err)
	}
	if sent["to"] != "group-1" || sent["recipient_type"] != "group" {
		t.Fatalf("sent=%+v", sent)
	}
	if _, err := groupSvc.Send(context.Background(), inst, "group-1", map[string]any{"type": "interactive"}); err == nil {
		t.Fatal("interactive group send should be rejected")
	}
	groupSvc.HandleCloudChange(inst.Id, "group_participants_update", []byte(`{"group_id":"group-1","event":"add"}`))
	events, err := groupSvc.ListEvents(inst.Id, "group-1", 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%+v err=%v", events, err)
	}
}
