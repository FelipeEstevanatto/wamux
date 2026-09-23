package whatsmeow_service

import (
	"testing"
	"time"
)

func TestReserveNctSaltSyncDeduplicatesWithinCooldown(t *testing.T) {
	client := &MyClient{}

	if !client.reserveNctSaltSync() {
		t.Fatal("first NCT-salt bootstrap should be reserved")
	}
	if client.reserveNctSaltSync() {
		t.Fatal("duplicate NCT-salt bootstrap should be suppressed during cooldown")
	}

	// Once the cooldown elapses the bootstrap may run again (self-heal after a
	// transient failure, or an account the server provisioned a salt for later).
	client.nctSaltSyncAt = time.Now().Add(-nctSaltSyncCooldown - time.Minute)
	if !client.reserveNctSaltSync() {
		t.Fatal("NCT-salt bootstrap should be allowed again after the cooldown")
	}
}

func TestEnsureNctSaltSyncedIsNilSafe(t *testing.T) {
	// A partially-initialised client must not panic; the bootstrap is a no-op.
	var nilClient *MyClient
	nilClient.ensureNctSaltSynced()

	(&MyClient{}).ensureNctSaltSynced()
}
