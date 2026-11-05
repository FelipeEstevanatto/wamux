package content

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// A document sent with a caption stays wrapped when the send path records it.
func TestSummarizeUnwrapsCaptionedDocument(t *testing.T) {
	got := Summarize(&waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
		Message: &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{Caption: proto.String("the report")}},
	}})
	if got.Type != "document" || got.Text != "the report" {
		t.Fatalf("got %+v", got)
	}
}

func TestSummarizeEmptyWrapperIsUnknown(t *testing.T) {
	got := Summarize(&waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{}})
	if got.Type != "unknown" {
		t.Fatalf("got %+v", got)
	}
}

func TestSummarizeButtons(t *testing.T) {
	cases := []struct {
		name string
		msg  *waE2E.Message
		kind string
		text string
	}{
		{"tapped template button", &waE2E.Message{TemplateButtonReplyMessage: &waE2E.TemplateButtonReplyMessage{
			SelectedDisplayText: proto.String("Tap me"),
		}}, "buttons_response", "Tap me"},
		{"sent buttons", &waE2E.Message{ButtonsMessage: &waE2E.ButtonsMessage{ContentText: proto.String("Pick")}}, "buttons", "Pick"},
		{"sent list", &waE2E.Message{ListMessage: &waE2E.ListMessage{Description: proto.String("Pick")}}, "list", "Pick"},
		{"sent interactive", &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
			Body: &waE2E.InteractiveMessage_Body{Text: proto.String("Pick")},
		}}, "interactive", "Pick"},
		{"wrapped interactive", &waE2E.Message{ViewOnceMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{
				Body: &waE2E.InteractiveMessage_Body{Text: proto.String("Pick")},
			}},
		}}, "interactive", "Pick"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Summarize(tc.msg); got.Type != tc.kind || got.Text != tc.text {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestSummarizeEditedCaption(t *testing.T) {
	got := Summarize(&waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
		EditedMessage: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("new caption")}},
	}})
	if got.Type != "edit" || got.Text != "new caption" {
		t.Fatalf("got %+v", got)
	}
}
