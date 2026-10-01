package message_repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
)

// A batch of content rows must go out as ONE statement, not N. This is the whole
// point of batching (fewer round trips), so assert the single multi-row insert.
func TestInsertMessagesBatchesHomogeneousRows(t *testing.T) {
	repo, mock := newMockRepo(t)

	msgs := []message_model.Message{
		{MessageID: "m1", InstanceId: "i", ChatJid: "c@s.whatsapp.net", MessageType: "text", TextContent: "a"},
		{MessageID: "m2", InstanceId: "i", ChatJid: "c@s.whatsapp.net", MessageType: "text", TextContent: "b"},
		{MessageID: "m3", InstanceId: "i", ChatJid: "c@s.whatsapp.net", MessageType: "text", TextContent: "c"},
	}

	// One INSERT with three VALUES tuples. The mock repo uses
	// SkipDefaultTransaction, so there is no Begin/Commit.
	mock.ExpectExec(`INSERT INTO "messages"`).
		WillReturnResult(sqlmock.NewResult(0, 3))

	if err := repo.InsertMessages(msgs); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expected a single batched insert: %v", err)
	}
}

// A status-only receipt and a content row must NOT share a statement: their
// ON CONFLICT SET lists differ (the receipt must not overwrite content). The
// batch is split into two statements, preserving the receipt semantics.
func TestInsertMessagesSplitsMixedUpdateColumns(t *testing.T) {
	repo, mock := newMockRepo(t)

	content := message_model.Message{MessageID: "m1", ChatJid: "c@s.whatsapp.net", MessageType: "text", TextContent: "hi"}
	receipt := message_model.Message{MessageID: "m1", Status: "Read"} // status-only

	mock.ExpectExec(`INSERT INTO "messages"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "messages"`).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.InsertMessages([]message_model.Message{content, receipt}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expected two grouped statements (content vs status-only): %v", err)
	}
}

func TestInsertMessagesEmptyIsNoop(t *testing.T) {
	repo, mock := newMockRepo(t)
	// No expectations queued: a no-op must not touch the DB.
	if err := repo.InsertMessages(nil); err != nil {
		t.Fatalf("InsertMessages(nil): %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("empty batch should be a no-op: %v", err)
	}
}
