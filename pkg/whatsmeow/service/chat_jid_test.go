package whatsmeow_service

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// With no client (or no LID store), canonicalization is a no-op that still
// strips the device suffix, so callers never panic.
func TestCanonicalChatJIDPassthroughWithoutClient(t *testing.T) {
	lid := types.NewJID("269182931329179", types.HiddenUserServer)
	if got := CanonicalChatJID(context.Background(), nil, lid); got.Server != types.HiddenUserServer || got.User != "269182931329179" {
		t.Fatalf("lid passthrough = %v", got)
	}

	pn := types.JID{User: "5514981170846", Device: 5, Server: types.DefaultUserServer}
	if got := CanonicalChatJID(context.Background(), nil, pn); got.Device != 0 || got.User != "5514981170846" {
		t.Fatalf("pn should be device-less, got %v", got)
	}
}

func TestAlternateChatJIDWithoutClient(t *testing.T) {
	if _, ok := AlternateChatJID(context.Background(), nil, types.NewJID("1", types.HiddenUserServer)); ok {
		t.Fatal("expected no alternates without a client")
	}
}
