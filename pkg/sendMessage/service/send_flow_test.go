package send_service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"

	config "github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
	"github.com/felipeestevanatto/wamux/pkg/safemap"
	"github.com/felipeestevanatto/wamux/pkg/utils"
	whatsmeow_service "github.com/felipeestevanatto/wamux/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
)

const (
	testInstanceID = "inst"
	testNumber     = "5511999990001"
)

var testRecipient = types.NewJID(testNumber, types.DefaultUserServer)

type sentMessage struct {
	to    types.JID
	msg   *waE2E.Message
	extra whatsmeow.SendRequestExtra
}

// fakeWhatsmeow records the event a send emits.
type fakeWhatsmeow struct {
	whatsmeow_service.WhatsmeowService
	events chan []byte
}

func (f *fakeWhatsmeow) CallWebhook(_ *instance_model.Instance, _ string, payload []byte) {
	f.events <- payload
}

type fakeRepo struct {
	message_repository.MessageRepository
	saved chan message_model.Message
}

func (f *fakeRepo) InsertMessage(m message_model.Message) error {
	f.saved <- m
	return nil
}

// sendHarness runs the real send logic, with only the WhatsApp calls replaced.
type sendHarness struct {
	*sendService
	instance  *instance_model.Instance
	wa        *fakeWhatsmeow
	repo      *fakeRepo
	mu        sync.Mutex
	sent      []sentMessage
	uploads   []whatsmeow.MediaType
	presences []types.ChatPresence
	sendErr   func(attempt int) error
}

func newSendHarness(t *testing.T) *sendHarness {
	t.Helper()
	cfg := &config.Config{
		LogDirectory:          t.TempDir(),
		DatabaseSaveMessages:  true,
		WebhookBase64MaxBytes: 1 << 20,
		MaxUploadBytes:        1 << 30,
	}
	logs := logger_wrapper.NewLoggerManager(cfg)
	t.Cleanup(func() { _ = logs.GetLogger(testInstanceID).Close() })

	own := types.NewJID("5511999990000", types.DefaultUserServer)
	clients := safemap.New[*whatsmeow.Client]()
	clients.Set(testInstanceID, whatsmeow.NewClient(&store.Device{ID: &own}, nil))

	wa := &fakeWhatsmeow{events: make(chan []byte, 8)}
	repo := &fakeRepo{saved: make(chan message_model.Message, 8)}
	h := &sendHarness{
		sendService: NewSendService(clients, wa, cfg, logs, repo, nil).(*sendService),
		instance:    &instance_model.Instance{Id: testInstanceID, Name: "One", Token: "instance-token"},
		wa:          wa,
		repo:        repo,
	}
	h.isConnected = func(*whatsmeow.Client) bool { return true }
	h.deliver = func(_ context.Context, _ *whatsmeow.Client, to types.JID, msg *waE2E.Message, extra whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.sent = append(h.sent, sentMessage{to, msg, extra})
		if h.sendErr != nil {
			if err := h.sendErr(len(h.sent)); err != nil {
				return whatsmeow.SendResponse{}, err
			}
		}
		return whatsmeow.SendResponse{ServerID: 7}, nil
	}
	h.upload = func(_ context.Context, _ *whatsmeow.Client, data []byte, kind whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.uploads = append(h.uploads, kind)
		return whatsmeow.UploadResponse{
			URL:           "https://mmg.test/file",
			DirectPath:    "/v/file",
			MediaKey:      []byte{1, 2, 3},
			FileEncSHA256: []byte{4},
			FileSHA256:    []byte{5},
			FileLength:    uint64(len(data)),
		}, nil
	}
	h.chatPresence = func(_ context.Context, _ *whatsmeow.Client, _ types.JID, state types.ChatPresence, _ types.ChatPresenceMedia) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.presences = append(h.presences, state)
		return nil
	}
	return h
}

func (h *sendHarness) only(t *testing.T) sentMessage {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(h.sent))
	}
	return h.sent[0]
}

func (h *sendHarness) event(t *testing.T) map[string]any {
	t.Helper()
	select {
	case raw := <-h.wa.events:
		var event map[string]any
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("event is not JSON: %v", err)
		}
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("no event was emitted")
		return nil
	}
}

