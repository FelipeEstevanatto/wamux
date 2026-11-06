package content

import (
	"bytes"
	"testing"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestSummarizeText(t *testing.T) {
	plain := Summarize(&waE2E.Message{Conversation: proto.String("hi there")})
	if plain.Type != "text" || plain.Text != "hi there" || plain.QuotedID != "" {
		t.Fatalf("conversation = %+v", plain)
	}

	ext := Summarize(&waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text:        proto.String("a reply"),
		ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("orig-id")},
	}})
	if ext.Type != "text" || ext.Text != "a reply" || ext.QuotedID != "orig-id" {
		t.Fatalf("extended text = %+v", ext)
	}
}

func TestSummarizeMediaCaptions(t *testing.T) {
	cases := []struct {
		name string
		msg  *waE2E.Message
		want string
		text string
	}{
		{"image", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("pic")}}, "image", "pic"},
		{"video", &waE2E.Message{VideoMessage: &waE2E.VideoMessage{Caption: proto.String("vid")}}, "video", "vid"},
		{"document", &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{Caption: proto.String("doc")}}, "document", "doc"},
		{"audio", &waE2E.Message{AudioMessage: &waE2E.AudioMessage{}}, "audio", ""},
		{"sticker", &waE2E.Message{StickerMessage: &waE2E.StickerMessage{}}, "sticker", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Summarize(tc.msg)
			if got.Type != tc.want || got.Text != tc.text {
				t.Fatalf("got %+v, want type %q text %q", got, tc.want, tc.text)
			}
		})
	}
}

func TestSummarizeLocationAndContact(t *testing.T) {
	loc := Summarize(&waE2E.Message{LocationMessage: &waE2E.LocationMessage{Name: proto.String("Paris")}})
	if loc.Type != "location" || loc.Text != "Paris" {
		t.Fatalf("location = %+v", loc)
	}

	contact := Summarize(&waE2E.Message{ContactMessage: &waE2E.ContactMessage{DisplayName: proto.String("Jane")}})
	if contact.Type != "contact" || contact.Text != "Jane" {
		t.Fatalf("contact = %+v", contact)
	}
}

func TestSummarizeReactionAndQuotedTarget(t *testing.T) {
	msg := &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{
		Text: proto.String("👍"),
		Key:  &waCommon.MessageKey{ID: proto.String("reacted-to")},
	}}
	got := Summarize(msg)
	if got.Type != "reaction" || got.Text != "👍" || got.QuotedID != "reacted-to" {
		t.Fatalf("reaction = %+v", got)
	}
}

func TestSummarizeDeleteAndEdit(t *testing.T) {
	revoke := Summarize(&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(),
	}})
	if revoke.Type != "delete" {
		t.Fatalf("revoke = %+v, want delete", revoke)
	}

	edit := Summarize(&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		Key:           &waCommon.MessageKey{ID: proto.String("target-id")},
		EditedMessage: &waE2E.Message{Conversation: proto.String("fixed typo")},
	}})
	if edit.Type != "edit" || edit.Text != "fixed typo" || edit.QuotedID != "target-id" {
		t.Fatalf("edit = %+v", edit)
	}

	// An edit whose replacement is an extended-text message.
	editExt := Summarize(&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		EditedMessage: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("ext edit")}},
	}})
	if editExt.Text != "ext edit" {
		t.Fatalf("extended edit text = %q", editExt.Text)
	}
}

func TestSummarizeInteractiveResponses(t *testing.T) {
	btn := Summarize(&waE2E.Message{ButtonsResponseMessage: &waE2E.ButtonsResponseMessage{
		Response: &waE2E.ButtonsResponseMessage_SelectedDisplayText{SelectedDisplayText: "Yes"},
	}})
	if btn.Type != "buttons_response" || btn.Text != "Yes" {
		t.Fatalf("buttons response = %+v", btn)
	}

	list := Summarize(&waE2E.Message{ListResponseMessage: &waE2E.ListResponseMessage{
		Title: proto.String("Option A"),
	}})
	if list.Type != "list_response" || list.Text != "Option A" {
		t.Fatalf("list response = %+v", list)
	}
}

