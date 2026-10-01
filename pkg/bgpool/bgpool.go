// Package bgpool runs fire-and-forget background work through a fixed-size
// worker pool with a bounded queue.
//
// # WHY THIS EXISTS
//
// Several places in the codebase hand work off with a bare `go func() {}()` —
// saving a poll vote, scheduling a reconnect, marking a message read, refreshing
// account limits, rotating QR codes. `go func` is unbounded by construction: a
// burst (a history sync, many instances reconnecting at once, a status
// broadcast) spawns one goroutine per event with no cap. Under load that is
// thousands of goroutines and unbounded pressure on the database and WhatsApp
// sockets — exactly the "concurrent map writes" class of problem, but with
// memory instead of a crash.
//
// A background job here is best-effort: if the queue is full the job is dropped
// and the caller is told so (Submit returns false), so it can decide whether to
// fall back to running inline or to log and move on. Dropping is deliberate —
// blocking an event-dispatch goroutine on a full queue would stall the whole
// client behind one slow database.
package bgpool

import (
	"sync/atomic"
)

// Defaults chosen to absorb a burst without letting memory grow without bound.
// Sized for the whole process, not per instance.
const (
	DefaultWorkers = 16
	DefaultQueue   = 4096
)

// Pool is a bounded executor. Use New to build one; the zero value is not
// usable. It is safe for concurrent use.
type Pool struct {
	queue   chan func()
	workers int

	// submitted/dropped let tests and /server/stats see whether the pool is
	// saturating. Plain counters are enough; exactness under contention is not
	// needed for a diagnostic.
	submitted atomic.Uint64
	dropped   atomic.Uint64
}

// New starts a pool with the given number of workers draining a queue of the
// given size. Non-positive values fall back to the defaults.
func New(workers, queueSize int) *Pool {
	if workers < 1 {
		workers = DefaultWorkers
	}
	if queueSize < 1 {
		queueSize = DefaultQueue
	}

	p := &Pool{queue: make(chan func(), queueSize), workers: workers}
	for i := 0; i < workers; i++ {
		go func() {
			for job := range p.queue {
				job()
			}
		}()
	}
	return p
}

// Submit enqueues job. It returns false when the queue is full (the job is NOT
// run) so the caller can fall back. A nil job is ignored and counts as
// submitted.
func (p *Pool) Submit(job func()) bool {
	if p == nil {
		return false
	}
	if job == nil {
		p.submitted.Add(1)
		return true
	}

	select {
	case p.queue <- job:
		p.submitted.Add(1)
		return true
	default:
		p.dropped.Add(1)
		return false
	}
}

// Go enqueues job, falling back to running it inline when the queue is full so
// the work is never lost. Use this when losing the job is worse than briefly
// blocking the caller; use Submit when dropping is acceptable.
func (p *Pool) Go(job func()) {
	if job == nil {
		return
	}
	if p == nil {
		job()
		return
	}
	if !p.Submit(job) {
		job()
	}
}

// Stats reports the pool's lifetime counters, for diagnostics.
func (p *Pool) Stats() (submitted, dropped uint64) {
	if p == nil {
		return 0, 0
	}
	return p.submitted.Load(), p.dropped.Load()
}

// Workers is how many goroutines drain the queue.
func (p *Pool) Workers() int {
	if p == nil {
		return 0
	}
	return p.workers
}
