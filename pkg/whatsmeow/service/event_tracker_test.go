package whatsmeow_service

import (
	"context"
	"testing"
	"time"
)

func TestEventTrackerWaitsForInflight(t *testing.T) {
	tr := newEventTracker()
	tr.begin()

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		done <- tr.drain(ctx)
	}()

	// The drain must not return while the handler is running.
	select {
	case err := <-done:
		t.Fatalf("drain returned early: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	tr.end()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("drain: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("drain did not return after the handler finished")
	}
}

func TestEventTrackerRespectsDeadline(t *testing.T) {
	tr := newEventTracker()
	tr.begin()
	defer tr.end()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := tr.drain(ctx); err == nil {
		t.Fatal("drain returned nil with a handler still running")
	}
	if got := tr.runningCount(); got != 1 {
		t.Fatalf("running = %d, want 1", got)
	}
}

func TestEventTrackerCountsConcurrentHandlers(t *testing.T) {
	tr := newEventTracker()
	const n = 8
	release := make(chan struct{})
	for i := 0; i < n; i++ {
		tr.begin()
		go func() {
			<-release
			tr.end()
		}()
	}
	if got := tr.runningCount(); got != n {
		t.Fatalf("running = %d, want %d", got, n)
	}
	close(release)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := tr.drain(ctx); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if got := tr.runningCount(); got != 0 {
		t.Fatalf("running = %d after drain", got)
	}
}
