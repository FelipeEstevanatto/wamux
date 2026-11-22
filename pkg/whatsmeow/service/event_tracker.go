package whatsmeow_service

import (
	"context"
	"sync"
)

// eventTracker counts the event handlers currently running for one instance.
//
// A whatsmeow handler is synchronous: the client waits for it before dispatching
// the next event. During shutdown the process used to stop HTTP, drain the
// persistence pool and exit, while a handler could still be downloading media or
// writing a history batch. That work died with the process and WhatsApp does not
// resend an acked message.
//
// begin/end wrap every dispatched event (including a media job queued on
// mediaPool). drain stops the caller until the count reaches zero or the context
// deadline expires. New events may still arrive between "drain started" and the
// socket actually closing; they are counted too, so they either finish or the
// deadline reports the stall.
type eventTracker struct {
	mu      sync.Mutex
	running int
	waiters []chan struct{}
}

func newEventTracker() *eventTracker { return &eventTracker{} }

func (t *eventTracker) begin() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.running++
	t.mu.Unlock()
}

func (t *eventTracker) end() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.running > 0 {
		t.running--
	}
	if t.running == 0 {
		for _, ch := range t.waiters {
			close(ch)
		}
		t.waiters = nil
	}
	t.mu.Unlock()
}

// drain waits until no handler is running. It returns ctx.Err() when the
// deadline expires first, leaving the remaining handlers to finish on their own.
func (t *eventTracker) drain(ctx context.Context) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	if t.running == 0 {
		t.mu.Unlock()
		return nil
	}
	ch := make(chan struct{})
	t.waiters = append(t.waiters, ch)
	t.mu.Unlock()

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// runningCount is for diagnostics and tests.
func (t *eventTracker) runningCount() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}
