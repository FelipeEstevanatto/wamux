package cloud

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/channel"
	cloud_model "github.com/felipeestevanatto/wamux/pkg/cloud/model"
	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var _ = sqlite.Open

func newCloudTestService(t *testing.T, handler http.HandlerFunc) (*Service, *config.Config, *httptest.Server) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/cloud.db"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&cloud_model.App{}, &cloud_model.Credentials{}, &cloud_model.Contact{},
		&cloud_model.Template{}, &cloud_model.ChatWindow{}, &cloud_model.WebhookEvent{},
		&cloud_model.MediaJob{}, &cloud_model.StatusEvent{}, &cloud_model.AccountEvent{},
		&cloud_model.UserPreference{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	srv := httptest.NewServer(handler)
	cfg := &config.Config{
		DataEncryptionKey:     []byte("01234567890123456789012345678901"),
		CloudAPIGraphBaseURL:  srv.URL,
		CloudAPIGraphVersion:  "v27.0",
		CloudAPIWindowHours:   24,
		CloudAPIRPS:           1000,
		CloudAPIMaxMediaBytes: 10 << 20,
	}
	client := NewClient(cfg, nil)
	svc := NewService(NewRepository(db), client, cfg, nil, nil, nil, nil, nil)
	if err := svc.SaveApp("app-1", "secret", "verify", "v27.0"); err != nil {
		t.Fatalf("save app: %v", err)
	}
	return svc, cfg, srv
}

func seedCloudCredentials(t *testing.T, svc *Service) *instance_model.Instance {
	t.Helper()
	inst := &instance_model.Instance{Id: "11111111-1111-1111-1111-111111111111", Name: "cloud", Integration: "WHATSAPP-BUSINESS"}
	noWindow := false
	if err := svc.SaveCredentials(inst.Id, CredentialsInput{
		AppID: "app-1", WABAID: "waba-1", PhoneNumberID: "phone-1", AccessToken: "token-1",
		EnforceWindow: &noWindow,
	}); err != nil {
		t.Fatalf("save credentials: %v", err)
	}
	return inst
}

func TestSaveCredentialsIsWriteOnlyAndPreservesRegistration(t *testing.T) {
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()
	inst := seedCloudCredentials(t, svc)
	summary := svc.CredentialSummary(inst)
	if summary["accessTokenSet"] != true {
		t.Fatalf("accessTokenSet = %v", summary["accessTokenSet"])
	}
	if _, ok := summary["accessToken"]; ok {
		t.Fatal("summary leaked accessToken")
	}
	creds, err := svc.Repository().GetCredentials(inst.Id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(creds.AccessTokenEnc, "token-1") {
		t.Fatal("access token stored in plaintext")
	}
	creds.Registered = true
	if err := svc.Repository().SaveCredentials(creds); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveCredentials(inst.Id, CredentialsInput{AccessToken: "token-2"}); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	after, _ := svc.Repository().GetCredentials(inst.Id)
	if !after.Registered {
		t.Fatal("rotation dropped registration state")
	}
}

func TestSendTextUsesWAMIDAndDoesNotRetryPost(t *testing.T) {
	var calls atomic.Int32
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v27.0/phone-1/messages" {
			calls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"messaging_product":"whatsapp","messages":[{"id":"wamid.test"}]}`))
			return
		}
		http.NotFound(w, r)
	})
	defer srv.Close()
	inst := seedCloudCredentials(t, svc)
	res, err := svc.SendText(context.Background(), inst, channelText("5511999999999", "hello"))
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if res.ID != "wamid.test" {
		t.Fatalf("id = %q", res.ID)
	}

	failSvc, _, failSrv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v27.0/phone-1/messages" {
			calls.Add(1)
			http.Error(w, `{"error":{"message":"boom","code":500}}`, http.StatusInternalServerError)
			return
		}
		http.NotFound(w, r)
	})
	defer failSrv.Close()
	failInst := seedCloudCredentials(t, failSvc)
	before := calls.Load()
	_, err = failSvc.SendText(context.Background(), failInst, channelText("5511999999999", "hello"))
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != before+1 {
		t.Fatalf("POST was retried: %d calls", calls.Load()-before)
	}
}

func TestWebhookSignatureAndBSUID(t *testing.T) {
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()
	inst := seedCloudCredentials(t, svc)
	body := []byte(`{
	  "object":"whatsapp_business_account",
	  "entry":[{"id":"waba-1","changes":[{"field":"messages","value":{
	    "metadata":{"phone_number_id":"phone-1"},
	    "contacts":[{"profile":{"name":"Ana","username":"ana"},"user_id":"US.123","wa_id":"5511888888888"}],
	    "messages":[{"from":"5511888888888","from_user_id":"US.123","id":"wamid.in","timestamp":"` + strconv.FormatInt(time.Now().Unix(), 10) + `","type":"text","text":{"body":"hi"}}]
	  }}]}]
	}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if err := svc.ReceiveWebhook(body, sig, ""); err != nil {
		t.Fatalf("webhook: %v", err)
	}
	contact, err := svc.Repository().GetContact(inst.Id, "bsuid:US.123")
	if err != nil {
		t.Fatalf("contact: %v", err)
	}
	if contact.WAID != "5511888888888" || contact.Username != "ana" {
		t.Fatalf("contact = %+v", contact)
	}
	win, err := svc.Repository().GetWindow(inst.Id, "bsuid:US.123")
	if err != nil || win.ExpiresAt.Before(time.Now()) {
		t.Fatalf("window = %+v, err=%v", win, err)
	}
	if err := svc.ReceiveWebhook(body, "sha256=00", ""); err != ErrInvalidSignature {
		t.Fatalf("bad signature err = %v", err)
	}
	// Duplicate delivery must not create a second event/message.
	if err := svc.ReceiveWebhook(body, sig, ""); err != nil {
		t.Fatalf("duplicate webhook: %v", err)
	}
}

