package whatsmeow_service

import (
	"sync"
	"testing"
	"time"

	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
)

// batchRecordingRepo records whether writes arrived as batches or singletons.
// It embeds the interface (nil) so only the two insert methods need defining.
type batchRecordingRepo struct {
	message_repository.MessageRepository
	mu           sync.Mutex
	batchCalls   int
	singleCalls  int
	messagesInGo int
}

func (r *batchRecordingRepo) InsertMessage(m message_model.Message) error {
	r.mu.Lock()
	r.singleCalls++
	r.mu.Unlock()
	return nil
}

func (r *batchRecordingRepo) InsertMessages(msgs []message_model.Message) error {
	r.mu.Lock()
	r.batchCalls++
	r.messagesInGo += len(msgs)
	r.mu.Unlock()
	return nil
}

func (r *batchRecordingRepo) snapshot() (batches, singles, inBatches int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.batchCalls, r.singleCalls, r.messagesInGo
}

// A burst of messages must be coalesced into batches: far fewer DB calls than
// messages, which is the whole point of the batched persistence path.
func TestPersistPoolCoalescesIntoBatches(t *testing.T) {
	repo := &batchRecordingRepo{}
	pool := newPersistPool(1, 4096)

	const total = 250
	for i := 0; i < total; i++ {
		pool.submit(persistJob{repo: repo, instanceID: "inst-1", message: message_model.Message{MessageID: "m"}})
	}

	// Wait for the worker to drain (bounded by batch window + flush).
	deadline := time.Now().Add(3 * time.Second)
	for {
		batches, singles, inBatches := repo.snapshot()
		if inBatches+singles >= total {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for persistence: batches=%d singles=%d inBatches=%d", batches, singles, inBatches)
		}
		time.Sleep(10 * time.Millisecond)
	}

	batches, singles, inBatches := repo.snapshot()
	if inBatches != total {
		t.Fatalf("persisted %d messages, want %d (batches=%d singles=%d)", inBatches, total, batches, singles)
	}
	// 250 messages at batch size 100 should be <= 4 calls, not 250.
	if batches > 5 {
		t.Fatalf("expected coalescing, got %d batch calls for %d messages", batches, total)
	}
	if singles != 0 {
		t.Fatalf("expected no per-message fallback, got %d", singles)
	}
}

// A single message must still be written promptly (the batch window bounds the
// wait), not held indefinitely for a full batch.
func TestPersistPoolFlushesSingleMessage(t *testing.T) {
	repo := &batchRecordingRepo{}
	pool := newPersistPool(1, 16)

	pool.submit(persistJob{repo: repo, instanceID: "inst-1", message: message_model.Message{MessageID: "m1"}})

	deadline := time.Now().Add(2 * time.Second)
	for {
		batches, singles, inBatches := repo.snapshot()
		if inBatches+singles >= 1 {
			if batches != 1 || inBatches != 1 {
				t.Fatalf("single message should flush as one batch: batches=%d inBatches=%d", batches, inBatches)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("single message was not flushed within the batch window")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
