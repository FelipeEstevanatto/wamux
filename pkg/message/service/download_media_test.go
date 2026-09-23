package message_service

import (
	"errors"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

func TestIsMediaGoneError(t *testing.T) {
	for _, err := range []error{
		whatsmeow.ErrMediaDownloadFailedWith403,
		whatsmeow.ErrMediaDownloadFailedWith404,
		whatsmeow.ErrMediaDownloadFailedWith410,
	} {
		if !isMediaGoneError(err) {
			t.Fatalf("%v should be treated as media-gone", err)
		}
	}
	if isMediaGoneError(errors.New("boom")) {
		t.Fatal("a generic error is not media-gone")
	}
}

func TestMediaKeyOf(t *testing.T) {
	key := []byte{1, 2, 3}
	msg := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{MediaKey: key}}
	if got := mediaKeyOf(msg); string(got) != string(key) {
		t.Fatalf("mediaKeyOf = %v, want %v", got, key)
	}
	if mediaKeyOf(&waE2E.Message{}) != nil {
		t.Fatal("a message without media should return a nil key")
	}
}

func TestDownloadMediaStructMessageInfo(t *testing.T) {
	d := &DownloadMediaStruct{
		Id:          "ABC",
		Chat:        "5511999999999",
		FromMe:      false,
		IsGroup:     true,
		Participant: "5511888888888",
	}
	info := d.messageInfo()
	if info == nil {
		t.Fatal("expected a MessageInfo")
	}
	if info.ID != "ABC" || !info.IsGroup || info.IsFromMe {
		t.Fatalf("unexpected info: %+v", info)
	}
	if info.Chat.String() != "5511999999999@s.whatsapp.net" {
		t.Fatalf("chat = %s", info.Chat)
	}
	if info.Sender.String() != "5511888888888@s.whatsapp.net" {
		t.Fatalf("sender = %s", info.Sender)
	}

	// Without the context the retry cannot be requested.
	if (&DownloadMediaStruct{Id: "ABC"}).messageInfo() != nil {
		t.Fatal("missing chat should return nil")
	}
	if (&DownloadMediaStruct{Chat: "5511999999999"}).messageInfo() != nil {
		t.Fatal("missing id should return nil")
	}
}
