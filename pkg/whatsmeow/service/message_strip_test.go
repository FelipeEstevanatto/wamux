package whatsmeow_service

import (
	"encoding/json"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func benchMessage() *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			ID:            "3EB0D28D11FECCB4A6CB20",
			MessageSource: types.MessageSource{Chat: types.NewJID("5514999999999", types.DefaultUserServer)},
		},
		Message: &waE2E.Message{
			ImageMessage: &waE2E.ImageMessage{
				Caption:       ptr("hello"),
				JPEGThumbnail: make([]byte, 10*1024),
				MediaKey:      make([]byte, 32),
			},
		},
	}
}

func ptr[T any](v T) *T { return &v }

// The payload already drops these thumbnails, so stripping them early must not
// change what is emitted — and must not touch the sticker thumbnail, which the
// payload keeps.
func TestStripMessageThumbnails(t *testing.T) {
	msg := &waE2E.Message{
		ImageMessage:    &waE2E.ImageMessage{JPEGThumbnail: []byte("img")},
		VideoMessage:    &waE2E.VideoMessage{JPEGThumbnail: []byte("vid")},
		DocumentMessage: &waE2E.DocumentMessage{JPEGThumbnail: []byte("doc")},
		StickerMessage:  &waE2E.StickerMessage{PngThumbnail: []byte("sticker")},
	}

	stripMessageThumbnails(msg)

	if msg.GetImageMessage().GetJPEGThumbnail() != nil {
		t.Error("image thumbnail not stripped")
	}
	if msg.GetVideoMessage().GetJPEGThumbnail() != nil {
		t.Error("video thumbnail not stripped")
	}
	if msg.GetDocumentMessage().GetJPEGThumbnail() != nil {
		t.Error("document thumbnail not stripped")
	}
	if string(msg.GetStickerMessage().GetPngThumbnail()) != "sticker" {
		t.Error("sticker thumbnail must be preserved (the payload keeps it)")
	}
}

func TestStripMessageThumbnailsNilSafe(t *testing.T) {
	stripMessageThumbnails(nil)
	stripMessageThumbnails(&waE2E.Message{})
}

// BenchmarkMessageMapRoundTripWithThumbnail models the old inbound path: the
// whole event (thumbnail included) is marshalled and unmarshalled to a map.
func BenchmarkMessageMapRoundTripWithThumbnail(b *testing.B) {
	evt := benchMessage()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		jsonBytes, err := json.Marshal(evt)
		if err != nil {
			b.Fatal(err)
		}
		var m map[string]interface{}
		if err := json.Unmarshal(jsonBytes, &m); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMessageMapRoundTripStripped models the new inbound path.
func BenchmarkMessageMapRoundTripStripped(b *testing.B) {
	evt := benchMessage()
	stripMessageThumbnails(evt.Message)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		jsonBytes, err := json.Marshal(evt)
		if err != nil {
			b.Fatal(err)
		}
		var m map[string]interface{}
		if err := json.Unmarshal(jsonBytes, &m); err != nil {
			b.Fatal(err)
		}
	}
}