func TestSummarizeNilAndUnknown(t *testing.T) {
	if got := Summarize(nil); got.Type != "unknown" {
		t.Fatalf("nil = %+v, want unknown", got)
	}
	if got := Summarize(&waE2E.Message{}); got.Type != "unknown" {
		t.Fatalf("empty = %+v, want unknown", got)
	}
}

func TestSummarizePollEventAndProduct(t *testing.T) {
	question := proto.String("Lunch?")
	for name, tc := range map[string]struct {
		msg  *waE2E.Message
		want Summary
	}{
		"poll":     {&waE2E.Message{PollCreationMessage: &waE2E.PollCreationMessage{Name: question}}, Summary{Type: "poll", Text: "Lunch?"}},
		"poll v3":  {&waE2E.Message{PollCreationMessageV3: &waE2E.PollCreationMessage{Name: question}}, Summary{Type: "poll", Text: "Lunch?"}},
		"event":    {&waE2E.Message{EventMessage: &waE2E.EventMessage{Name: proto.String("Sales")}}, Summary{Type: "event", Text: "Sales"}},
		"product":  {&waE2E.Message{ProductMessage: &waE2E.ProductMessage{Product: &waE2E.ProductMessage_ProductSnapshot{Title: proto.String("Shirt")}}}, Summary{Type: "product", Text: "Shirt"}},
		"no title": {&waE2E.Message{ProductMessage: &waE2E.ProductMessage{}}, Summary{Type: "product"}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Summarize(tc.msg); got.Type != tc.want.Type || got.Text != tc.want.Text {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSummarizeFileDetails(t *testing.T) {
	size := proto.Uint64(2048)
	forwarded := &waE2E.ContextInfo{IsForwarded: proto.Bool(true)}
	document := &waE2E.DocumentMessage{FileName: proto.String("a.pdf"), FileLength: size, ContextInfo: forwarded}

	for name, tc := range map[string]struct {
		msg       *waE2E.Message
		file      string
		size      int64
		forwarded bool
	}{
		"document": {&waE2E.Message{DocumentMessage: document}, "a.pdf", 2048, true},
		"captioned document": {
			&waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{DocumentMessage: document}}},
			"a.pdf", 2048, true,
		},
		"image":          {&waE2E.Message{ImageMessage: &waE2E.ImageMessage{FileLength: size}}, "", 2048, false},
		"video":          {&waE2E.Message{VideoMessage: &waE2E.VideoMessage{FileLength: size, ContextInfo: forwarded}}, "", 2048, true},
		"audio":          {&waE2E.Message{AudioMessage: &waE2E.AudioMessage{FileLength: size}}, "", 2048, false},
		"forwarded text": {&waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("x"), ContextInfo: forwarded}}, "", 0, true},
		"plain text":     {&waE2E.Message{Conversation: proto.String("x")}, "", 0, false},
		"location":       {&waE2E.Message{LocationMessage: &waE2E.LocationMessage{ContextInfo: forwarded}}, "", 0, true},
	} {
		t.Run(name, func(t *testing.T) {
			got := Summarize(tc.msg)
			if got.FileName != tc.file || got.FileSize != tc.size || got.Forwarded != tc.forwarded {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestSummarizeDocumentPreview(t *testing.T) {
	preview := []byte{0xff, 0xd8, 0xff}
	document := &waE2E.DocumentMessage{JPEGThumbnail: preview, PageCount: proto.Uint32(12)}

	for name, msg := range map[string]*waE2E.Message{
		"document": {DocumentMessage: document},
		"captioned": {DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{DocumentMessage: document},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			got := Summarize(msg)
			if got.PageCount != 12 || !bytes.Equal(got.Thumbnail, preview) {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestSummarizeVideoPreview(t *testing.T) {
	preview := []byte{0xff, 0xd8}
	got := Summarize(&waE2E.Message{VideoMessage: &waE2E.VideoMessage{JPEGThumbnail: preview}})

	if !bytes.Equal(got.Thumbnail, preview) {
		t.Fatalf("got %+v", got)
	}
}

// It lives in the row and in every history page, so a huge one is dropped, not stored.
func TestSummarizeDropsAnOversizedPreview(t *testing.T) {
	got := Summarize(&waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
		JPEGThumbnail: make([]byte, maxThumbnailBytes+1),
	}})

	if got.Thumbnail != nil {
		t.Fatalf("kept %d bytes", len(got.Thumbnail))
	}
}
