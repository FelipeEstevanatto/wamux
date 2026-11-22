package message_repository

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
)

func historyRow(messageID, text string) message_model.Message {
	return message_model.Message{
		MessageID:   messageID,
		InstanceId:  "inst-history",
		Timestamp:   "2026-01-01 00:00:00",
		Source:      "5511999999999",
		ChatJid:     "5511999999999@s.whatsapp.net",
		SenderJid:   "5511999999999@s.whatsapp.net",
		MessageType: "text",
		TextContent: text,
		Status:      "Received",
	}
}

// One poison row must not discard the rest of the history batch. The chunk is
// retried once, then each row is written alone.
func TestInsertHistoryMessagesFallsBackToRows(t *testing.T) {
	repo, mock := newMockRepo(t)

	// Initial chunk + one retry fail.
	mock.ExpectExec(`INSERT INTO "messages"`).WillReturnError(errors.New("poison row"))
	mock.ExpectExec(`INSERT INTO "messages"`).WillReturnError(errors.New("poison row"))
	// Row-by-row: one fails, one inserts, one is a duplicate already stored.
	mock.ExpectExec(`INSERT INTO "messages"`).WillReturnError(errors.New("poison row"))
	mock.ExpectExec(`INSERT INTO "messages"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "messages"`).WillReturnResult(sqlmock.NewResult(0, 0))

	report, err := repo.InsertHistoryMessages([]message_model.Message{
		historyRow("a", "first"),
		historyRow("b", "second"),
		historyRow("c", "third"),
	})
	if err != nil {
		t.Fatalf("InsertHistoryMessages: %v", err)
	}
	if report.Total != 3 || report.Inserted != 1 || report.SkippedExisting != 1 || report.Failed != 1 {
		t.Fatalf("report = %+v", report)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// Duplicate rows in one batch are collapsed before the write and reported, so a
// caller can tell "already stored" from "sent twice by the phone".
func TestInsertHistoryMessagesCollapsesDuplicates(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectExec(`INSERT INTO "messages"`).WillReturnResult(sqlmock.NewResult(0, 1))

	report, err := repo.InsertHistoryMessages([]message_model.Message{
		historyRow("same", "first"),
		historyRow("same", "second"),
	})
	if err != nil {
		t.Fatalf("InsertHistoryMessages: %v", err)
	}
	if report.Total != 2 || report.Duplicates != 1 || report.Inserted != 1 {
		t.Fatalf("report = %+v", report)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// When every row fails, the caller still receives the counts and an error so it
// can retry the batch later.
func TestInsertHistoryMessagesReportsAllFailed(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectExec(`INSERT INTO "messages"`).WillReturnError(errors.New("down"))
	mock.ExpectExec(`INSERT INTO "messages"`).WillReturnError(errors.New("down"))
	for i := 0; i < 2; i++ {
		mock.ExpectExec(`INSERT INTO "messages"`).WillReturnError(errors.New("down"))
	}

	report, err := repo.InsertHistoryMessages([]message_model.Message{
		historyRow("a", "first"),
		historyRow("b", "second"),
	})
	if err == nil {
		t.Fatal("expected an error when every row failed")
	}
	if report.Failed != 2 || report.Inserted != 0 {
		t.Fatalf("report = %+v", report)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
