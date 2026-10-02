package content

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

// The summary "Type" is a closed set; both the API contract and the dashboard
// depend on it, so an unexpected value is a bug.
var validTypes = map[string]struct{}{
	"text": {}, "image": {}, "video": {}, "audio": {}, "document": {},
	"sticker": {}, "location": {}, "contact": {}, "reaction": {}, "edit": {},
	"delete": {}, "poll": {}, "poll_update": {}, "buttons_response": {},
	"list_response": {}, "interactive_response": {}, "unknown": {},
}

// FuzzSummarize drives Summarize with a message whose top-level Conversation is
// arbitrary bytes. Summarize type-switches on the concrete message variant, so
// arbitrary bytes let the fuzzer reach many branches; the invariants checked
// are exactly the ones the readback API and the dashboard rely on.
func FuzzSummarize(f *testing.F) {
	for _, s := range []string{
		"", "hi", "\x00\xff", "multi\nline", "{" + `"k":1` + "}",
		"emoji 😀", string([]byte{0, 1, 2, 3}),
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, conversation string) {
		msg := &waE2E.Message{Conversation: &conversation}

		s := Summarize(msg)

		// 1. No panic, and the type is always from the closed set.
		if _, ok := validTypes[s.Type]; !ok {
			t.Fatalf("Summarize produced unknown type %q", s.Type)
		}
		// 2. A NON-EMPTY conversation is always classified as text and surfaces
		//    the exact body (the fundamental readback guarantee). An empty
		//    conversation is an empty message: it falls through to "unknown".
		if conversation != "" {
			if s.Type != "text" || s.Text != conversation {
				t.Fatalf("Summarize(conversation=%q) = %+v, want type=text text=%q", conversation, s, conversation)
			}
		} else if s.Type != "unknown" {
			t.Fatalf("Summarize(empty conversation) = %+v, want type=unknown", s)
		}
		// 3. Idempotent: summarizing the same message twice is stable.
		if again := Summarize(msg); again != s {
			t.Fatalf("Summarize is not deterministic: %+v vs %+v", s, again)
		}
	})
}