func (h *sendHarness) savedRow(t *testing.T) message_model.Message {
	t.Helper()
	select {
	case row := <-h.repo.saved:
		return row
	case <-time.After(2 * time.Second):
		t.Fatal("nothing was saved")
		return message_model.Message{}
	}
}

func raw() *bool {
	format := false
	return &format
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestSendTextDeliversEmitsAndStores(t *testing.T) {
	h := newSendHarness(t)

	got, err := h.SendText(&TextStruct{Number: testNumber, Text: "hello", FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}

	sent := h.only(t)
	if sent.to != testRecipient {
		t.Errorf("sent to %s, want %s", sent.to, testRecipient)
	}
	if sent.msg.GetExtendedTextMessage().GetText() != "hello" {
		t.Errorf("text = %q", sent.msg.GetExtendedTextMessage().GetText())
	}
	if sent.extra.ID == "" || sent.extra.ID != got.Info.ID {
		t.Errorf("extra ID %q, info ID %q", sent.extra.ID, got.Info.ID)
	}
	if !got.Info.IsFromMe || got.Info.ServerID != 7 {
		t.Errorf("info = %+v", got.Info)
	}

	event := h.event(t)
	if event["event"] != "SendMessage" || event["instanceId"] != testInstanceID {
		t.Errorf("event = %v", event)
	}

	row := h.savedRow(t)
	if row.MessageID != got.Info.ID || row.TextContent != "hello" || row.Status != "Sent" || !row.IsFromMe {
		t.Errorf("saved row = %+v", row)
	}
	if row.ChatJid != testRecipient.String() {
		t.Errorf("saved chat = %q", row.ChatJid)
	}
}

func TestSendKeepsTheCallersMessageID(t *testing.T) {
	h := newSendHarness(t)

	if _, err := h.SendText(&TextStruct{Number: testNumber, Text: "x", Id: "CUSTOM", FormatJid: raw()}, h.instance); err != nil {
		t.Fatal(err)
	}

	if id := h.only(t).extra.ID; id != "CUSTOM" {
		t.Fatalf("ID = %q", id)
	}
}

func TestSendRejectsAnInvalidNumber(t *testing.T) {
	h := newSendHarness(t)

	if _, err := h.SendText(&TextStruct{Number: "not a number@bad", Text: "x", FormatJid: raw()}, h.instance); err == nil {
		t.Fatal("want an error")
	}
	if len(h.sent) != 0 {
		t.Fatal("sent anyway")
	}
}

func TestSendAddsQuoteContext(t *testing.T) {
	h := newSendHarness(t)
	quoted := QuotedStruct{MessageID: "ORIGINAL", Participant: testRecipient.String()}

	if _, err := h.SendText(&TextStruct{Number: testNumber, Text: "reply", Quoted: quoted, FormatJid: raw()}, h.instance); err != nil {
		t.Fatal(err)
	}

	info := h.only(t).msg.GetExtendedTextMessage().GetContextInfo()
	if info.GetStanzaID() != "ORIGINAL" || info.GetParticipant() != testRecipient.String() {
		t.Fatalf("context = %+v", info)
	}
}

func TestSendMarksForwardedMessages(t *testing.T) {
	h := newSendHarness(t)
	score := uint32(4)

	if _, err := h.SendText(&TextStruct{Number: testNumber, Text: "x", ForwardingScore: &score, FormatJid: raw()}, h.instance); err != nil {
		t.Fatal(err)
	}

	info := h.only(t).msg.GetExtendedTextMessage().GetContextInfo()
	if info.GetForwardingScore() != 4 || !info.GetIsForwarded() {
		t.Fatalf("context = %+v", info)
	}
}

func TestSendShowsTypingBeforeADelayedMessage(t *testing.T) {
	h := newSendHarness(t)

	if _, err := h.SendText(&TextStruct{Number: testNumber, Text: "x", Delay: 1, FormatJid: raw()}, h.instance); err != nil {
		t.Fatal(err)
	}

	if len(h.presences) != 2 || h.presences[0] != types.ChatPresenceComposing || h.presences[1] != types.ChatPresencePaused {
		t.Fatalf("presences = %v", h.presences)
	}
}

func TestSendCarriesTheChatsDisappearingTimer(t *testing.T) {
	h := newSendHarness(t)
	whatsmeow_service.SetCachedChatEphemeral(testInstanceID, testRecipient, 86400)
	t.Cleanup(func() { whatsmeow_service.SetCachedChatEphemeral(testInstanceID, testRecipient, 0) })

	if _, err := h.SendText(&TextStruct{Number: testNumber, Text: "x", FormatJid: raw()}, h.instance); err != nil {
		t.Fatal(err)
	}

	if got := h.only(t).msg.GetExtendedTextMessage().GetContextInfo().GetExpiration(); got != 86400 {
		t.Fatalf("expiration = %d", got)
	}
}

func TestSendRetriesOnlyAfterADisconnect(t *testing.T) {
	t.Run("disconnect is retried", func(t *testing.T) {
		h := newSendHarness(t)
		h.sendErr = func(attempt int) error {
			if attempt == 1 {
				return errors.New("client disconnected")
			}
			return nil
		}

		if _, err := h.sendTextWithRetry(&TextStruct{Number: testNumber, Text: "x", FormatJid: raw()}, h.instance, 2); err != nil {
			t.Fatalf("send: %v", err)
		}
		if len(h.sent) != 2 {
			t.Fatalf("attempts = %d, want 2", len(h.sent))
		}
	})

	t.Run("other errors fail at once", func(t *testing.T) {
		h := newSendHarness(t)
		h.sendErr = func(int) error { return errors.New("server said no") }

		_, err := h.sendTextWithRetry(&TextStruct{Number: testNumber, Text: "x", FormatJid: raw()}, h.instance, 3)
		if err == nil || !strings.Contains(err.Error(), "server said no") {
			t.Fatalf("err = %v", err)
		}
		if len(h.sent) != 1 {
			t.Fatalf("attempts = %d, want 1", len(h.sent))
		}
	})
}

func TestSendFailureEmitsAndStoresNothing(t *testing.T) {
	h := newSendHarness(t)
	h.sendErr = func(int) error { return errors.New("server said no") }

	if _, err := h.SendText(&TextStruct{Number: testNumber, Text: "x", FormatJid: raw()}, h.instance); err == nil {
		t.Fatal("want an error")
	}

	select {
	case <-h.wa.events:
		t.Fatal("emitted an event for a failed send")
	case <-h.repo.saved:
		t.Fatal("stored a failed send")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSendNeedsAConnectedClient(t *testing.T) {
	h := newSendHarness(t)
	h.isConnected = func(*whatsmeow.Client) bool { return false }

	_, err := h.ensureClientConnected(testInstanceID)
	if err == nil || !strings.Contains(err.Error(), "client disconnected") {
		t.Fatalf("err = %v", err)
	}
}

func TestSendImageFile(t *testing.T) {
	h := newSendHarness(t)
	file := testPNG(t)

	got, err := h.SendMediaFile(&MediaStruct{Number: testNumber, Type: "image", Caption: "cap", FormatJid: raw()}, file, h.instance)
	if err != nil {
		t.Fatalf("SendMediaFile: %v", err)
	}

	if len(h.uploads) != 1 || h.uploads[0] != whatsmeow.MediaImage {
		t.Fatalf("uploads = %v", h.uploads)
	}
	image := h.only(t).msg.GetImageMessage()
	if image.GetCaption() != "cap" || image.GetMimetype() != "image/png" || image.GetURL() != "https://mmg.test/file" {
		t.Errorf("image = %+v", image)
	}
	if !bytes.Equal(image.GetMediaKey(), []byte{1, 2, 3}) || image.GetFileLength() != uint64(len(file)) {
		t.Errorf("key %v, length %d", image.GetMediaKey(), image.GetFileLength())
	}
	if image.GetWidth() != 8 || image.GetHeight() != 6 || len(image.GetJPEGThumbnail()) == 0 {
		t.Errorf("width %d, height %d, thumbnail %d bytes", image.GetWidth(), image.GetHeight(), len(image.GetJPEGThumbnail()))
	}
	if got.MediaMimetype != "image/png" {
		t.Errorf("MediaMimetype = %q", got.MediaMimetype)
	}
}

func TestSendDocumentFile(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendMediaFile(&MediaStruct{Number: testNumber, Type: "document", Filename: "notes.txt", Caption: "read", FormatJid: raw()}, []byte("plain text"), h.instance)
	if err != nil {
		t.Fatalf("SendMediaFile: %v", err)
	}

	if h.uploads[0] != whatsmeow.MediaDocument {
		t.Fatalf("uploads = %v", h.uploads)
	}
	msg := h.only(t).msg
	doc := msg.GetDocumentMessage()
	if doc == nil {
		doc = msg.GetDocumentWithCaptionMessage().GetMessage().GetDocumentMessage()
	}
	if doc.GetFileName() != "notes.txt" {
		t.Fatalf("document = %+v", doc)
	}
}

func TestSendRejectsAFileOfTheWrongType(t *testing.T) {
	for name, kind := range map[string]string{"as image": "image", "as video": "video"} {
		t.Run(name, func(t *testing.T) {
			h := newSendHarness(t)

			_, err := h.SendMediaFile(&MediaStruct{Number: testNumber, Type: kind, FormatJid: raw()}, []byte("plain text"), h.instance)
			if err == nil || !strings.Contains(err.Error(), "Invalid file format") {
				t.Fatalf("err = %v", err)
			}
			if len(h.uploads) != 0 || len(h.sent) != 0 {
				t.Fatalf("uploaded %d, sent %d", len(h.uploads), len(h.sent))
			}
		})
	}
}

func TestSendMediaEventCarriesBase64UnderTheLimit(t *testing.T) {
	file := testPNG(t)
	message := func(t *testing.T, limit int64) map[string]any {
		h := newSendHarness(t)
		h.config.WebhookFiles = true
		h.config.WebhookBase64MaxBytes = limit
		if _, err := h.SendMediaFile(&MediaStruct{Number: testNumber, Type: "image", FormatJid: raw()}, file, h.instance); err != nil {
			t.Fatal(err)
		}
		return h.event(t)["data"].(map[string]any)["Message"].(map[string]any)
	}

	if got := message(t, 1<<20); got["base64"] == nil || got["base64"] == "" {
		t.Errorf("under the limit: base64 = %v", got["base64"])
	}
	over := message(t, 1)
	if over["base64"] != nil || over["base64Omitted"] != true {
		t.Errorf("over the limit: %v", over)
	}
}

func TestSendPoll(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendPoll(&PollStruct{Number: testNumber, Question: "Lunch?", MaxAnswer: 2, Options: []string{"Yes", "No"}, FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatalf("SendPoll: %v", err)
	}

	msg := h.only(t).msg
	poll := msg.GetPollCreationMessage()
	if poll == nil {
		poll = msg.GetPollCreationMessageV3()
	}
	if poll.GetName() != "Lunch?" || len(poll.GetOptions()) != 2 || poll.GetSelectableOptionsCount() != 2 {
		t.Fatalf("poll = %+v", poll)
	}
}

func TestSendLocation(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendLocation(&LocationStruct{Number: testNumber, Name: "Office", Address: "Main St", Latitude: -16.5, Longitude: -49.25, FormatJid: raw()}, h.instance)
	if err != nil {
		t.Fatalf("SendLocation: %v", err)
	}

	location := h.only(t).msg.GetLocationMessage()
	if location.GetName() != "Office" || location.GetDegreesLatitude() != -16.5 || location.GetDegreesLongitude() != -49.25 {
		t.Fatalf("location = %+v", location)
	}
}

func TestSendContact(t *testing.T) {
	h := newSendHarness(t)

	_, err := h.SendContact(&ContactStruct{
		Number:    testNumber,
		Vcard:     utils.VCardStruct{FullName: "Ana Test", Phone: "5511999990002"},
		FormatJid: raw(),
	}, h.instance)
	if err != nil {
		t.Fatalf("SendContact: %v", err)
	}

	contact := h.only(t).msg.GetContactMessage()
	if contact.GetDisplayName() != "Ana Test" || !strings.Contains(contact.GetVcard(), "5511999990002") {
		t.Fatalf("contact = %+v", contact)
	}
}
