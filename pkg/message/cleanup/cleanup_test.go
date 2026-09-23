package message_cleanup

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeDeleter records the cutoffs it was asked to delete before.
type fakeDeleter struct {
	mu      sync.Mutex
	cutoffs []string
	deleted int64
	err     error
}

func (f *fakeDeleter) DeleteMessagesOlderThan(cutoff string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cutoffs = append(f.cutoffs, cutoff)
	return f.deleted, f.err
}

func (f *fakeDeleter) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.cutoffs...)
}

func TestSweepUsesTheRetentionCutoff(t *testing.T) {
	repo := &fakeDeleter{}
	c := NewCleaner(repo, 365).(*cleaner)
	c.now = func() time.Time { return time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC) }

	c.sweep()

	calls := repo.calls()
	if len(calls) != 1 {
		t.Fatalf("delete calls = %v, want exactly 1", calls)
	}
	if want := "2025-09-23 12:00:00"; calls[0] != want {
		t.Fatalf("cutoff = %q, want %q (365 days before now)", calls[0], want)
	}
}

func TestSweepHonoursAShorterRetention(t *testing.T) {
	repo := &fakeDeleter{}
	c := NewCleaner(repo, 30).(*cleaner)
	c.now = func() time.Time { return time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC) }

	c.sweep()

	if calls := repo.calls(); len(calls) != 1 || calls[0] != "2026-08-24 12:00:00" {
		t.Fatalf("cutoff = %v, want 2026-08-24 12:00:00", calls)
	}
}

// Retention of zero means "keep forever", so nothing must be scheduled.
func TestStartDoesNothingWhenRetentionIsZero(t *testing.T) {
	repo := &fakeDeleter{}
	c := NewCleaner(repo, 0).(*cleaner)
	c.startDelay = time.Millisecond
	c.runInterval = time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Start(ctx)
	time.Sleep(30 * time.Millisecond)

	if calls := repo.calls(); len(calls) != 0 {
		t.Fatalf("expected no sweeps with retention disabled, got %v", calls)
	}
}

func TestLoopSweepsThenStopsOnCancel(t *testing.T) {
	repo := &fakeDeleter{}
	c := NewCleaner(repo, 365).(*cleaner)
	c.startDelay = time.Millisecond
	c.runInterval = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)

	// Wait for at least one sweep.
	deadline := time.After(2 * time.Second)
	for len(repo.calls()) == 0 {
		select {
		case <-deadline:
			t.Fatal("cleaner never swept")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	cancel()
	time.Sleep(20 * time.Millisecond) // let an in-flight sweep finish
	settled := len(repo.calls())
	time.Sleep(40 * time.Millisecond)

	if got := len(repo.calls()); got != settled {
		t.Fatalf("cleaner kept sweeping after cancel: %d -> %d", settled, got)
	}
}
