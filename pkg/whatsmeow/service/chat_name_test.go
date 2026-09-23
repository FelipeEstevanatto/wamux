package whatsmeow_service

import (
	"context"
	"testing"
)

// The special sources do not need a live client, so they are covered here; the
// contact/LID/group lookups require a real whatsmeow store and are exercised
// against a live account instead.
func TestResolveChatIdentitySpecialSources(t *testing.T) {
	ctx := context.Background()
	cases := map[string]string{
		"status":           "Status",
		"0":                "Status",
		"123456@broadcast": "Transmissão",
	}
	for in, want := range cases {
		if got := resolveChatIdentity(ctx, nil, in); got.Name != want {
			t.Errorf("resolveChatIdentity(%q).Name = %q, want %q", in, got.Name, want)
		}
	}
}

// With no live clients an unknown source resolves to an empty identity; the
// frontend then falls back to "+<key>". An unresolved source must not invent a
// phone number, otherwise two unrelated conversations could be merged.
func TestResolveChatIdentityUnresolvedIsEmpty(t *testing.T) {
	ctx := context.Background()
	if got := resolveChatIdentity(ctx, nil, "5514981170846"); got.Name != "" || got.Phone != "" {
		t.Errorf("no clients: got %+v, want empty identity", got)
	}
	if got := resolveChatIdentity(ctx, nil, ""); got.Name != "" || got.Phone != "" {
		t.Errorf("empty source: got %+v, want empty identity", got)
	}
}
