package bgpool

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGoRunsEveryJob(t *testing.T) {
	p := New(4, 8)
	var count atomic.Int64
	var wg sync.WaitGroup

	total := 100
	wg.Add(total)
	for i := 0; i < total; i++ {
		p.Go(func() {
			count.Add(1)
			wg.Done()
		})
	}
	wg.Wait()

	if got := count.Load(); got != int64(total) {
		t.Fatalf("ran %d jobs, want %d", got, total)
	}
}

// Go must run the job inline when the queue is full so nothing is lost.
func TestGoFallsBackInlineWhenFull(t *testing.T) {
	// No workers, tiny queue: the queue fills immediately and every later job
	// must run inline on the calling goroutine.
	p := New(1, 1)

	block := make(chan struct{})
	p.Submit(func() { <-block }) // occupies the single worker
	p.Submit(func() {})          // occupies the single queue slot

	done := make(chan struct{})
	go func() {
		p.Go(func() {}) // queue full -> runs inline
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("Go blocked instead of running inline when the queue was full")
	}
	close(block)
	p2 := p // keep referenced
	_ = p2
}

func TestSubmitReportsDrop(t *testing.T) {
	p := New(1, 1)
	block := make(chan struct{})
	defer close(block)

	if !p.Submit(func() { <-block }) { // worker picks this up
		t.Fatalf("first submit should succeed")
	}
	// Give the worker a moment to take the first job off the queue.
	time.Sleep(10 * time.Millisecond)
	if !p.Submit(func() {}) { // fills the queue
		t.Fatalf("second submit should succeed")
	}

	// The queue is now full; the next submit must report the drop.
	if p.Submit(func() {}) {
		t.Fatalf("expected submit to fail when the queue is full")
	}

	submitted, dropped := p.Stats()
	if dropped == 0 {
		t.Fatalf("dropped counter not incremented (submitted=%d)", submitted)
	}
	if p.Workers() != 1 {
		t.Fatalf("Workers() = %d, want 1", p.Workers())
	}
}

func TestNilPoolIsSafe(t *testing.T) {
	var p *Pool
	ran := false
	p.Go(func() { ran = true })
	if !ran {
		t.Fatalf("nil pool should run the job inline")
	}
	if p.Submit(func() {}) {
		t.Fatalf("nil pool submit should return false")
	}
	if p.Workers() != 0 {
		t.Fatalf("nil pool workers should be 0")
	}
}

func TestNewUsesDefaultsForInvalidSizes(t *testing.T) {
	p := New(0, 0)
	if p.Workers() != DefaultWorkers {
		t.Fatalf("workers = %d, want default %d", p.Workers(), DefaultWorkers)
	}
}