func TestRegisterAndDiagnostics(t *testing.T) {
	var registered atomic.Bool
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/register"):
			registered.Store(true)
			_, _ = w.Write([]byte(`{"success":true}`))
		case strings.HasSuffix(r.URL.Path, "/phone-1"):
			_, _ = w.Write([]byte(`{"display_phone_number":"+5511","verified_name":"Acme","quality_rating":"GREEN","status":"CONNECTED"}`))
		default:
			http.NotFound(w, r)
		}
	})
	defer srv.Close()
	inst := seedCloudCredentials(t, svc)
	status, err := svc.Register(inst, "123456")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !registered.Load() || !status.Registered {
		t.Fatal("not registered")
	}
	diag, err := svc.Diagnostics(inst)
	if err != nil || !diag.TokenValid {
		t.Fatalf("diagnostics = %+v, err=%v", diag, err)
	}
}

func channelText(to, text string) channel.TextMessage {
	return channel.TextMessage{To: to, Body: text}
}

type recordingSink struct {
	mu     sync.Mutex
	events []map[string]any
}

func (r *recordingSink) CallWebhook(_ *instance_model.Instance, _ string, data []byte) {
	var payload map[string]any
	if json.Unmarshal(data, &payload) == nil {
		r.mu.Lock()
		r.events = append(r.events, payload)
		r.mu.Unlock()
	}
}
func (r *recordingSink) SendToGlobalQueues(string, []byte, string) {}
func (r *recordingSink) snapshot() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]map[string]any(nil), r.events...)
}

