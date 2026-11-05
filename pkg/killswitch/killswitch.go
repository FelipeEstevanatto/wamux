// Package killswitch tells one running client loop to stop, and how.
//
// # WHY THIS EXISTS
//
// Client teardown used to be a `chan bool` in a shared map. A send reaches one
// receiver, so a presence goroutine parked on the channel took the signal from
// StartClient. StartClient only polled once a second, so non-blocking sends
// missed it, and re-reading the map meant `close` followed by `Delete` was never
// seen. Every reconnect left a loop behind, each holding a whatsmeow client.
//
// A Switch fixes that by being a broadcast that cannot be missed: firing closes a
// channel, so every holder observes it however late it looks. Holders keep the
// pointer they were given, so replacing the map entry never hides a signal.
//
// The Mode exists because a woken loop must not always clean up. After a
// reconnect the shared maps already describe the replacement client, so the old
// loop has to leave them alone.
package killswitch

import "sync"

// Mode is what the woken client loop should do.
type Mode int

const (
	// Detach disconnects the client and exits without touching shared state. The
	// caller owns the instance's lifecycle (reconnect, delete, replacement).
	Detach Mode = iota
	// Stop disconnects, cleans up and stays down. The caller persists the
	// instance's new state.
	Stop
	// Restart disconnects, cleans up and starts a fresh client.
	Restart
)

// Switch is a one-shot broadcast. A nil *Switch is valid and never fires, so
// callers need no guard for an instance with no running client.
type Switch struct {
	once sync.Once
	done chan struct{}
	mode Mode
}

// New returns an unfired switch.
func New() *Switch {
	return &Switch{done: make(chan struct{})}
}

// Fire signals every holder and never blocks, so it is safe inside an event
// handler. Only the first call counts; it reports whether this one did.
func (s *Switch) Fire(mode Mode) bool {
	if s == nil {
		return false
	}
	fired := false
	s.once.Do(func() {
		s.mode = mode
		close(s.done)
		fired = true
	})
	return fired
}

// Done is closed once the switch fires. For a nil switch it returns nil, which
// blocks forever, matching "never fires".
func (s *Switch) Done() <-chan struct{} {
	if s == nil {
		return nil
	}
	return s.done
}

// Fired reports whether the switch has fired.
func (s *Switch) Fired() bool {
	if s == nil {
		return false
	}
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

// Mode returns the mode of the winning Fire. Call it only after Done is closed:
// the close is what publishes the value.
func (s *Switch) Mode() Mode {
	return s.mode
}
