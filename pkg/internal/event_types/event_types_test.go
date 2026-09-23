package event_types

import "testing"

func TestPasskeyEventTypeRegistered(t *testing.T) {
	if !IsEventType(PASSKEY) {
		t.Fatal("PASSKEY must be a valid, subscribable event type")
	}
	found := false
	for _, e := range AllEventTypes {
		if e == PASSKEY {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("PASSKEY must be part of AllEventTypes so ALL covers it")
	}
}
