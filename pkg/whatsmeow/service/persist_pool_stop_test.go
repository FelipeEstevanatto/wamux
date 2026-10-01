package whatsmeow_service

import (
	"sync"
	"testing"

	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
)

// recordingRepo records which message IDs reached the database, so a test can
// assert that a shutdown actually flushed the pool's pending batch.
type recordingRepo struct {
	message_repository.MessageRepository
	mu  sync.Mutex
	got []string
}

func (r *recordingRepo) InsertMessage(m message_model.Message) error {
	r.mu.Lock()
	r.got = append(r.got, m.MessageID)
	r.mu.Unlock()
	return nil
}

func (r *recordingRepo) InsertMessages(msgs []message_model.Message) error {
	r.mu.Lock()
	for _, m := range msgs {
		r.got = append(r.got, m.MessageID)
	}
	r.mu.Unlock()
	return nil
}

func (r *recordingRepo) messages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.got...)
}

// The bug this whole fix exists for: a message submitted but not yet flushed
// (still inside the 20 ms batching window) must be written when Stop is called.
// Otherwise a clean restart silently loses it.
func TestPersistPoolStopFlushesPendingBatch(t *testing.T) {
	repo := &recordingRepo{}
	pool := newPersistPool(1, 10)

	if !pool.submit(persistJob{repo: repo, instanceID: "inst-1", message: message_model.Message{MessageID: "pending"}}) {
		t.Fatal("submit unexpectedly rejected")
	}

	// Stop returns only after the workers have flushed their current batch.
	pool.Stop()

	got := repo.messages()
	if len(got) != 1 || got[0] != "pending" {
		t.Fatalf("pending batch lost on shutdown: persisted %v, want [pending]", got)
	}
}

// After Stop, submit must be rejected deterministically (not panic by sending on
// a closed channel) so the caller's inline fallback persists the message.
func TestPersistPoolSubmitAfterStopIsRejected(t *testing.T) {
	pool := newPersistPool(1, 4)
	pool.Stop()

	if pool.submit(persistJob{repo: &recordingRepo{}, instanceID: "inst-1", message: message_model.Message{MessageID: "after"}}) {
		t.Fatal("submit after Stop must be rejected so the message is persisted inline")
	}
}

// Stop is called from main's shutdown path, which may run more than once
// (deferred cleanup plus the explicit call), so it must be idempotent.
func TestPersistPoolStopIsIdempotent(t *testing.T) {
	pool := newPersistPool(2, 4)
	pool.Stop()
	pool.Stop()
}

// Event handlers keep calling persistMessageAsync while shutdown runs, so a
// submit can race Stop. That must never panic (send on a closed channel); run
// under -race to exercise the guard.
func TestPersistPoolStopRacesSubmitWithoutPanic(t *testing.T) {
	repo := &recordingRepo{}
	pool := newPersistPool(2, 8)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				pool.submit(persistJob{repo: repo, instanceID: "inst-1", message: message_model.Message{MessageID: "m"}})
			}
		}()
	}

	pool.Stop()
	wg.Wait()
}
