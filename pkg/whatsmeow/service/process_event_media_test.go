package whatsmeow_service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"go.mau.fi/whatsmeow/util/gcmutil"
	"go.mau.fi/whatsmeow/util/hkdfutil"
	"google.golang.org/protobuf/proto"

	localmedia "github.com/felipeestevanatto/wamux/pkg/media"
	poll_model "github.com/felipeestevanatto/wamux/pkg/poll/model"
	poll_service "github.com/felipeestevanatto/wamux/pkg/poll/service"
)

type recordingStorage struct {
	err    error
	stored []byte
	name   string
	mime   string
}

func (s *recordingStorage) Store(_ context.Context, data []byte, name, mime string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	s.stored, s.name, s.mime = data, name, mime
	return "https://cdn.example/" + name, nil
}

func (s *recordingStorage) StoreReader(context.Context, io.Reader, int64, string, string) (string, error) {
	return "", errors.New("not expected: the file is small")
}

func (s *recordingStorage) Delete(context.Context, string) error { return nil }

func (s *recordingStorage) GetURL(context.Context, string) (string, error) { return "", nil }

func mediaHarness(t *testing.T, file []byte) *eventHarness {
	t.Helper()
	h := newEventHarness(t)
	h.cli.config.WebhookFiles = true
	h.cli.mediaDownloader = func(context.Context, whatsmeow.DownloadableMessage) ([]byte, error) {
		return file, nil
	}
	return h
}

func mediaMessage(t *testing.T, message *waE2E.Message) *events.Message {
	t.Helper()
	evt := inbound(t, "MSG1", "")
	evt.Message = message
	return evt
}

func TestDownloadedMediaIsEmbeddedAsBase64(t *testing.T) {
	cases := []struct {
		name    string
		message *waE2E.Message
		mime    string
	}{
		{"image", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, "image/jpeg"},
		{"audio", &waE2E.Message{AudioMessage: &waE2E.AudioMessage{}}, "audio/ogg"},
		{"video", &waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}, "video/mp4"},
		{"document", &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{Mimetype: proto.String("application/pdf")}}, "application/pdf"},
		{"reply image", &waE2E.Message{AssociatedChildMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}},
		}}, "image/jpeg"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := mediaHarness(t, []byte("the file"))
			got := h.one(t, mediaMessage(t, tc.message))
			wantField(t, got, base64.StdEncoding.EncodeToString([]byte("the file")), "data", "Message", "base64")
			if m := h.messages.stored()[0]; m.MediaMimetype != tc.mime {
				t.Errorf("stored mimetype %q, want %q", m.MediaMimetype, tc.mime)
			}
		})
	}
}

func TestDownloadFailureKeepsTheEventAndSkipsStorage(t *testing.T) {
	h := mediaHarness(t, nil)
	h.cli.mediaDownloader = func(context.Context, whatsmeow.DownloadableMessage) ([]byte, error) {
		return nil, errors.New("gone")
	}
	storage := &recordingStorage{}
	h.svc.storage, h.svc.deliveryMode = storage, "s3"
	got := h.one(t, mediaMessage(t, &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}))
	wantField(t, got, "MSG1", "data", "Info", "ID")
	if storage.stored != nil {
		t.Error("stored something after a failed download")
	}
}

func TestMediaGoesToObjectStorage(t *testing.T) {
	cases := []struct {
		mode       string
		wantBase64 bool
	}{{"s3", false}, {"both", true}}
	for _, tc := range cases {
		t.Run(tc.mode, func(t *testing.T) {
			h := mediaHarness(t, []byte("the file"))
			storage := &recordingStorage{}
			h.svc.storage, h.svc.deliveryMode = storage, tc.mode
			got := h.one(t, mediaMessage(t, &waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}))

			if string(storage.stored) != "the file" || storage.name != "MSG1.mp4" || storage.mime != "video/mp4" {
				t.Errorf("stored %q as %s (%s)", storage.stored, storage.name, storage.mime)
			}
			wantField(t, got, "https://cdn.example/MSG1.mp4", "data", "Message", "mediaUrl")
			if (field(got, "data", "Message", "base64") != nil) != tc.wantBase64 {
				t.Errorf("base64 present = %v, want %v", field(got, "data", "Message", "base64") != nil, tc.wantBase64)
			}
			if m := h.messages.stored()[0]; m.MediaUrl != "https://cdn.example/MSG1.mp4" {
				t.Errorf("stored url %q", m.MediaUrl)
			}
		})
	}
}

