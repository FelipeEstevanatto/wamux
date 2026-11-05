package whatsmeow_service

import (
	"encoding/json"
	"errors"
	"testing"

	"go.mau.fi/whatsmeow/types"

	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	"github.com/felipeestevanatto/wamux/pkg/safemap"
)

type recordingProducer struct {
	queues []string
	err    error
}

func (p *recordingProducer) Produce(queue string, _ []byte, _ string, _ string) error {
	p.queues = append(p.queues, queue)
	return p.err
}

func (p *recordingProducer) CreateGlobalQueues() error { return nil }

// signingProducer stands in for the HTTP webhook producer.
type signingProducer struct {
	recordingProducer
	keys []string
}

func (p *signingProducer) ProduceSigned(queue string, _ []byte, _ string, _ string, key []byte) error {
	p.queues = append(p.queues, queue)
	p.keys = append(p.keys, string(key))
	return p.err
}

type routingFixture struct {
	svc                       *whatsmeowService
	rabbit, nats, ws, webhook *recordingProducer
}

func newRoutingFixture(t *testing.T) *routingFixture {
	t.Helper()
	logs := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	t.Cleanup(func() { _ = logs.GetLogger(testInstanceID).Close() })
	f := &routingFixture{
		rabbit: &recordingProducer{}, nats: &recordingProducer{},
		ws: &recordingProducer{}, webhook: &recordingProducer{},
	}
	f.svc = &whatsmeowService{
		config:            &config.Config{},
		loggerWrapper:     logs,
		webhookHmacKeys:   safemap.New[string](),
		rabbitmqProducer:  f.rabbit,
		natsProducer:      f.nats,
		websocketProducer: f.ws,
		webhookProducer:   f.webhook,
	}
	return f
}

func routedPayload(t *testing.T, event string, data map[string]any) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"event": event, "data": data})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// delivered reports whether an instance subscribed to events gets this event.
func delivered(t *testing.T, events, event string, data map[string]any) bool {
	t.Helper()
	f := newRoutingFixture(t)
	instance := &instance_model.Instance{Id: testInstanceID, Events: events, Webhook: "https://hook.example"}
	f.svc.CallWebhook(instance, "q", routedPayload(t, event, data))
	return len(f.webhook.queues) == 1
}

// Every event processEvent or the send path emits, with the subscription that
// must receive it. Adding an event means adding it here.
var eventSubscriptions = map[string][]string{
	"Message":                 {"MESSAGE"},
	"SendMessage":             {"SEND_MESSAGE"},
	"Receipt":                 {"READ_RECEIPT"},
	"Presence":                {"PRESENCE"},
	"HistorySync":             {"HISTORY_SYNC"},
	"ChatPresence":            {"CHAT_PRESENCE"},
	"Archive":                 {"CHAT_PRESENCE"},
	"CallOffer":               {"CALL"},
	"CallAccept":              {"CALL"},
	"CallTerminate":           {"CALL"},
	"CallOfferNotice":         {"CALL"},
	"CallRelayLatency":        {"CALL"},
	"Connected":               {"CONNECTION"},
	"PairSuccess":             {"CONNECTION"},
	"TemporaryBan":            {"CONNECTION"},
	"LoggedOut":               {"CONNECTION"},
	"ConnectFailure":          {"CONNECTION"},
	"Disconnected":            {"CONNECTION"},
	"AccountReachoutTimelock": {"CONNECTION"},
	"PrivacySettings":         {"CONNECTION"},
	"Blocklist":               {"CONNECTION"},
	"LabelEdit":               {"LABEL"},
	"LabelAssociationChat":    {"LABEL"},
	"LabelAssociationMessage": {"LABEL"},
	"Contact":                 {"CONTACT"},
	"PushName":                {"CONTACT"},
	"Picture":                 {"PICTURE"},
	"UserAbout":               {"USER_ABOUT"},
	"GroupInfo":               {"GROUP"},
	"JoinedGroup":             {"GROUP"},
	"NewsletterJoin":          {"NEWSLETTER"},
	"NewsletterLeave":         {"NEWSLETTER"},
	"NewsletterLiveUpdate":    {"NEWSLETTER"},
	"NewsletterMuteChange":    {"NEWSLETTER"},
	"QRCode":                  {"QRCODE"},
	"QRTimeout":               {"QRCODE"},
	"QRSuccess":               {"QRCODE"},
	"PasskeyRequest":          {"PASSKEY", "QRCODE"},
	"PasskeyConfirmation":     {"PASSKEY", "QRCODE"},
	"PasskeyError":            {"PASSKEY", "QRCODE"},
	"PairError":               {"PASSKEY", "QRCODE"},
	"ButtonClick":             {"BUTTON_CLICK", "MESSAGE"},
}

func TestEventsReachTheirSubscribers(t *testing.T) {
	for event, subscriptions := range eventSubscriptions {
		t.Run(event, func(t *testing.T) {
			for _, subscription := range subscriptions {
				if !delivered(t, subscription, event, nil) {
					t.Errorf("not delivered to %s", subscription)
				}
			}
			if !delivered(t, "ALL", event, nil) {
				t.Error("not delivered to ALL")
			}
		})
	}
}

