package whatsmeow_service

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/felipeestevanatto/wamux/pkg/bgpool"
	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	instance_repository "github.com/felipeestevanatto/wamux/pkg/instance/repository"
	"github.com/felipeestevanatto/wamux/pkg/killswitch"
	label_model "github.com/felipeestevanatto/wamux/pkg/label/model"
	label_repository "github.com/felipeestevanatto/wamux/pkg/label/repository"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
	"github.com/felipeestevanatto/wamux/pkg/passkey/ceremony"
	"github.com/felipeestevanatto/wamux/pkg/safemap"
	storage_interfaces "github.com/felipeestevanatto/wamux/pkg/storage/interfaces"
)

// The harness runs processEvent for real against a whatsmeow client that never
// connects: network calls fail with "not connected", so nothing here can reach
// WhatsApp. Avoid paths that dial anyway (Disconnected on an unpaired device,
// undecryptable IDs starting 66/67): they call Connect.

const (
	testInstanceID = "11111111-2222-3333-4444-555555555555"
	testToken      = "test-instance-token"
	testContact    = "5511999990001@s.whatsapp.net"
	testContactLID = "100000000000001@lid"
	testGroup      = "120363000000000001@g.us"
	testOwnJID     = "5511999990000:7@s.whatsapp.net"
)

var testTime = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

type delivery struct {
	queue   string
	payload []byte
}

// event decodes a delivery so tests can assert on its fields.
func (d delivery) event(t *testing.T) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(d.payload, &out); err != nil {
		t.Fatalf("payload is not JSON: %v\n%s", err, d.payload)
	}
	return out
}

// field walks nested objects, returning nil if any step is missing.
func field(m map[string]any, path ...string) any {
	var cur any = m
	for _, key := range path {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = obj[key]
	}
	return cur
}

type typebotCall struct {
	jid, pushName, text string
	fromMe              bool
}

// fakeService records what processEvent asks of the service. The embedded
// interface is nil, so an unexpected call panics instead of passing silently.
type fakeService struct {
	WhatsmeowService
	webhooks     chan delivery
	globals      chan string
	reconnects   chan string
	mediaRetries chan *events.MediaRetry
	mu           sync.Mutex
	typebot      []typebotCall
	storage      storage_interfaces.MediaStorage
	deliveryMode string
}

func (f *fakeService) CallWebhook(_ *instance_model.Instance, queue string, payload []byte) {
	f.webhooks <- delivery{queue: queue, payload: payload}
}

func (f *fakeService) SendToGlobalQueues(event string, _ []byte, _ string) { f.globals <- event }

func (f *fakeService) ReconnectClient(id string) error {
	f.reconnects <- id
	return nil
}

func (f *fakeService) HandleMediaRetry(_ string, evt *events.MediaRetry) { f.mediaRetries <- evt }

func (f *fakeService) DispatchToTypebot(_ *instance_model.Instance, jid, pushName, text string, fromMe bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.typebot = append(f.typebot, typebotCall{jid, pushName, text, fromMe})
}

func (f *fakeService) MediaStorageFor(*instance_model.Instance) (storage_interfaces.MediaStorage, string) {
	return f.storage, f.deliveryMode
}

func (f *fakeService) typebotCalls() []typebotCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]typebotCall(nil), f.typebot...)
}

type connectedUpdate struct {
	connected bool
	reason    string
}

type fakeInstanceRepo struct {
	instance_repository.InstanceRepository
	mu        sync.Mutex
	connected []connectedUpdate
	qrcodes   []string
	jids      []string
	settings  []map[string]interface{}
}

func (r *fakeInstanceRepo) UpdateConnected(_ string, connected bool, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connected = append(r.connected, connectedUpdate{connected, reason})
	return nil
}

func (r *fakeInstanceRepo) UpdateQrcode(_ string, qr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.qrcodes = append(r.qrcodes, qr)
	return nil
}

func (r *fakeInstanceRepo) UpdateJid(_ string, jid string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jids = append(r.jids, jid)
	return nil
}

func (r *fakeInstanceRepo) UpdateConnectSettings(_ string, updates map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settings = append(r.settings, updates)
	return nil
}

// fakeMessageRepo works inline: the harness has no persist pool, so a message is
// stored by the time processEvent returns.
type fakeMessageRepo struct {
	message_repository.MessageRepository
	mu           sync.Mutex
	messages     []message_model.Message
	history      []message_model.Message
	revoked      []revokedCall
	refuseRevoke bool
}

type revokedCall struct {
	row    message_model.Message
	chats  []string
	direct bool
}

func (r *fakeMessageRepo) InsertMessage(m message_model.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, m)
	return nil
}

func (r *fakeMessageRepo) InsertHistoryMessages(rows []message_model.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.history = append(r.history, rows...)
	return nil
}