func TestStorageFailureStillDeliversTheMessage(t *testing.T) {
	h := mediaHarness(t, []byte("the file"))
	h.svc.storage, h.svc.deliveryMode = &recordingStorage{err: errors.New("bucket down")}, "both"
	got := h.one(t, mediaMessage(t, &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}))
	if field(got, "data", "Message", "mediaUrl") != nil {
		t.Error("mediaUrl set after a failed upload")
	}
	if field(got, "data", "Message", "base64") == nil {
		t.Error("base64 lost with the upload")
	}
}

func TestLocalCopyIsUsedWithoutObjectStorage(t *testing.T) {
	dir := t.TempDir()
	if err := localmedia.Configure(dir); err != nil {
		t.Fatal(err)
	}
	h := mediaHarness(t, []byte("the file"))
	h.cli.config.MediaLocalStore = true
	got := h.one(t, mediaMessage(t, &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}))

	wantField(t, got, localmedia.URLPath("MSG1"), "data", "Message", "mediaUrl")
	if m := h.messages.stored()[0]; m.MediaUrl != localmedia.URLPath("MSG1") {
		t.Errorf("stored url %q", m.MediaUrl)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*", "*"))
	if err != nil || len(files) != 1 {
		t.Fatalf("files = %v, %v", files, err)
	}
	if data, _ := os.ReadFile(files[0]); string(data) != "the file" {
		t.Errorf("saved %q", data)
	}
}

// A file that declared no size can turn out to be over the limit.
func TestUndeclaredLargeFileIsNotEmbedded(t *testing.T) {
	h := mediaHarness(t, bytes.Repeat([]byte{1}, 2048))
	h.cli.config.WebhookBase64MaxBytes = 1024
	got := h.one(t, mediaMessage(t, &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}))
	wantField(t, got, true, "data", "Message", "base64Omitted")
	if field(got, "data", "Message", "base64") != nil {
		t.Error("base64 present over the limit")
	}
}

func TestUndecodableStickerIsKeptAsWebp(t *testing.T) {
	h := mediaHarness(t, []byte("not a webp"))
	h.one(t, mediaMessage(t, &waE2E.Message{StickerMessage: &waE2E.StickerMessage{}}))
	if m := h.messages.stored()[0]; m.MediaMimetype != "image/webp" {
		t.Errorf("mimetype %q", m.MediaMimetype)
	}
}

func TestAutoReadUsesTheJIDsWhatsAppAddressed(t *testing.T) {
	previous := autoReadDelay
	autoReadDelay = time.Millisecond
	defer func() { autoReadDelay = previous }()

	type read struct {
		ids          []types.MessageID
		chat, sender types.JID
	}
	h := newEventHarness(t)
	h.cli.Instance.ReadMessages = true
	reads := make(chan read, 2)
	h.cli.readMarker = func(_ context.Context, ids []types.MessageID, _ time.Time, chat, sender types.JID) error {
		reads <- read{ids, chat, sender}
		return nil
	}

	evt := inbound(t, "MSG1", "hi")
	lid := mustJID(t, testContactLID)
	evt.Info.Chat, evt.Info.Sender = lid, lid
	evt.Info.SenderAlt = mustJID(t, testContact)
	got := h.one(t, evt)
	wantField(t, got, testContact, "data", "Info", "Chat") // swapped for the payload

	select {
	case r := <-reads:
		if len(r.ids) != 1 || r.ids[0] != "MSG1" || r.chat != lid || r.sender != lid {
			t.Fatalf("read %+v, want the original LID", r)
		}
	case <-testTimeout():
		t.Fatal("message was not marked read")
	}
}

