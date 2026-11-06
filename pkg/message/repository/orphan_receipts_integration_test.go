package message_repository

import (
	"testing"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
)

const orphanInstance = "sem-orphan"

func countedTotal(t *testing.T, repo MessageRepository) int64 {
	t.Helper()
	total, err := repo.CountByInstance(orphanInstance)
	if err != nil {
		t.Fatal(err)
	}
	return total
}

func orphanRepo(t *testing.T) (MessageRepository, func(id, day string) message_model.Message) {
	t.Helper()
	db := realDB(t)
	if err := EnsureMessageCounters(db); err != nil {
		t.Fatalf("EnsureMessageCounters: %v", err)
	}
	db.Exec(`DELETE FROM messages WHERE instance_id = ?`, orphanInstance)
	t.Cleanup(func() { db.Exec(`DELETE FROM messages WHERE instance_id = ?`, orphanInstance) })

	repo := NewMessageRepository(db, WithRollup(true))
	receipt := func(id, day string) message_model.Message {
		return message_model.Message{MessageID: id, InstanceId: orphanInstance, Status: "Read", Source: "100", Timestamp: day + " 12:00:00"}
	}
	return repo, receipt
}

func orphanContent(id string) message_model.Message {
	return message_model.Message{
		MessageID: id, InstanceId: orphanInstance, Status: "Sent", Source: "100",
		Timestamp: "2026-10-01 12:00:00", ChatJid: "100@s.whatsapp.net", SenderJid: "100@s.whatsapp.net",
		MessageType: "text", TextContent: "hello",
	}
}

func TestReceiptWithoutAMessageIsNotCounted(t *testing.T) {
	repo, receipt := orphanRepo(t)

	if err := repo.InsertMessage(receipt("sem-orphan-1", "2026-10-01")); err != nil {
		t.Fatal(err)
	}

	if got := countedTotal(t, repo); got != 0 {
		t.Fatalf("counted %d", got)
	}
}

// The reason these rows exist: a receipt can beat its message here.
func TestMessageArrivingAfterItsReceiptIsCountedOnceWithTheStatus(t *testing.T) {
	repo, receipt := orphanRepo(t)

	if err := repo.InsertMessage(receipt("sem-orphan-2", "2026-10-01")); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertMessage(orphanContent("sem-orphan-2")); err != nil {
		t.Fatal(err)
	}

	if got := countedTotal(t, repo); got != 1 {
		t.Fatalf("counted %d", got)
	}
	row, err := repo.GetMessageByIDForInstance(orphanInstance, "sem-orphan-2")
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "Read" || row.TextContent != "hello" {
		t.Fatalf("row = %+v", row)
	}
}

func TestReceiptAfterItsMessageMovesTheCountNotAddsToIt(t *testing.T) {
	repo, receipt := orphanRepo(t)

	if err := repo.InsertMessage(orphanContent("sem-orphan-3")); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertMessage(receipt("sem-orphan-3", "2026-10-01")); err != nil {
		t.Fatal(err)
	}

	if got := countedTotal(t, repo); got != 1 {
		t.Fatalf("counted %d", got)
	}
}

func TestDeleteOrphanReceiptsKeepsMessagesAndRecentReceipts(t *testing.T) {
	repo, receipt := orphanRepo(t)
	for _, row := range []message_model.Message{
		receipt("sem-orphan-old", "2026-10-01"),
		receipt("sem-orphan-new", "2026-10-09"),
		orphanContent("sem-orphan-kept"),
	} {
		if err := repo.InsertMessage(row); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := repo.DeleteOrphanReceipts("2026-10-05 00:00:00"); err != nil {
		t.Fatal(err)
	}

	if row, _ := repo.GetMessageByIDForInstance(orphanInstance, "sem-orphan-old"); row != nil {
		t.Error("an old receipt survived")
	}
	if row, _ := repo.GetMessageByIDForInstance(orphanInstance, "sem-orphan-new"); row == nil {
		t.Error("a recent receipt was deleted, so its message could lose the status")
	}
	if row, _ := repo.GetMessageByIDForInstance(orphanInstance, "sem-orphan-kept"); row == nil {
		t.Error("deleted a real message")
	}
	if got := countedTotal(t, repo); got != 1 {
		t.Fatalf("counted %d", got)
	}
}
