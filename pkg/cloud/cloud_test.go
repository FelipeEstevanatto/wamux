package cloud

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
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
		&cloud_model.UserPreference{}, &cloud_model.ContactIdentity{}, &cloud_model.IdentityEvent{},
		&cloud_model.RateBucket{}, &message_model.Message{},
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

func TestIdentityUpdateMergesMessagesAndWindow(t *testing.T) {
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()
	inst := seedCloudCredentials(t, svc)
	db := svc.repo.DB()
	if err := db.AutoMigrate(&message_model.Message{}, &cloud_model.ContactIdentity{}, &cloud_model.IdentityEvent{}); err != nil {
		t.Fatal(err)
	}
	old := &cloud_model.Contact{InstanceID: inst.Id, ContactID: "bsuid:OLD.1", BSUID: "OLD.1", WAID: "5511888888888"}
	if err := svc.repo.UpsertContact(old); err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().Add(time.Hour)
	if err := svc.repo.UpsertWindow(&cloud_model.ChatWindow{InstanceID: inst.Id, ContactID: old.ContactID, ChatJID: "5511888888888@s.whatsapp.net", LastInboundAt: time.Now(), ExpiresAt: expiry}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&message_model.Message{MessageID: "wamid.identity", InstanceId: inst.Id, ChatJid: "5511888888888@s.whatsapp.net", ContactID: old.ContactID, MessageType: "text", Status: "Received"}).Error; err != nil {
		t.Fatal(err)
	}
	update := IdentityUpdate{WAID: "5511888888888", OldBSUID: "OLD.1", NewBSUID: "NEW.1", EventTime: time.Now()}
	if err := svc.ApplyIdentityUpdate(inst.Id, update); err != nil {
		t.Fatal(err)
	}
	var msg message_model.Message
	if err := db.Where("message_id = ?", "wamid.identity").First(&msg).Error; err != nil {
		t.Fatal(err)
	}
	if msg.ContactID != old.ContactID {
		t.Fatalf("message contact = %q, want %q", msg.ContactID, old.ContactID)
	}
	contact, err := svc.repo.GetContact(inst.Id, old.ContactID)
	if err != nil || contact.BSUID != "NEW.1" {
		t.Fatalf("contact = %+v err=%v", contact, err)
	}
	win, err := svc.repo.GetWindow(inst.Id, old.ContactID)
	if err != nil || win.ExpiresAt.Before(expiry.Add(-time.Second)) {
		t.Fatalf("window = %+v err=%v", win, err)
	}
	ids, _ := svc.repo.ListContactIdentities(inst.Id, old.ContactID)
	if len(ids) < 2 {
		t.Fatalf("identities = %+v", ids)
	}
	// Replaying the same transition must not move rows or create another event.
	if err := svc.ApplyIdentityUpdate(inst.Id, update); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&cloud_model.IdentityEvent{}).Where("instance_id = ?", inst.Id).Count(&count)
	if count != 1 {
		t.Fatalf("identity events = %d, want 1", count)
	}
}

func TestMemoryPhoneLimiterPerPhoneAndBackoff(t *testing.T) {
	lim := NewMemoryPhoneLimiter(1000)
	ctx := context.Background()
	if err := lim.Wait(ctx, "phone-a"); err != nil {
		t.Fatal(err)
	}
	if err := lim.Wait(ctx, "phone-b"); err != nil {
		t.Fatal(err)
	}
	before := lim.EffectiveRPS("phone-a")
	lim.Observe("phone-a", 429, 0)
	if got := lim.EffectiveRPS("phone-a"); got >= before {
		t.Fatalf("rate after 429 = %v, want less than %v", got, before)
	}
	if got := lim.EffectiveRPS("phone-b"); got != before {
		t.Fatalf("other phone rate = %v, want %v", got, before)
	}
}

func TestNewPhoneLimiterFallsBackForSQLite(t *testing.T) {
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()
	if _, ok := NewPhoneLimiter(svc.repo.DB(), 8).(*MemoryPhoneLimiter); !ok {
		t.Fatal("SQLite should use the in-memory limiter")
	}
}

