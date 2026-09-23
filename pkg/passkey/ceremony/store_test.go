package ceremony

import (
	"encoding/json"
	"testing"
)

// A socket drop clears the ceremony so the extension stops polling a dead
// challenge instead of waiting out the TTL (issue #107). Clear must report
// whether anything was removed so the caller can log/act only on a real clear.
func TestClearReportsWhetherCeremonyExisted(t *testing.T) {
	s := NewStore()
	token := s.Start("instance-1", json.RawMessage(`{"challenge":"abc"}`))

	if _, _, ok := s.Lookup(token); !ok {
		t.Fatal("ceremony should be lookup-able right after Start")
	}
	if !s.HasActiveByInstance("instance-1") {
		t.Fatal("instance should have an active ceremony right after Start")
	}

	if !s.Clear("instance-1") {
		t.Fatal("Clear should report that a ceremony was removed")
	}
	if _, _, ok := s.Lookup(token); ok {
		t.Fatal("ceremony token must not resolve after Clear")
	}
	if s.HasActiveByInstance("instance-1") {
		t.Fatal("instance must not have an active ceremony after Clear")
	}
	if s.Clear("instance-1") {
		t.Fatal("Clear on an instance with no ceremony should report false")
	}
}