func TestAutoReadSkipsOwnAndIgnoredMessages(t *testing.T) {
	previous := autoReadDelay
	autoReadDelay = time.Millisecond
	defer func() { autoReadDelay = previous }()

	cases := map[string]func(*eventHarness, *events.Message){
		"off":     func(h *eventHarness, _ *events.Message) { h.cli.Instance.ReadMessages = false },
		"own":     func(_ *eventHarness, e *events.Message) { e.Info.IsFromMe = true },
		"ignored": func(h *eventHarness, e *events.Message) { h.cli.Instance.IgnoreGroups = true; inGroup(t, e) },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			h := newEventHarness(t)
			h.cli.Instance.ReadMessages = true
			read := make(chan struct{}, 1)
			h.cli.readMarker = func(context.Context, []types.MessageID, time.Time, types.JID, types.JID) error {
				read <- struct{}{}
				return nil
			}
			evt := inbound(t, "MSG1", "hi")
			setup(h, evt)
			h.cli.processEvent(evt)
			select {
			case <-read:
				t.Fatal("marked read")
			case <-time.After(30 * time.Millisecond):
			}
		})
	}
}

func TestQRCodesRotateAndThenTimeOut(t *testing.T) {
	first, next := qrFirstCodeLifetime, qrNextCodeLifetime
	qrFirstCodeLifetime, qrNextCodeLifetime = time.Millisecond, time.Millisecond
	defer func() { qrFirstCodeLifetime, qrNextCodeLifetime = first, next }()

	h := newEventHarness(t)
	h.cli.WAClient.Store.ID = nil
	h.cli.config.LogType = "json"
	h.cli.processEvent(&events.QR{Codes: []string{"code-1", "code-2"}})

	ds := h.deliveries(t, 3)
	var seen []string
	for _, d := range ds {
		e := d.event(t)
		if e["event"] == "QRCode" {
			seen = append(seen, field(e, "data", "code").(string))
		}
	}
	if len(seen) != 2 {
		t.Fatalf("codes = %v", seen)
	}
	byEvent(t, ds, "QRTimeout")
	if !h.cli.kill.Fired() {
		t.Error("kill switch not fired")
	}
	if n := len(h.instances.qrcodes); n != 3 || h.instances.qrcodes[n-1] != "" {
		t.Errorf("qrcode updates = %d, last must clear it", n)
	}
}

// An active passkey ceremony owns the socket, so QR rotation must leave it alone.
func TestQRRotationYieldsToAPasskeyCeremony(t *testing.T) {
	h := newEventHarness(t)
	h.cli.WAClient.Store.ID = nil
	h.cli.passkeyCeremony.Start(testInstanceID, []byte(`{}`))
	h.none(t, &events.QR{Codes: []string{"code-1"}})
	if h.cli.kill.Fired() {
		t.Error("kill switch fired during a ceremony")
	}
}

// memorySecrets holds the secret of the one poll under test.
type memorySecrets struct {
	store.MsgSecretStore
	secret []byte
	sender types.JID
}

func (m memorySecrets) GetMessageSecret(context.Context, types.JID, types.JID, types.MessageID) ([]byte, types.JID, error) {
	return m.secret, m.sender, nil
}

type recordingPolls struct {
	poll_service.PollService
	votes chan *poll_model.PollVote
}

func (r recordingPolls) SavePollVote(_ context.Context, vote *poll_model.PollVote) error {
	r.votes <- vote
	return nil
}

// pollVote builds a genuinely encrypted vote, so this runs the real decryption.
func pollVote(t *testing.T, h *eventHarness) *events.Message {
	t.Helper()
	own := mustJID(t, "5511999990000@s.whatsapp.net")
	h.cli.WAClient.Store.MsgSecrets = memorySecrets{secret: bytes.Repeat([]byte{7}, 32), sender: own}
	h.cli.clientPointer.Set(testInstanceID, h.cli.WAClient)

	contact := mustJID(t, testContact)
	message, err := h.cli.WAClient.BuildPollVote(context.Background(), &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: contact, Sender: own, IsFromMe: true},
		ID:            "POLL1",
	}, []string{"Yes"})
	if err != nil {
		t.Fatal(err)
	}

	evt := inbound(t, "VOTE1", "")
	evt.Message = message
	evt.Info.Sender, evt.Info.IsFromMe = mustJID(t, testOwnJID), true
	return evt
}

