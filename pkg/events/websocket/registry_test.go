package websocket_producer

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/config"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	"github.com/gorilla/websocket"
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

// dialSubscriber connects a real subscriber through ServeWs.
func dialSubscriber(t *testing.T, p *websocketProducer, instanceID string) *websocket.Conn {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(w, r, instanceID, p)
	}))
	t.Cleanup(srv.Close)

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func subscriberCount(p *websocketProducer) int {
	p.clientsMux.RLock()
	defer p.clientsMux.RUnlock()
	return len(p.broadcast)
}

func waitForCount(t *testing.T, p *websocketProducer, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for subscriberCount(p) != want {
		if time.Now().After(deadline) {
			t.Fatalf("broadcast subscribers = %d, want %d", subscriberCount(p), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A subscriber that stops reading must not hold up the caller, which goes on to
// deliver the same event to the webhook. The blocking writer this replaces let
// 41 of 2,000 events through.
func TestStalledSubscriberNeverBlocksProduce(t *testing.T) {
	p := newTestProducer(t, "inst")
	dialSubscriber(t, p, "") // never reads
	waitForCount(t, p, 1)

	payload := bytes.Repeat([]byte("x"), 64<<10)
	start := time.Now()
	for i := 0; i < 2000; i++ {
		if err := p.Produce("q", payload, "inst", ""); err != nil {
			t.Fatalf("Produce: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("2000 events took %s: Produce waited on the socket", elapsed)
	}

	// And it is evicted, rather than kept buffering.
	waitForCount(t, p, 0)
}

// A stalled subscriber must not cost a healthy sibling anything.
func TestHealthySubscriberKeepsReceivingNextToStalledOne(t *testing.T) {
	p := newTestProducer(t, "inst")
	dialSubscriber(t, p, "") // stalled
	healthy := dialSubscriber(t, p, "inst")
	waitForCount(t, p, 1)
	deadline := time.Now().Add(5 * time.Second)
	for {
		p.clientsMux.RLock()
		n := len(p.clients["inst"])
		p.clientsMux.RUnlock()
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("healthy subscriber never registered")
		}
		time.Sleep(5 * time.Millisecond)
	}

	const events = 600
	received := make(chan int, 1)
	go func() {
		n := 0
		for n < events {
			_ = healthy.SetReadDeadline(time.Now().Add(5 * time.Second))
			if _, _, err := healthy.ReadMessage(); err != nil {
				break
			}
			n++
		}
		received <- n
	}()

	payload := bytes.Repeat([]byte("x"), 64<<10)
	for i := 0; i < events; i++ {
		_ = p.Produce("q", payload, "inst", "")
		time.Sleep(200 * time.Microsecond) // a realistic reader keeps up
	}
	if n := <-received; n != events {
		t.Fatalf("healthy subscriber received %d of %d", n, events)
	}
}

// The wire format is what the manager parses.
func TestFrameFormatIsUnchanged(t *testing.T) {
	p := newTestProducer(t, "inst")
	conn := dialSubscriber(t, p, "inst")
	deadline := time.Now().Add(5 * time.Second)
	for {
		p.clientsMux.RLock()
		n := len(p.clients["inst"])
		p.clientsMux.RUnlock()
		if n == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if err := p.Produce("Inst.Message", []byte(`{"event":"Message"}`), "inst", ""); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var got map[string]string
	if err := conn.ReadJSON(&got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got["queue"] != "inst.message" || got["payload"] != `{"event":"Message"}` {
		t.Fatalf("frame = %v", got)
	}
}

// A payload can be a whole video in base64: with nobody listening it must not be
// copied or encoded at all.
func TestProduceDoesNoWorkWithoutSubscribers(t *testing.T) {
	p := newTestProducer(t, "inst")
	payload := bytes.Repeat([]byte("x"), 1<<20)

	allocs := testing.AllocsPerRun(5, func() {
		_ = p.Produce("q", payload, "inst", "")
	})
	if allocs > 5 {
		t.Fatalf("%.0f allocations with no subscribers", allocs)
	}
}
