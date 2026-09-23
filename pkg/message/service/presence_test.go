package message_service

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

// An operation that briefly marks the device available (typing indicator,
// presence subscription) must return the device to the instance's configured
// presence. Leaving it available silences push notifications on the operator's
// phone, which is the bug reported in issues #70/#54/#55.
func TestPresenceAfterTransientOnline(t *testing.T) {
	if got := presenceAfterTransientOnline(true); got != types.PresenceAvailable {
		t.Fatalf("alwaysOnline=true should stay available, got %q", got)
	}
	if got := presenceAfterTransientOnline(false); got != types.PresenceUnavailable {
		t.Fatalf("alwaysOnline=false should restore unavailable, got %q", got)
	}
}