func TestEventsSkipOtherSubscribers(t *testing.T) {
	for event, subscriptions := range eventSubscriptions {
		t.Run(event, func(t *testing.T) {
			wanted := map[string]bool{}
			for _, s := range subscriptions {
				wanted[s] = true
			}
			for _, other := range []string{"MESSAGE", "PRESENCE", "CALL", "LABEL", "CONTACT", "PICTURE"} {
				if !wanted[other] && delivered(t, other, event, nil) {
					t.Errorf("leaked to %s", other)
				}
			}
		})
	}
}

// A group or newsletter subscriber gets that chat's traffic without subscribing
// to all messages. Receipts carry the chat at the top of data, messages under Info.
func TestGroupAndNewsletterFallback(t *testing.T) {
	info := func(chat string) map[string]any { return map[string]any{"Info": map[string]any{"Chat": chat}} }
	top := func(chat string) map[string]any { return map[string]any{"Chat": chat} }

	cases := []struct {
		name         string
		subscription string
		event        string
		data         map[string]any
		want         bool
	}{
		{"message to group", "GROUP", "Message", info(testGroup), true},
		{"message to newsletter", "NEWSLETTER", "Message", info("1@newsletter"), true},
		{"direct message to group subscriber", "GROUP", "Message", info(testContact), false},
		{"group message to newsletter subscriber", "NEWSLETTER", "Message", info(testGroup), false},
		{"sent to group", "GROUP", "SendMessage", info(testGroup), true},
		{"sent to newsletter", "NEWSLETTER", "SendMessage", info("1@newsletter"), true},
		{"sent direct", "GROUP", "SendMessage", info(testContact), false},
		{"group receipt", "GROUP", "Receipt", top(testGroup), true},
		{"newsletter receipt", "NEWSLETTER", "Receipt", top("1@newsletter"), true},
		{"direct receipt", "GROUP", "Receipt", top(testContact), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := delivered(t, tc.subscription, tc.event, tc.data); got != tc.want {
				t.Errorf("delivered = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSubscriptionListParsing(t *testing.T) {
	if !delivered(t, "PRESENCE,MESSAGE", "Message", nil) {
		t.Error("second subscription in a list was not honored")
	}
	if !delivered(t, "NOT_AN_EVENT,MESSAGE", "Message", nil) {
		t.Error("an invalid entry stopped a valid one")
	}
}

// An instance that chose nothing gets the defaults, and only those.
func TestEmptySubscriptionsUseTheDefaults(t *testing.T) {
	for _, events := range []string{"", "  "} {
		if !delivered(t, events, "Message", nil) {
			t.Errorf("%q: Message not delivered", events)
		}
		if !delivered(t, events, "SendMessage", nil) {
			t.Errorf("%q: SendMessage not delivered", events)
		}
		if delivered(t, events, "Presence", nil) {
			t.Errorf("%q: Presence delivered", events)
		}
	}
}

// Subscribing only to invalid names is a choice that matched nothing, not an
// empty list, so it gets no defaults.
func TestOnlyInvalidSubscriptionsDeliverNothing(t *testing.T) {
	if delivered(t, "NOT_AN_EVENT", "Message", nil) {
		t.Error("delivered")
	}
}

// Pinned so a change is deliberate: names must match exactly, so
// "PRESENCE, MESSAGE" and "message" are discarded, and OfflineSyncCompleted has no
// case in CallWebhook, so only ALL subscribers get it.
func TestSubscriptionGaps(t *testing.T) {
	cases := []struct{ name, events, event string }{
		{"spaces are not trimmed", "PRESENCE, MESSAGE", "Message"},
		{"lower case is discarded", "message", "Message"},
		{"offline sync has no subscription", "CONNECTION", "OfflineSyncCompleted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if delivered(t, tc.events, tc.event, nil) {
				t.Fatal("delivered: behaviour changed, update this test")
			}
		})
	}
}

func TestUnroutablePayloadsAreDropped(t *testing.T) {
	for name, payload := range map[string]string{
		"not json":      `nope`,
		"no event name": `{"data":{}}`,
		"unknown event": `{"event":"Mystery"}`,
	} {
		t.Run(name, func(t *testing.T) {
			f := newRoutingFixture(t)
			instance := &instance_model.Instance{Id: testInstanceID, Events: "MESSAGE", Webhook: "https://hook.example"}
			f.svc.CallWebhook(instance, "q", []byte(payload))
			if len(f.webhook.queues) != 0 {
				t.Errorf("delivered %v", f.webhook.queues)
			}
		})
	}
}

func TestSinksAreChosenByInstanceSettings(t *testing.T) {
	cases := []struct {
		name                          string
		instance                      instance_model.Instance
		rabbit, nats, ws, webhookWant int
	}{
		{name: "nothing enabled"},
		{name: "webhook disabled", instance: instance_model.Instance{Webhook: "disabled"}},
		{name: "webhook", instance: instance_model.Instance{Webhook: "https://hook.example"}, webhookWant: 1},
		{name: "rabbit", instance: instance_model.Instance{RabbitmqEnable: "enabled"}, rabbit: 1},
		{name: "nats", instance: instance_model.Instance{NatsEnable: "true"}, nats: 1},
		{name: "websocket", instance: instance_model.Instance{WebSocketEnable: "enabled"}, ws: 1},
		{name: "unknown value", instance: instance_model.Instance{RabbitmqEnable: "yes"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRoutingFixture(t)
			instance := tc.instance
			instance.Id = testInstanceID
			f.svc.sendToQueueOrWebhook(&instance, "q", []byte(`{}`))
			got := []int{len(f.rabbit.queues), len(f.nats.queues), len(f.ws.queues), len(f.webhook.queues)}
			want := []int{tc.rabbit, tc.nats, tc.ws, tc.webhookWant}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("rabbit/nats/ws/webhook = %v, want %v", got, want)
				}
			}
		})
	}
}