func TestPollVoteIsDecryptedAndSaved(t *testing.T) {
	h := newEventHarness(t)
	polls := recordingPolls{votes: make(chan *poll_model.PollVote, 1)}
	h.cli.pollService = polls
	h.one(t, pollVote(t, h))

	select {
	case vote := <-polls.votes:
		if vote.PollMessageID != "POLL1" || vote.InstanceID != testInstanceID {
			t.Errorf("vote %+v", vote)
		}
		if len(vote.SelectedOptions) != 1 {
			t.Errorf("selected %v", vote.SelectedOptions)
		}
		if vote.VoterJid != "5511999990000@s.whatsapp.net" {
			t.Errorf("voter %q must lose its device suffix", vote.VoterJid)
		}
	case <-testTimeout():
		t.Fatal("vote was not saved")
	}
}

// A vote that cannot be decrypted is still a message event.
func TestUndecryptableVoteStillDelivers(t *testing.T) {
	h := newEventHarness(t)
	polls := recordingPolls{votes: make(chan *poll_model.PollVote, 1)}
	h.cli.pollService = polls
	evt := pollVote(t, h)
	h.cli.WAClient.Store.MsgSecrets = memorySecrets{secret: bytes.Repeat([]byte{9}, 32), sender: evt.Info.Sender}
	h.one(t, evt)
	select {
	case vote := <-polls.votes:
		t.Fatalf("saved %+v", vote)
	case <-time.After(30 * time.Millisecond):
	}
}

// sealedEdit encrypts an edit the way WhatsApp does: the key depends on the sender
// as addressed on the wire, which for most contacts is now a LID.
func sealedEdit(t *testing.T, secret []byte, originalID string, sender types.JID, text string) *waE2E.Message {
	t.Helper()
	// The shape a phone really sends: the sealed content is itself the edit.
	target := &waCommon.MessageKey{RemoteJID: proto.String(sender.String()), ID: proto.String(originalID)}
	plaintext, err := proto.Marshal(&waE2E.Message{
		MessageContextInfo: &waE2E.MessageContextInfo{MessageSecret: []byte("secret")},
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
			Key:  target,
			EditedMessage: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: proto.String(text),
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Mirrors whatsmeow's generateMsgSecretKey for a message edit.
	useCase := []byte(originalID + sender.String() + sender.String() + string(whatsmeow.EncSecretMessageEdit))
	iv := bytes.Repeat([]byte{3}, 12)
	payload, err := gcmutil.Encrypt(hkdfutil.SHA256(secret, nil, useCase, 32), iv, plaintext, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &waE2E.Message{SecretEncryptedMessage: &waE2E.SecretEncryptedMessage{
		SecretEncType:    waE2E.SecretEncryptedMessage_MESSAGE_EDIT.Enum(),
		TargetMessageKey: target,
		EncPayload:       payload,
		EncIV:            iv,
	}}
}

// Found with a real phone: the edit was decrypted after the LID was swapped for the
// phone number, which changes the key, so every edit reached consumers as a blob.
func TestEditFromALIDContactIsDecrypted(t *testing.T) {
	h := newEventHarness(t)
	lid := mustJID(t, testContactLID)
	secret := bytes.Repeat([]byte{7}, 32)
	h.cli.WAClient.Store.MsgSecrets = memorySecrets{secret: secret, sender: lid}
	h.cli.clientPointer.Set(testInstanceID, h.cli.WAClient)

	evt := inbound(t, "EDIT1", "")
	evt.Info.Chat, evt.Info.Sender = lid, lid
	evt.Info.SenderAlt = mustJID(t, testContact)
	evt.Message = sealedEdit(t, secret, "ORIGINAL", lid, "the new text")

	got := h.one(t, evt)
	wantField(t, got, true, "data", "IsEdit")
	wantField(t, got, "edit", "data", "messageType")
	wantField(t, got, "the new text", "data", "Message", "protocolMessage", "editedMessage", "extendedTextMessage", "text")
	if field(got, "data", "Message", "messageContextInfo") != nil {
		t.Error("the message secret reached the payload")
	}
	wantField(t, got, testContact, "data", "Info", "Chat")

	m := h.messages.stored()[0]
	if m.MessageType != "edit" || m.TextContent != "the new text" || m.QuotedMessageID != "ORIGINAL" {
		t.Errorf("stored %+v", m)
	}
}
