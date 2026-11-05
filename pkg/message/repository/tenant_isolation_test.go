package message_repository

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"testing"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
)

// Rows must order by the counter key, with the day compared on its own. Ordering
// by the full timestamp looks equivalent and is not: these two batches would take
// the same two counters in opposite orders, and deadlock.
func TestSortForCounterLocksUsesTheDayNotTheTime(t *testing.T) {
	row := func(id, ts, source string) message_model.Message {
		return message_model.Message{MessageID: id, InstanceId: "i", Timestamp: ts, Status: "Received", Source: source}
	}
	first := []message_model.Message{row("a", "2026-11-05 08:00:00", "B"), row("b", "2026-11-05 09:00:00", "A")}
	second := []message_model.Message{row("c", "2026-11-05 08:00:00", "A"), row("d", "2026-11-05 09:00:00", "B")}

	sources := func(rows []message_model.Message) []string {
		sortForCounterLocks(rows)
		return []string{rows[0].Source, rows[1].Source}
	}
	if a, b := sources(first), sources(second); !reflect.DeepEqual(a, b) {
		t.Fatalf("batches lock counters in different orders: %v vs %v", a, b)
	}
}

// Two instances on this server in one conversation get the same message id.
// Each must keep its own view of it.
func TestSameMessageIDIsSeparatePerInstance(t *testing.T) {
	db := realDB(t)
	repo := NewMessageRepository(db)
	const id = "sem-shared-id"
	ts := "2026-10-01 12:00:00"

	sender := message_model.Message{MessageID: id, InstanceId: "sem-a", Timestamp: ts, Status: "Sent", Source: "b",
		ChatJid: "b@s.whatsapp.net", SenderJid: "a@s.whatsapp.net", MessageType: "text", TextContent: "hi", IsFromMe: true}
	receiver := message_model.Message{MessageID: id, InstanceId: "sem-b", Timestamp: ts, Status: "Received", Source: "a",
		ChatJid: "a@s.whatsapp.net", SenderJid: "a@s.whatsapp.net", MessageType: "text", TextContent: "hi"}
	if err := repo.InsertMessage(sender); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertMessage(receiver); err != nil {
		t.Fatal(err)
	}

	a, err := repo.GetMessageByIDForInstance("sem-a", id)
	if err != nil || a == nil {
		t.Fatalf("sender row: %v %v", a, err)
	}
	b, err := repo.GetMessageByIDForInstance("sem-b", id)
	if err != nil || b == nil {
		t.Fatalf("receiver row: %v %v", b, err)
	}
	if !a.IsFromMe || a.ChatJid != "b@s.whatsapp.net" || a.Status != "Sent" {
		t.Fatalf("sender row was overwritten: %+v", a)
	}
	if b.IsFromMe || b.ChatJid != "a@s.whatsapp.net" || b.Status != "Received" {
		t.Fatalf("receiver row is wrong: %+v", b)
	}

	// A receipt moves only its own instance's row.
	if err := repo.InsertMessage(message_model.Message{MessageID: id, InstanceId: "sem-a", Timestamp: ts, Status: "Read", Source: "b"}); err != nil {
		t.Fatal(err)
	}
	a, _ = repo.GetMessageByIDForInstance("sem-a", id)
	b, _ = repo.GetMessageByIDForInstance("sem-b", id)
	if a.Status != "Read" || b.Status != "Received" {
		t.Fatalf("status leaked across instances: a=%s b=%s", a.Status, b.Status)
	}
}

// The persistence pool writes from several workers at once. With the rollup
// trigger installed, unordered batches failed 94% of the time with deadlocks.
// Timestamps vary across the day so the ordering is tested on the real key, and
// half the writes are status updates, which move rows between counters.
func TestConcurrentBatchesDoNotDeadlockWithRollup(t *testing.T) {
	db := realDB(t)
	if err := EnsureMessageCounters(db); err != nil {
		t.Fatalf("install rollup: %v", err)
	}
	repo := NewMessageRepository(db, WithRollup(true))

	const workers, batches, size = 8, 12, 100
	var mu sync.Mutex
	var failures []string
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(int64(w) + 1))
			for b := 0; b < batches; b++ {
				rows := make([]message_model.Message, size)
				for i := range rows {
					source := fmt.Sprintf("5511%03d", r.Intn(200))
					rows[i] = message_model.Message{
						MessageID: fmt.Sprintf("sem-dl-%d-%d-%d", w, b, i), InstanceId: "sem-dl",
						Timestamp: fmt.Sprintf("2026-10-01 %02d:%02d:00", r.Intn(24), r.Intn(60)),
						Status:    "Received", Source: source, ChatJid: source + "@s.whatsapp.net", MessageType: "text", TextContent: "x",
					}
				}
				if err := repo.InsertMessages(rows); err != nil {
					mu.Lock()
					failures = append(failures, err.Error())
					mu.Unlock()
					continue
				}
				for i := range rows {
					rows[i] = message_model.Message{MessageID: rows[i].MessageID, InstanceId: rows[i].InstanceId,
						Timestamp: rows[i].Timestamp, Source: rows[i].Source, Status: "Read"}
				}
				if err := repo.InsertMessages(rows); err != nil {
					mu.Lock()
					failures = append(failures, err.Error())
					mu.Unlock()
				}
			}
		}(w)
	}
	wg.Wait()
	db.Exec("DELETE FROM messages WHERE message_id LIKE 'sem-dl-%'")

	deadlocks := 0
	for _, f := range failures {
		if strings.Contains(f, "deadlock") {
			deadlocks++
		}
	}
	t.Logf("%d statements, %d failed, %d deadlocks", workers*batches*2, len(failures), deadlocks)
	if len(failures) != 0 {
		t.Fatalf("%d of %d batches failed; first: %s", len(failures), workers*batches*2, failures[0])
	}
}