func TestOneFailingSinkDoesNotStopTheOthers(t *testing.T) {
	f := newRoutingFixture(t)
	f.rabbit.err = errors.New("rabbit down")
	f.nats.err = errors.New("nats down")
	f.ws.err = errors.New("no subscribers")
	f.svc.sendToQueueOrWebhook(&instance_model.Instance{
		Id: testInstanceID, RabbitmqEnable: "enabled", NatsEnable: "enabled",
		WebSocketEnable: "enabled", Webhook: "https://hook.example",
	}, "q", []byte(`{}`))
	if len(f.webhook.queues) != 1 {
		t.Fatal("webhook skipped after other sinks failed")
	}
}

func TestWebhookIsSignedWhenAKeyExists(t *testing.T) {
	f := newRoutingFixture(t)
	signer := &signingProducer{}
	f.svc.webhookProducer = signer
	instance := &instance_model.Instance{Id: testInstanceID, Webhook: "https://hook.example"}

	f.svc.sendToQueueOrWebhook(instance, "q", []byte(`{}`))
	if len(signer.queues) != 1 || len(signer.keys) != 0 {
		t.Fatalf("without a key: queues=%v keys=%v", signer.queues, signer.keys)
	}

	f.svc.SetWebhookHmacKey(testInstanceID, "signing-key")
	f.svc.sendToQueueOrWebhook(instance, "q", []byte(`{}`))
	if len(signer.keys) != 1 || signer.keys[0] != "signing-key" {
		t.Fatalf("keys = %v", signer.keys)
	}
}

// End to end: what processEvent emits must survive CallWebhook's own parsing, or
// the fallback routing above reads nothing.
func TestProcessedGroupMessageRoutesToGroupSubscriber(t *testing.T) {
	h := newEventHarness(t)
	h.cli.processEvent(inGroup(t, inbound(t, "MSG1", "hi")))
	d := h.deliveries(t, 1)[0]

	f := newRoutingFixture(t)
	f.svc.CallWebhook(&instance_model.Instance{
		Id: testInstanceID, Events: "GROUP", Webhook: "https://hook.example",
	}, d.queue, d.payload)
	if len(f.webhook.queues) != 1 {
		t.Fatal("a real group message did not reach a GROUP subscriber")
	}
}

func TestProcessedGroupReceiptRoutesToGroupSubscriber(t *testing.T) {
	h := newEventHarness(t)
	r := receipt(t, types.ReceiptTypeRead, "A")
	r.Chat = mustJID(t, testGroup)
	h.cli.processEvent(r)
	d := h.deliveries(t, 1)[0]

	f := newRoutingFixture(t)
	f.svc.CallWebhook(&instance_model.Instance{
		Id: testInstanceID, Events: "GROUP", Webhook: "https://hook.example",
	}, d.queue, d.payload)
	if len(f.webhook.queues) != 1 {
		t.Fatal("a real group receipt did not reach a GROUP subscriber")
	}
}

// WEBHOOK_URL is meant to get every instance's events, and the instance needs no
// webhook of its own. "disabled" opts out.
func TestGlobalWebhookReachesInstancesWithoutOne(t *testing.T) {
	cases := []struct {
		name    string
		webhook string
		global  string
		want    int
	}{
		{"no webhook anywhere", "", "", 0},
		{"global only", "", "https://global.example", 1},
		{"instance and global", "https://hook.example", "https://global.example", 1},
		{"disabled keeps global out", "disabled", "https://global.example", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRoutingFixture(t)
			f.svc.config.WebhookUrl = tc.global
			f.svc.sendToQueueOrWebhook(&instance_model.Instance{Id: testInstanceID, Webhook: tc.webhook}, "q", []byte(`{}`))
			if got := len(f.webhook.queues); got != tc.want {
				t.Fatalf("producer called %d times, want %d", got, tc.want)
			}
		})
	}
}
