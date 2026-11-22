package message_cleanup

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeDeleter records the cutoffs it was asked to delete before.
type fakeDeleter struct {
	orphanCutoffs []string
	mu            sync.Mutex
	cutoffs       []string
	deleted       int64
	err           error
}

func (f *fakeDeleter) DeleteOrphanReceipts(cutoff string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.orphanCutoffs = append(f.orphanCutoffs, cutoff)
	return 0, nil
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

type fakeTableDeleter struct {
	calls   []tableCall
	deleted int64
	err     error
}

type tableCall struct {
	table, column string
	cutoff        time.Time
	text          bool
}

func (f *fakeTableDeleter) DeleteOlderThan(table, column string, cutoff time.Time, textColumn bool) (int64, error) {
	f.calls = append(f.calls, tableCall{table, column, cutoff, textColumn})
	return f.deleted, f.err
}

type fakeMetrics struct {
	deleted map[string]int64
}

func (f *fakeMetrics) AddCleanupDeleted(table string, n int64) {
	if f.deleted == nil {
		f.deleted = map[string]int64{}
	}
	f.deleted[table] += n
}

func TestSweepPrunesConfiguredTablesAndReportsMetrics(t *testing.T) {
	repo := &fakeDeleter{}
	deleter := &fakeTableDeleter{deleted: 4}
	metrics := &fakeMetrics{}
	c := NewCleaner(repo, 0,
		WithTableRetention(deleter, TableRetention{Table: "send_operations", Column: "created_at", Days: 30}),
		WithTableRetention(deleter, TableRetention{Table: "chat_reads", Column: "read_at", Days: 90, TextColumn: true}),
		WithCleanupMetrics(metrics),
	).(*cleaner)
	c.now = func() time.Time { return time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC) }

	c.sweep()

	if len(deleter.calls) != 2 {
		t.Fatalf("calls = %+v", deleter.calls)
	}
	if deleter.calls[0].table != "send_operations" || deleter.calls[0].cutoff.Format("2006-01-02") != "2026-08-24" {
		t.Fatalf("first call = %+v", deleter.calls[0])
	}
	if !deleter.calls[1].text || deleter.calls[1].cutoff.Format("2006-01-02") != "2026-06-25" {
		t.Fatalf("second call = %+v", deleter.calls[1])
	}
	if metrics.deleted["send_operations"] != 4 || metrics.deleted["chat_reads"] != 4 {
		t.Fatalf("metrics = %+v", metrics.deleted)
	}
}

func TestTableRetentionRejectsDisabledAndUnsafeRules(t *testing.T) {
	c := NewCleaner(&fakeDeleter{}, 0,
		WithTableRetention(&fakeTableDeleter{}, TableRetention{Table: "send_operations", Column: "created_at", Days: 0}),
		WithTableRetention(&fakeTableDeleter{}, TableRetention{Table: "messages; DROP TABLE x", Column: "created_at", Days: 1}),
	).(*cleaner)
	if len(c.tableRetentions) != 0 {
		t.Fatalf("rules = %+v, want none", c.tableRetentions)
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

// Message rows were pruned but their files never were, so the disk grew forever.
func TestSweepPrunesMediaWithItsOwnRetention(t *testing.T) {
	repo := &fakeDeleter{}
	var cutoff time.Time
	c := NewCleaner(repo, 365, WithMediaRetention(30, func(t time.Time) (int, error) {
		cutoff = t
		return 3, nil
	})).(*cleaner)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	c.sweep()

	if want := now.AddDate(0, 0, -30); !cutoff.Equal(want) {
		t.Fatalf("cutoff = %v, want %v", cutoff, want)
	}
}

func TestMediaRetentionRunsWithoutMessageRetention(t *testing.T) {
	ran := false
	c := NewCleaner(&fakeDeleter{}, 0, WithMediaRetention(30, func(time.Time) (int, error) {
		ran = true
		return 0, nil
	})).(*cleaner)

	c.sweep()

	if !ran {
		t.Fatal("media was not pruned")
	}
}

func TestZeroMediaRetentionKeepsFiles(t *testing.T) {
	c := NewCleaner(&fakeDeleter{}, 365, WithMediaRetention(0, func(time.Time) (int, error) {
		t.Fatal("pruned with retention disabled")
		return 0, nil
	})).(*cleaner)

	c.sweep()
}

// Keeping messages forever must not mean keeping receipts that never found one.
func TestOrphanReceiptsAreSweptEvenWithoutRetention(t *testing.T) {
	repo := &fakeDeleter{}
	c := NewCleaner(repo, 0).(*cleaner)
	c.now = func() time.Time { return time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC) }

	c.sweep()

	if len(repo.orphanCutoffs) != 1 || repo.orphanCutoffs[0] != "2026-11-05 12:00:00" {
		t.Fatalf("cutoffs = %v", repo.orphanCutoffs)
	}
	if len(repo.cutoffs) != 0 {
		t.Fatalf("deleted messages with retention off: %v", repo.cutoffs)
	}
}
