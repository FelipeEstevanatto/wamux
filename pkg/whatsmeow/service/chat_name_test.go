package whatsmeow_service

import (
	"context"
	"testing"
)

// The special sources do not need a live client, so they are covered here; the
// contact/LID/group lookups require a real whatsmeow store and are exercised
// against a live account instead.
func TestResolveChatNameSpecialSources(t *testing.T) {
	ctx := context.Background()
	cases := map[string]string{
		"status":           "Status",
		"0":                "Status",
		"123456@broadcast": "Transmissão",
		"status@broadcast": "Transmissão",
	}
	for in, want := range cases {
		if got := resolveChatName(ctx, nil, in); got != want {
			t.Errorf("resolveChatName(%q) = %q, want %q", in, got, want)
		}
	}
}

// With no live clients an unknown source must fall back to "+<user>" so the
// dashboard still shows something instead of a blank row.
func TestResolveChatNameFallsBackToPhone(t *testing.T) {
	ctx := context.Background()
	if got := resolveChatName(ctx, nil, "5511900000000"); got != "+5511900000000" {
		t.Errorf("resolveChatName = %q, want +5511900000000", got)
	}
	if got := resolveChatName(ctx, nil, ""); got != "" {
		t.Errorf("empty source = %q, want empty", got)
	}
}