func TestDBPhoneLimiterSharesTokens(t *testing.T) {
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()
	db := svc.repo.DB()
	if err := db.AutoMigrate(&cloud_model.RateBucket{}); err != nil {
		t.Fatal(err)
	}
	lim := &dbPhoneLimiter{db: db, baseRPS: 1000}
	ctx := context.Background()
	if err := lim.Wait(ctx, "phone-db"); err != nil {
		t.Fatal(err)
	}
	if err := lim.Wait(ctx, "phone-db"); err != nil {
		t.Fatal(err)
	}
	var bucket cloud_model.RateBucket
	if err := db.Where("phone_number_id = ?", "phone-db").First(&bucket).Error; err != nil {
		t.Fatal(err)
	}
	if bucket.EffectiveRPS != 1000 || bucket.Tokens >= 1000 {
		t.Fatalf("bucket = %+v", bucket)
	}
}

func TestWebhookFieldCoverageAndUnknownFallback(t *testing.T) {
	svc, app, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()
	inst := seedCloudCredentials(t, svc)
	creds, err := svc.repo.GetCredentials(inst.Id)
	if err != nil {
		t.Fatal(err)
	}
	_ = app
	for field := range knownWebhookFields {
		if !isKnownWebhookField(field) {
			t.Fatalf("%s missing from coverage table", field)
		}
		// Empty values must be harmless: Meta retries must never crash on a
		// partially populated payload.
		if err := svc.processChange(webhookEntry{ID: creds.WABAID}, webhookChange{Field: field, Value: []byte(`{}`)}); err != nil && !errors.Is(err, errSkipChange) {
			t.Fatalf("%s: %v", field, err)
		}
	}
	unknown := "future_meta_field"
	if err := svc.processChange(webhookEntry{ID: creds.WABAID}, webhookChange{Field: unknown, Value: []byte(`{"x":1}`)}); err != nil {
		t.Fatalf("unknown field returned error: %v", err)
	}
	events, err := svc.repo.ListAccountEvents(inst.Id, unknown, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventType != "unknown" {
		t.Fatalf("events = %+v", events)
	}
}

func TestWebhookSignatureFailureObserver(t *testing.T) {
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()
	obs := &fakeCloudObserver{}
	svc.SetObserver(obs)
	if err := svc.ReceiveWebhook([]byte(`{}`), "sha256=bad", "app-1"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("err = %v", err)
	}
	if obs.signatureFailures != 1 {
		t.Fatalf("signature failures = %d", obs.signatureFailures)
	}
}

type fakeCloudObserver struct {
	graph, send, webhook, media, unknownField          int
	signatureFailures, windowExpiries, unknownOutcomes int
}

func (f *fakeCloudObserver) ObserveCloudGraph(string, time.Duration, error) { f.graph++ }
func (f *fakeCloudObserver) ObserveCloudSend(string, string)                { f.send++ }
func (f *fakeCloudObserver) ObserveCloudWebhook(string, string)             { f.webhook++ }
func (f *fakeCloudObserver) ObserveCloudWebhookSignatureFailure()           { f.signatureFailures++ }
func (f *fakeCloudObserver) ObserveCloudUnknownField(string)                { f.unknownField++ }
func (f *fakeCloudObserver) ObserveCloudWindowExpired()                     { f.windowExpiries++ }
func (f *fakeCloudObserver) ObserveCloudMediaJob(string)                    { f.media++ }
func (f *fakeCloudObserver) ObserveCloudRateLimit(string, float64)          {}
func (f *fakeCloudObserver) ObserveCloudUnknownOutcome()                    { f.unknownOutcomes++ }
func (f *fakeCloudObserver) SetCloudMediaQueueDepth(int)                    {}

func TestBusinessManagementUsesCursorsAndCachesAnalytics(t *testing.T) {
	var analyticsCalls int
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/phone_numbers"):
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("after") == "" {
				_, _ = w.Write([]byte(`{"data":[{"id":"phone-1","display_phone_number":"+5511"}],"paging":{"cursors":{"after":"cursor-1"}}}`))
			} else {
				_, _ = w.Write([]byte(`{"data":[{"id":"phone-2","display_phone_number":"+5522"}],"paging":{"cursors":{"after":""}}}`))
			}
		case strings.HasSuffix(r.URL.Path, "/analytics"):
			analyticsCalls++
			_, _ = w.Write([]byte(`{"data":[{"granularity":"DAY"}]}`))
		case strings.HasSuffix(r.URL.Path, "/owned_product_catalogs"):
			if r.Method == http.MethodPost {
				_, _ = w.Write([]byte(`{"id":"catalog-1","name":"Shop"}`))
			} else {
				_, _ = w.Write([]byte(`{"data":[{"id":"catalog-1","name":"Shop"}]}`))
			}
		default:
			t.Logf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()
	inst := seedCloudCredentials(t, svc)
	creds, _ := svc.repo.GetCredentials(inst.Id)
	creds.BusinessID = "business-1"
	if err := svc.repo.SaveCredentials(creds); err != nil {
		t.Fatal(err)
	}
	phones, err := svc.ListBusinessPhoneNumbers(inst)
	if err != nil {
		t.Fatal(err)
	}
	if len(phones) != 2 || phones[1].ID != "phone-2" {
		t.Fatalf("phones = %+v", phones)
	}
	if _, err := svc.GetBusinessAnalytics(inst, "2026-01-01", "2026-01-31", "DAY", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetBusinessAnalytics(inst, "2026-01-01", "2026-01-31", "DAY", ""); err != nil {
		t.Fatal(err)
	}
	if analyticsCalls != 1 {
		t.Fatalf("analytics calls = %d, want 1 (cached)", analyticsCalls)
	}
	cats, err := svc.ListBusinessCatalogs(inst)
	if err != nil || len(cats) != 1 {
		t.Fatalf("catalogs = %+v err=%v", cats, err)
	}
	if _, err := svc.CreateBusinessCatalog(inst, "Shop"); err != nil {
		t.Fatal(err)
	}
}

func TestCloudInteractivePayloadFixtures(t *testing.T) {
	var bodies []map[string]any
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		_, _ = w.Write([]byte(`{"messages":[{"id":"wamid.test"}]}`))
	})
	defer srv.Close()
	inst := seedCloudCredentials(t, svc)
	ctx := context.Background()
	if _, err := svc.SendCTAURL(ctx, inst, channel.CTAURLMessage{To: "5511888888888", Body: "Open", URL: "https://example.com", DisplayText: "Open"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendProduct(ctx, inst, channel.ProductMessage{To: "5511888888888", CatalogID: "cat-1", ProductRetailerID: "sku-1", Body: "Buy"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendProductList(ctx, inst, channel.ProductListMessage{To: "5511888888888", CatalogID: "cat-1", Body: "List", Sections: []channel.ProductSection{{Title: "S", Products: []string{"sku-1"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendCatalog(ctx, inst, channel.CatalogMessage{To: "5511888888888", Body: "Catalog"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendContactRequest(ctx, inst, channel.ContactRequestMessage{To: "5511888888888", Body: "Contact"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SendInteractive(ctx, inst, channel.InteractiveMessage{To: "5511888888888", Body: "CTA", Buttons: []channel.InteractiveButton{{Type: "url", Text: "Go", URL: "https://example.com"}}}); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 6 {
		t.Fatalf("requests = %d", len(bodies))
	}
	want := []string{"cta_url", "product", "product_list", "catalog_message", "request_contact_info", "cta_url"}
	for i, body := range bodies {
		interactive, _ := body["interactive"].(map[string]any)
		if got, _ := interactive["type"].(string); got != want[i] {
			t.Fatalf("body %d interactive type = %q, want %q: %+v", i, got, want[i], body)
		}
	}
	action, _ := bodies[1]["interactive"].(map[string]any)["action"].(map[string]any)
	if action["catalog_id"] != "cat-1" || action["product_retailer_id"] != "sku-1" {
		t.Fatalf("product action = %+v", action)
	}
}

func TestEmbeddedSignupStartCallbackSelect(t *testing.T) {
	svc, cfg, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/oauth/access_token"):
			_, _ = w.Write([]byte(`{"access_token":"embedded-token","expires_in":3600}`))
		case strings.HasSuffix(r.URL.Path, "/me"):
			_, _ = w.Write([]byte(`{"id":"business-1"}`))
		case strings.HasSuffix(r.URL.Path, "/owned_whatsapp_business_accounts"):
			_, _ = w.Write([]byte(`{"data":[{"id":"waba-9","name":"My WABA"}]}`))
		case strings.HasSuffix(r.URL.Path, "/phone_numbers"):
			_, _ = w.Write([]byte(`{"data":[{"id":"phone-9","display_phone_number":"+551199","verified_name":"Shop"}]}`))
		case strings.HasSuffix(r.URL.Path, "/subscribed_apps"):
			_, _ = w.Write([]byte(`{"success":true}`))
		case strings.HasSuffix(r.URL.Path, "/phone-9"):
			_, _ = w.Write([]byte(`{"display_phone_number":"+551199","verified_name":"Shop","quality_rating":"GREEN","status":"CONNECTED"}`))
		default:
			t.Logf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()
	cfg.CloudAPIEmbeddedConfigID = "config-1"
	cfg.CloudAPIEmbeddedRedirectURI = "https://wamux.test/cloud/embedded/callback"
	inst := seedCloudCredentials(t, svc)

	start, err := svc.StartEmbeddedSignup(inst)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(start.URL, "config_id=config-1") || start.State == "" {
		t.Fatalf("start = %+v", start)
	}
	assets, err := svc.CompleteEmbeddedSignup("code-1", start.State)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 1 || assets[0].PhoneNumberID != "phone-9" || assets[0].WABAID != "waba-9" {
		t.Fatalf("assets = %+v", assets)
	}
	// A second callback for the same state must not expose a token or mutate
	// state silently; the code was already exchanged.
	if _, err := svc.CompleteEmbeddedSignup("code-1", "wrong-state"); err == nil {
		t.Fatal("expected invalid state error")
	}
	status, err := svc.SelectEmbeddedAsset(inst, start.State, "waba-9", "phone-9")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Subscribed {
		t.Fatalf("status = %+v", status)
	}
	creds, err := svc.repo.GetCredentials(inst.Id)
	if err != nil || creds.WABAID != "waba-9" || creds.PhoneNumberID != "phone-9" || creds.BusinessID != "business-1" {
		t.Fatalf("creds = %+v err=%v", creds, err)
	}
	if _, err := svc.EmbeddedAssets(start.State); err == nil {
		t.Fatal("state should be consumed after select")
	}
}

func TestBusinessPermissionErrors(t *testing.T) {
	for _, code := range []int{10, 200} {
		e := MapGraphError(&GraphError{Code: code, StatusCode: http.StatusForbidden, Message: "permission"})
		if e.Status != http.StatusForbidden || e.Code != "cloud_permission_denied" {
			t.Fatalf("code %d => %+v", code, e)
		}
	}
}

func TestMemoryPhoneLimiterHonorsRetryAfter(t *testing.T) {
	lim := NewMemoryPhoneLimiter(1000)
	lim.Observe("phone-pause", 429, 100*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := lim.Wait(ctx, "phone-pause"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait err = %v, want deadline exceeded", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := lim.Wait(context.Background(), "phone-pause"); err != nil {
		t.Fatal(err)
	}
}

func TestSignedUserIDUpdateWebhookMergesIdentity(t *testing.T) {
	svc, _, srv := newCloudTestService(t, func(w http.ResponseWriter, r *http.Request) {})
	defer srv.Close()
	inst := seedCloudCredentials(t, svc)
	if err := svc.repo.UpsertContact(&cloud_model.Contact{
		InstanceID: inst.Id, ContactID: "bsuid:OLD.1", BSUID: "OLD.1", WAID: "5511777777777",
	}); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"object":"whatsapp_business_account","entry":[{"id":"waba-1","changes":[{"field":"user_id_update","value":{"wa_id":"5511777777777","old_user_id":"OLD.1","new_user_id":"NEW.1","timestamp":` + strconv.FormatInt(time.Now().Unix(), 10) + `}}]}]}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	if err := svc.ReceiveWebhook(body, "sha256="+hex.EncodeToString(mac.Sum(nil)), ""); err != nil {
		t.Fatal(err)
	}
	contact, err := svc.repo.FindContactByBSUID(inst.Id, "NEW.1")
	if err != nil || contact.ContactID != "bsuid:OLD.1" {
		t.Fatalf("contact = %+v err=%v", contact, err)
	}
}
