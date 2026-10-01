package websocket_producer

import (
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
)

// newTestProducer builds a producer backed by a real file logger (the registry
// methods log). Loggers write asynchronously, so close them before t.TempDir's
// own cleanup removes the directory; cleanup runs LIFO, so this runs first.
func newTestProducer(t *testing.T, instanceIDs ...string) *websocketProducer {
	t.Helper()
	dir := t.TempDir()
	lm := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: dir})
	t.Cleanup(func() {
		for _, id := range instanceIDs {
			lm.GetLogger(id).Close()
		}
	})
	return NewWebsocketProducer(lm)
}

// drop must not mutate the caller's backing array: Produce snapshots the slice
// under a read lock and a concurrent removal must not disturb it.
func TestDropRemovesOnlyTargetAndAllocates(t *testing.T) {
	a, b, c := &client{}, &client{}, &client{}
	list := []*client{a, b, c}

	got := drop(list, b)
	if len(got) != 2 || got[0] != a || got[1] != c {
		t.Fatalf("drop = %v, want [a c]", got)
	}
	if len(list) != 3 {
		t.Fatalf("original slice was mutated: %v", list)
	}
}

// Two tabs subscribed to the same instance must both stay registered; removing
// one must not cut delivery to the other (the bug this registry fixes).
func TestAddRemoveClientKeepsSiblings(t *testing.T) {
	p := newTestProducer(t, "inst")
	a, b := &client{}, &client{}

	p.addClient("inst", a)
	p.addClient("inst", b)

	p.clientsMux.RLock()
	got := append([]*client(nil), p.clients["inst"]...)
	p.clientsMux.RUnlock()
	if len(got) != 2 {
		t.Fatalf("expected 2 subscribers, got %d", len(got))
	}

	p.removeClient("inst", a)
	p.clientsMux.RLock()
	got = append([]*client(nil), p.clients["inst"]...)
	_, exists := p.clients["inst"]
	p.clientsMux.RUnlock()
	if len(got) != 1 || got[0] != b || !exists {
		t.Fatalf("sibling subscriber lost: %v (exists=%v)", got, exists)
	}

	// Removing the last subscriber drops the whole entry.
	p.removeClient("inst", b)
	p.clientsMux.RLock()
	_, exists = p.clients["inst"]
	p.clientsMux.RUnlock()
	if exists {
		t.Fatal("instance entry should be deleted once empty")
	}
}

func TestClientRegistryIsolatesInstances(t *testing.T) {
	p := newTestProducer(t, "one", "two")
	a, b := &client{}, &client{}
	p.addClient("one", a)
	p.addClient("two", b)

	p.removeClient("one", a)

	p.clientsMux.RLock()
	_, oneExists := p.clients["one"]
	two := append([]*client(nil), p.clients["two"]...)
	p.clientsMux.RUnlock()

	if oneExists {
		t.Fatal("removing instance one's client must not leave an entry")
	}
	if len(two) != 1 || two[0] != b {
		t.Fatalf("instance two was affected: %v", two)
	}
}

func TestBroadcastRegistry(t *testing.T) {
	p := newTestProducer(t)
	a, b := &client{}, &client{}

	p.addBroadcastClient(a)
	p.addBroadcastClient(b)
	p.clientsMux.RLock()
	n := len(p.broadcast)
	p.clientsMux.RUnlock()
	if n != 2 {
		t.Fatalf("broadcast clients = %d, want 2", n)
	}

	p.removeBroadcastClient(a)
	p.clientsMux.RLock()
	remaining := append([]*client(nil), p.broadcast...)
	p.clientsMux.RUnlock()
	if len(remaining) != 1 || remaining[0] != b {
		t.Fatalf("broadcast removal = %v, want [b]", remaining)
	}
}
