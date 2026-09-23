package whatsmeow_service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
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

// The whole point of the pool: a burst of messages must not run an unbounded
// number of inserts (and goroutines) at once.
func TestPersistPoolBoundsConcurrency(t *testing.T) {
	const jobs = 60
	repo := &trackingRepo{done: make(chan struct{}, jobs)}
	pool := newPersistPool(3, jobs)

	for i := 0; i < jobs; i++ {
		if !pool.submit(persistJob{repo: repo, instanceID: "inst-1", message: message_model.Message{MessageID: "m"}}) {
			t.Fatalf("submit %d unexpectedly rejected", i)
		}
	}
	for i := 0; i < jobs; i++ {
		<-repo.done
	}

	max := atomic.LoadInt32(&repo.maxInflight)
	if max > 3 {
		t.Fatalf("max concurrency = %d, want <= 3", max)
	}
	if max < 2 {
		t.Fatalf("pool did not run jobs concurrently (max = %d)", max)
	}
}

// blockingRepo blocks every insert until release is closed, so the test can pin
// the single worker and observe the queue filling up.
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
