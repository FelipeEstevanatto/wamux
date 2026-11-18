package send_service

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func TestSendAlbumSendsContainerAndItems(t *testing.T) {
	h := newSendHarness(t)
	item := base64.StdEncoding.EncodeToString(testPNG(t))

	result, err := h.SendAlbum(&AlbumStruct{
		Number:    testNumber,
		FormatJid: raw(),
		Items: []AlbumItem{
			{Mediatype: "image", Media: item, Caption: "one"},
			{Mediatype: "image", Media: item, Caption: "two"},
		},
	}, h.instance)
	if err != nil {
		t.Fatalf("SendAlbum: %v", err)
	}

	if result.Album.ExpectedImageCount != 2 || result.Album.ExpectedVideoCount != 0 {
		t.Fatalf("album counts = %+v", result.Album)
	}
	if len(result.Items) != 2 {
		t.Fatalf("result has %d items, want 2", len(result.Items))
	}

	h.mu.Lock()
	sent := append([]sentMessage(nil), h.sent...)
	h.mu.Unlock()

	if len(sent) != 3 {
		t.Fatalf("sent %d messages, want container + 2 items", len(sent))
	}
	if sent[0].msg.GetAlbumMessage() == nil {
		t.Fatalf("first message is %T, want AlbumMessage", sent[0].msg)
	}
	if got := sent[0].msg.GetAlbumMessage().GetExpectedImageCount(); got != 2 {
		t.Fatalf("container image count = %d, want 2", got)
	}
	for i, wantCaption := range []string{"one", "two"} {
		msg := sent[i+1].msg
		if msg.GetImageMessage() == nil {
			t.Fatalf("item %d is not an image", i)
		}
		if got := msg.GetImageMessage().GetCaption(); got != wantCaption {
			t.Fatalf("item %d caption = %q, want %q", i, got, wantCaption)
		}
		assoc := msg.GetMessageContextInfo().GetMessageAssociation()
		if assoc == nil {
			t.Fatalf("item %d has no album association", i)
		}
		if assoc.GetAssociationType() != waE2E.MessageAssociation_MEDIA_ALBUM {
			t.Fatalf("item %d association = %v", i, assoc.GetAssociationType())
		}
		if got := assoc.GetParentMessageKey().GetID(); got != result.Album.ID {
			t.Fatalf("item %d parent = %q, want %q", i, got, result.Album.ID)
		}
		if got := assoc.GetMessageIndex(); got != int32(i) {
			t.Fatalf("item %d index = %d", i, got)
		}
	}

	// Two history rows, one per item; the container is metadata only.
	var rows int
	for {
		select {
		case <-h.repo.saved:
			rows++
			continue
		case <-time.After(200 * time.Millisecond):
		}
		break
	}
	if rows != 2 {
		t.Fatalf("saved %d rows, want 2", rows)
	}
}

func TestSendAlbumValidatesItems(t *testing.T) {
	h := newSendHarness(t)
	item := base64.StdEncoding.EncodeToString(testPNG(t))

	cases := []struct {
		name string
		data *AlbumStruct
	}{
		{"one item", &AlbumStruct{Number: testNumber, Items: []AlbumItem{{Mediatype: "image", Media: item}}}},
		{"too many", &AlbumStruct{Number: testNumber, Items: make([]AlbumItem, 31)}},
		{"bad type", &AlbumStruct{Number: testNumber, Items: []AlbumItem{{Mediatype: "audio", Media: item}, {Mediatype: "image", Media: item}}}},
		{"no media", &AlbumStruct{Number: testNumber, Items: []AlbumItem{{Mediatype: "image"}, {Mediatype: "image", Media: item}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := h.SendAlbum(tc.data, h.instance); err == nil {
				t.Fatal("expected an error")
			}
		})
	}

	if _, err := h.SendAlbum(&AlbumStruct{Number: testNumber, Items: []AlbumItem{
		{Mediatype: "image", Media: "not base64!"},
		{Mediatype: "image", Media: item},
	}}, h.instance); err == nil || !strings.Contains(err.Error(), "base64") {
		t.Fatalf("invalid base64 error = %v", err)
	}
	h.mu.Lock()
	sent := len(h.sent)
	h.mu.Unlock()
	if sent != 0 {
		t.Fatalf("invalid media sent %d messages", sent)
	}
}

func TestSendAlbumFetchesURLItemsBeforeSending(t *testing.T) {
	h := newSendHarness(t)
	png := testPNG(t)
	server := mediaServer(t, map[string][]byte{"/a.png": png, "/b.png": png})

	result, err := h.SendAlbum(&AlbumStruct{
		Number:    testNumber,
		FormatJid: raw(),
		Items: []AlbumItem{
			{Mediatype: "image", Media: server.URL + "/a.png", Caption: "from-url"},
			{Mediatype: "image", Media: server.URL + "/b.png"},
		},
	}, h.instance)
	if err != nil {
		t.Fatalf("SendAlbum: %v", err)
	}
	h.mu.Lock()
	sent := append([]sentMessage(nil), h.sent...)
	h.mu.Unlock()
	if len(result.Items) != 2 || len(sent) != 3 {
		t.Fatalf("items %d, sent %d", len(result.Items), len(sent))
	}
	if got := sent[1].msg.GetImageMessage().GetCaption(); got != "from-url" {
		t.Fatalf("caption = %q", got)
	}
	if sent[1].msg.GetImageMessage().GetMimetype() != "image/png" {
		t.Fatalf("mimetype = %q", sent[1].msg.GetImageMessage().GetMimetype())
	}
}

func TestSendAlbumDoesNotSendWhenAURLItemFails(t *testing.T) {
	h := newSendHarness(t)
	server := mediaServer(t, map[string][]byte{"/a.png": testPNG(t)})

	_, err := h.SendAlbum(&AlbumStruct{
		Number:    testNumber,
		FormatJid: raw(),
		Items: []AlbumItem{
			{Mediatype: "image", Media: server.URL + "/a.png"},
			{Mediatype: "image", Media: server.URL + "/missing.png"},
		},
	}, h.instance)
	if err == nil || !strings.Contains(err.Error(), "album item 1") {
		t.Fatalf("err = %v", err)
	}
	h.mu.Lock()
	sent := len(h.sent)
	h.mu.Unlock()
	if sent != 0 {
		t.Fatalf("sent %d messages before the album was complete", sent)
	}
}

func TestSendAlbumRevokesWhenALaterItemFails(t *testing.T) {
	h := newSendHarness(t)
	h.sendErr = func(attempt int) error {
		if attempt == 3 {
			return errors.New("whatsapp rejected item")
		}
		return nil
	}
	item := base64.StdEncoding.EncodeToString(testPNG(t))

	result, err := h.SendAlbum(&AlbumStruct{
		Number: testNumber, FormatJid: raw(),
		Items: []AlbumItem{
			{Mediatype: "image", Media: item},
			{Mediatype: "image", Media: item},
		},
	}, h.instance)
	if err == nil || result != nil || !strings.Contains(err.Error(), "album item 1") {
		t.Fatalf("result=%v err=%v", result, err)
	}

	h.mu.Lock()
	sent := append([]sentMessage(nil), h.sent...)
	h.mu.Unlock()
	var revoked []string
	for _, msg := range sent {
		protoMsg := msg.msg.GetProtocolMessage()
		if protoMsg == nil || protoMsg.GetType() != waE2E.ProtocolMessage_REVOKE {
			continue
		}
		revoked = append(revoked, protoMsg.GetKey().GetID())
	}
	if len(revoked) != 2 {
		t.Fatalf("revoked %v, want the container and the item that was accepted", revoked)
	}
}
