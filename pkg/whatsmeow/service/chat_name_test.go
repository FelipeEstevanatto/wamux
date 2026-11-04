package whatsmeow_service

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/types"
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
	if got := resolveChatIdentity(ctx, nil, "5511900000000"); got.Name != "" || got.Phone != "" {
		t.Errorf("no clients: got %+v, want empty identity", got)
	}
	if got := resolveChatIdentity(ctx, nil, ""); got.Name != "" || got.Phone != "" {
		t.Errorf("empty source: got %+v, want empty identity", got)
	}
}

// A whatsmeow client can be connected while its contact store is absent (for
// example right after a restart). Contact lookups must degrade to an empty name
// instead of panicking the dashboard request.
func TestContactDisplayNameNilStore(t *testing.T) {
	jid := types.NewJID("5511900000000", types.DefaultUserServer)
	if got := contactDisplayName(context.Background(), nil, jid); got != "" {
		t.Errorf("nil contact store: got %q, want empty", got)
	}
}