func (r *fakeMessageRepo) MarkMessageDeleted(row message_model.Message, chats []string, direct bool) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.revoked = append(r.revoked, revokedCall{row, chats, direct})
	return !r.refuseRevoke, nil
}

func (r *fakeMessageRepo) stored() []message_model.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]message_model.Message(nil), r.messages...)
}

type fakeLabelRepo struct {
	label_repository.LabelRepository
	labels []label_model.Label
}

func (r *fakeLabelRepo) UpsertLabel(l label_model.Label) error {
	r.labels = append(r.labels, l)
	return nil
}

type eventHarness struct {
	cli       *MyClient
	svc       *fakeService
	instances *fakeInstanceRepo
	messages  *fakeMessageRepo
	labels    *fakeLabelRepo
}

func mustJID(t *testing.T, s string) types.JID {
	t.Helper()
	jid, err := types.ParseJID(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return jid
}

// newEventHarness builds a paired client with history saving on.
func newEventHarness(t *testing.T) *eventHarness {
	t.Helper()
	own := mustJID(t, testOwnJID)
	logs := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	// One worker, so a queued job doubles as a barrier: see the cleanup below.
	pool := bgpool.New(1, 64)

	h := &eventHarness{
		svc: &fakeService{
			webhooks:     make(chan delivery, 16),
			globals:      make(chan string, 16),
			reconnects:   make(chan string, 16),
			mediaRetries: make(chan *events.MediaRetry, 4),
			deliveryMode: "base64",
		},
		instances: &fakeInstanceRepo{},
		messages:  &fakeMessageRepo{},
		labels:    &fakeLabelRepo{},
	}
	h.cli = &MyClient{
		service:            h.svc,
		WAClient:           whatsmeow.NewClient(&store.Device{ID: &own, PushName: "Test Bot"}, nil),
		userID:             testInstanceID,
		token:              testToken,
		Instance:           &instance_model.Instance{Id: testInstanceID, Name: "test", Token: testToken},
		instanceRepository: h.instances,
		messageRepository:  h.messages,
		labelRepository:    h.labels,
		clientPointer:      safemap.New[*whatsmeow.Client](),
		kill:               killswitch.New(),
		userInfoCache:      cache.New(time.Minute, time.Minute),
		groupInfoCache:     cache.New(time.Minute, time.Minute),
		processedMessages:  cache.New(time.Minute, time.Minute),
		config:             &config.Config{DatabaseSaveMessages: true, WebhookBase64MaxBytes: 5 << 20},
		loggerWrapper:      logs,
		passkeyCeremony:    ceremony.NewStore(),
		bgPool:             pool,
	}

	t.Cleanup(func() {
		// Background jobs log, so let them finish before the log folder goes.
		done := make(chan struct{})
		pool.Go(func() { close(done) })
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("background jobs did not finish")
		}
		reconnectSucceeded(testInstanceID)
		_ = logs.GetLogger(testInstanceID).Close()
	})
	return h
}

// deliveries runs nothing itself: it waits for exactly want deliveries, then
// makes sure no extra one follows. CallWebhook runs on its own goroutine, so the
// "no more" check has to allow it a moment.
func (h *eventHarness) deliveries(t *testing.T, want int) []delivery {
	t.Helper()
	var got []delivery
	timeout := time.After(2 * time.Second)
	for len(got) < want {
		select {
		case d := <-h.svc.webhooks:
			got = append(got, d)
		case <-timeout:
			t.Fatalf("got %d deliveries, want %d", len(got), want)
		}
	}
	select {
	case d := <-h.svc.webhooks:
		t.Fatalf("unexpected extra delivery on %s: %s", d.queue, d.payload)
	case <-time.After(50 * time.Millisecond):
	}
	return got
}

// one runs evt and returns its single delivery.
func (h *eventHarness) one(t *testing.T, evt interface{}) map[string]any {
	t.Helper()
	h.cli.processEvent(evt)
	return h.deliveries(t, 1)[0].event(t)
}

func (h *eventHarness) none(t *testing.T, evt interface{}) {
	t.Helper()
	h.cli.processEvent(evt)
	h.deliveries(t, 0)
}

func byEvent(t *testing.T, ds []delivery, name string) map[string]any {
	t.Helper()
	for _, d := range ds {
		if e := d.event(t); e["event"] == name {
			return e
		}
	}
	t.Fatalf("no %s delivery among %d", name, len(ds))
	return nil
}

func wantField(t *testing.T, m map[string]any, want any, path ...string) {
	t.Helper()
	if got := field(m, path...); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%v = %v, want %v", path, got, want)
	}
}

func testTimeout() <-chan time.Time { return time.After(2 * time.Second) }
