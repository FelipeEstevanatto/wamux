package message_service

import (
	"errors"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func TestReactionAuthor(t *testing.T) {
	chat := types.NewJID("5511999999999", types.DefaultUserServer)
	group := types.NewJID("12345", types.GroupServer)

	// our own message -> empty author (BuildMessageKey keeps FromMe=true)
	if a, ok := reactionAuthor(true, false, "", chat); !ok || !a.IsEmpty() {
		t.Fatalf("fromMe -> (%v, %v), want empty author", a, ok)
	}
	// 1:1 from the other party -> the chat is the author
	if a, ok := reactionAuthor(false, false, "", chat); !ok || a != chat {
		t.Fatalf("1:1 -> (%v, %v), want the chat", a, ok)
	}
	// group with a participant -> the (canonical) participant
	a, ok := reactionAuthor(false, true, "+5511888888888", group)
	if !ok || a.User != "5511888888888" || a.Server != types.DefaultUserServer {
		t.Fatalf("group participant -> (%v, %v)", a, ok)
	}
	// group without a participant -> unknown; the caller keeps FromMe=false
	if a, ok := reactionAuthor(false, true, "", group); ok || !a.IsEmpty() {
		t.Fatalf("group without participant -> (%v, %v), want unknown", a, ok)
	}
}

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
