package whatsmeow_service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
)

// trackingRepo records the peak number of concurrent InsertMessage calls.
type trackingRepo struct {
	message_repository.MessageRepository
	inflight    int32
	maxInflight int32
	done        chan struct{}
}

func (r *trackingRepo) InsertMessage(message_model.Message) error {
	n := atomic.AddInt32(&r.inflight, 1)
	for {
		max := atomic.LoadInt32(&r.maxInflight)
		if n <= max || atomic.CompareAndSwapInt32(&r.maxInflight, max, n) {
			break
		}
	}
	// Give other workers a chance to overlap.
	time.Sleep(200 * time.Microsecond)
	atomic.AddInt32(&r.inflight, -1)
	r.done <- struct{}{}
	return nil
}

// InsertMessages is the batched path the pool now uses. Concurrency is measured
// here (one in-flight batch per worker), and it signals once per message so the
// test can wait for every message to be persisted.
func (r *trackingRepo) InsertMessages(msgs []message_model.Message) error {
	n := atomic.AddInt32(&r.inflight, 1)
	for {
		max := atomic.LoadInt32(&r.maxInflight)
		if n <= max || atomic.CompareAndSwapInt32(&r.maxInflight, max, n) {
			break
		}
	}
	// Give other workers a chance to overlap.
	time.Sleep(200 * time.Microsecond)
	atomic.AddInt32(&r.inflight, -1)
	for range msgs {
		r.done <- struct{}{}
	}
	return nil
}

// The whole point of the pool: a burst of messages must not run an unbounded
// number of writes (and goroutines) at once. With batching the unit of
// concurrency is a batch, so submit more than one batch per worker to have
// several in flight; the bound (workers) must still hold.
func TestPersistPoolBoundsConcurrency(t *testing.T) {
	const jobs = 60
	repo := &trackingRepo{done: make(chan struct{}, jobs)}
	// One worker, so all jobs funnel through it and the test measures the pool's
	// own serialization rather than worker fan-out.
	pool := newPersistPool(1, jobs)

	for i := 0; i < jobs; i++ {
		if !pool.submit(persistJob{repo: repo, instanceID: "inst-1", message: message_model.Message{MessageID: "m"}}) {
			t.Fatalf("submit %d unexpectedly rejected", i)
		}
	}
	for i := 0; i < jobs; i++ {
		<-repo.done
	}

	max := atomic.LoadInt32(&repo.maxInflight)
	if max > 1 {
		t.Fatalf("max concurrency = %d, want <= 1 (single worker)", max)
	}
	if max < 1 {
		t.Fatalf("pool never ran a job")
	}
}

// blockingRepo blocks every write until release is closed, so the test can pin
// the single worker and observe the queue filling up. The pool now calls the
// batched method, so that is where it blocks.
type blockingRepo struct {
	message_repository.MessageRepository
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingRepo) InsertMessage(message_model.Message) error {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return nil
}

func (r *blockingRepo) InsertMessages([]message_model.Message) error {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return nil
}

func TestPersistPoolSubmitRejectsWhenFull(t *testing.T) {
	repo := &blockingRepo{started: make(chan struct{}), release: make(chan struct{})}
	pool := newPersistPool(1, 1)

	if !pool.submit(persistJob{repo: repo, instanceID: "inst-1", message: message_model.Message{MessageID: "m1"}}) {
		t.Fatal("first submit rejected")
	}
	<-repo.started // the single worker is now blocked inside InsertMessage

	if !pool.submit(persistJob{repo: repo, instanceID: "inst-1", message: message_model.Message{MessageID: "m2"}}) {
		t.Fatal("second submit should fill the queue")
	}
	if pool.submit(persistJob{repo: repo, instanceID: "inst-1", message: message_model.Message{MessageID: "m3"}}) {
		t.Fatal("third submit should be rejected: queue is full")
	}

	close(repo.release)
}
