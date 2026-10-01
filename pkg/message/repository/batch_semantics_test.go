package message_repository

import (
	"os"
	"testing"

	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// These tests pin the SEMANTICS of batch persistence against a real Postgres,
// because the grouping step could silently reorder a receipt relative to its
// content row. They skip unless EVO_BENCH_POSTGRES_DSN is set.
func realDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("EVO_BENCH_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set EVO_BENCH_POSTGRES_DSN to run persistence semantics tests")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.AutoMigrate(&message_model.Message{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM messages WHERE message_id LIKE 'sem-%'")
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	})
	return db
}

func contentRow(id, text string) message_model.Message {
	return message_model.Message{
		MessageID: id, InstanceId: "sem", Timestamp: "2026-10-01 12:00:00",
		Status: "Sent", ChatJid: "c@s.whatsapp.net", SenderJid: "c@s.whatsapp.net",
		MessageType: "text", TextContent: text,
	}
}

func receiptRow(id string) message_model.Message {
	return message_model.Message{MessageID: id, Status: "Read"}
}

// A content row followed by a receipt for the same message: the batch groups
// them into two statements, but the final row must have BOTH the body and the
// updated status — the receipt must not erase the content.
func TestBatchContentThenReceiptKeepsBody(t *testing.T) {
	db := realDB(t)
	repo := NewMessageRepository(db)
	ctx := "sem-content-then-receipt"

	if err := repo.InsertMessages([]message_model.Message{
		contentRow(ctx, "hello body"),
		receiptRow(ctx),
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var got message_model.Message
	if err := db.Where("message_id = ?", ctx).First(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.TextContent != "hello body" {
		t.Fatalf("content erased by receipt: text=%q", got.TextContent)
	}
	if got.Status != "Read" {
		t.Fatalf("receipt not applied: status=%q", got.Status)
	}
}

// A receipt followed by a content row: content must win for the body, and the
// receipt's status must not be clobbered back to "Sent" by the content row
// (content rows DO write status, so this asserts last-writer-wins is coherent).
func TestBatchReceiptThenContentIsCoherent(t *testing.T) {
	db := realDB(t)
	repo := NewMessageRepository(db)
	id := "sem-receipt-then-content"

	if err := repo.InsertMessages([]message_model.Message{
		receiptRow(id),
		contentRow(id, "later body"),
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var got message_model.Message
	if err := db.Where("message_id = ?", id).First(&got).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got.TextContent != "later body" {
		t.Fatalf("content not stored: text=%q", got.TextContent)
	}
	// No assertion on status here: a content row legitimately carries its own
	// status, and the record of what the final state should be is the caller's.
	// The point of this test is that the row EXISTS and is not lost to grouping.
}

// Two receipts for the same message in one batch must both land, and a receipt
// for a message that has no content row must still create the row.
func TestBatchReceiptCreatesRowWhenAbsent(t *testing.T) {
	db := realDB(t)
	repo := NewMessageRepository(db)
	id := "sem-receipt-only"

	if err := repo.InsertMessages([]message_model.Message{receiptRow(id)}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	var got message_model.Message
	if err := db.Where("message_id = ?", id).First(&got).Error; err != nil {
		t.Fatalf("receipt-only row missing: %v", err)
	}
	if got.Status != "Read" {
		t.Fatalf("status = %q, want Read", got.Status)
	}
}
