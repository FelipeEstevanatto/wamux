package message_service

import (
	"strings"
	"testing"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
)

func TestDownloadName(t *testing.T) {
	const when = "2026-11-06 00:06:12"
	for name, tc := range map[string]struct {
		msg  message_model.Message
		want string
	}{
		"photo": {
			message_model.Message{MessageType: "image", MediaMimetype: "image/jpeg", Timestamp: when},
			"WaMux Image 2026-11-06 at 00.06.12.jpg",
		},
		"video": {
			message_model.Message{MessageType: "video", MediaMimetype: "video/mp4", Timestamp: when},
			"WaMux Video 2026-11-06 at 00.06.12.mp4",
		},
		"voice note with a codec in its type": {
			message_model.Message{MessageType: "audio", MediaMimetype: "audio/ogg; codecs=opus", Timestamp: when},
			"WaMux Audio 2026-11-06 at 00.06.12.ogg",
		},
		"sticker": {
			message_model.Message{MessageType: "sticker", MediaMimetype: "image/webp", Timestamp: when},
			"WaMux Sticker 2026-11-06 at 00.06.12.webp",
		},
		"document keeps its name": {
			message_model.Message{FileName: "report.final.pdf", MediaMimetype: "application/pdf", Timestamp: when},
			"report.final.pdf",
		},
		"document missing its extension": {
			message_model.Message{FileName: "report", MediaMimetype: "application/pdf", Timestamp: when},
			"report.pdf",
		},
		"nothing known": {
			message_model.Message{},
			"WaMux File",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := downloadName(&tc.msg); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A name WaMux makes up must be valid on Windows too.
func TestDownloadNameHasNoForbiddenCharacters(t *testing.T) {
	got := downloadName(&message_model.Message{MessageType: "image", MediaMimetype: "image/png", Timestamp: "2026-11-06 00:06:12"})

	if strings.ContainsAny(got, `:*?"<>|`) {
		t.Fatalf("got %q", got)
	}
}
