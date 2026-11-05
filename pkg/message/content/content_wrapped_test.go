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