func TestWebhookEmitsOneMessageEvent(t *testing.T) {
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()
	seedCloudCredentials(t, svc)
	sink := &recordingSink{}
	svc.events = sink
	body := []byte(`{"object":"whatsapp_business_account","entry":[{"id":"waba-1","changes":[{"field":"messages","value":{"metadata":{"phone_number_id":"phone-1"},"contacts":[{"profile":{"name":"Ana"},"user_id":"US.123","wa_id":"5511888888888"}],"messages":[{"from":"5511888888888","from_user_id":"US.123","id":"wamid.event","timestamp":"` + strconv.FormatInt(time.Now().Unix(), 10) + `","type":"text","text":{"body":"hi"}}]}}]}]}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	if err := svc.ReceiveWebhook(body, "sha256="+hex.EncodeToString(mac.Sum(nil)), ""); err != nil {
		t.Fatalf("webhook: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	events := sink.snapshot()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0]["event"] != "Message" || events[0]["channel"] != "cloud" {
		t.Fatalf("event = %+v", events[0])
	}
}

func TestPartialMalformedChangeDoesNotDropValidMessage(t *testing.T) {
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()
	seedCloudCredentials(t, svc)
	sink := &recordingSink{}
	svc.events = sink
	body := []byte(`{"object":"whatsapp_business_account","entry":[{"id":"waba-1","changes":[{"field":"messages","value":"not-an-object"},{"field":"messages","value":{"metadata":{"phone_number_id":"phone-1"},"contacts":[{"profile":{"name":"Ana"},"user_id":"US.456"}],"messages":[{"from_user_id":"US.456","id":"wamid.event2","timestamp":"` + strconv.FormatInt(time.Now().Unix(), 10) + `","type":"text","text":{"body":"hi"}}]}}]}]}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	if err := svc.ReceiveWebhook(body, "sha256="+hex.EncodeToString(mac.Sum(nil)), ""); err != nil {
		t.Fatalf("webhook: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if events := sink.snapshot(); len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

func TestSendInteractiveAndMarkRead(t *testing.T) {
	var messages atomic.Int32
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/messages") {
			messages.Add(1)
			_, _ = w.Write([]byte(`{"messaging_product":"whatsapp","messages":[{"id":"wamid.i"}]}`))
			return
		}
		http.NotFound(w, r)
	})
	defer srv.Close()
	inst := seedCloudCredentials(t, svc)
	if got, _ := svc.repo.GetCredentials(inst.Id); got != nil {
		t.Logf("enforce=%v", got.EnforceWindow)
	}
	_, err := svc.SendInteractive(context.Background(), inst, channel.InteractiveMessage{
		To: "5511999999999", Body: "Choose", ButtonText: "Menu",
		Sections: []channel.InteractiveSection{{Title: "Plans", Rows: []channel.InteractiveListRow{{ID: "a", Title: "A"}}}},
	})
	if err != nil {
		t.Fatalf("interactive: %v", err)
	}
	if err := svc.MarkRead(context.Background(), inst, "wamid.i", true); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if messages.Load() != 2 {
		t.Fatalf("messages requests = %d, want 2", messages.Load())
	}
}

func TestMapGraphErrorWindowAndRegistration(t *testing.T) {
	if got := MapGraphError(&GraphError{Code: 131047, StatusCode: 400}).Code; got != "cloud_window_expired" {
		t.Fatalf("window code = %q", got)
	}
	if got := MapGraphError(&GraphError{Code: 133010, StatusCode: 400}).Code; got != "cloud_phone_not_registered" {
		t.Fatalf("registration code = %q", got)
	}
}

func TestWindowPreCheckBlocksFreeForm(t *testing.T) {
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"messaging_product":"whatsapp","messages":[{"id":"wamid.window"}]}`))
	})
	defer srv.Close()
	inst := &instance_model.Instance{Id: "11111111-1111-1111-1111-111111111111", Integration: "WHATSAPP-BUSINESS"}
	enforce := true
	if err := svc.SaveCredentials(inst.Id, CredentialsInput{
		AppID: "app-1", WABAID: "waba-1", PhoneNumberID: "phone-1", AccessToken: "token-1",
		EnforceWindow: &enforce,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendText(context.Background(), inst, channelText("5511999999999", "blocked")); err == nil {
		t.Fatal("expected window error")
	}
	now := time.Now()
	if err := svc.repo.UpsertWindow(&cloud_model.ChatWindow{
		InstanceID: inst.Id, ContactID: "phone:5511999999999",
		LastInboundAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendText(context.Background(), inst, channelText("5511999999999", "allowed")); err != nil {
		t.Fatalf("send in window: %v", err)
	}
}

func TestConnectVerifiesAndSubscribes(t *testing.T) {
	var subscribed atomic.Bool
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/phone-1"):
			_, _ = w.Write([]byte(`{"display_phone_number":"+5511","verified_name":"Acme","quality_rating":"GREEN","status":"CONNECTED"}`))
		case strings.HasSuffix(r.URL.Path, "/waba-1/subscribed_apps"):
			subscribed.Store(true)
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(w, r)
		}
	})
	defer srv.Close()
	inst := seedCloudCredentials(t, svc)
	status, err := svc.Connect(inst)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if !subscribed.Load() || !status.Subscribed || status.DisplayPhoneNumber != "+5511" {
		t.Fatalf("status = %+v subscribed=%v", status, subscribed.Load())
	}
	if !inst.Connected {
		t.Fatal("instance was not marked connected")
	}
}
