package message_repository

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDistinctSendersScopesToInstanceAndChat(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery(`SELECT DISTINCT sender_jid FROM messages`).
		WithArgs("inst-1", "123@g.us").
		WillReturnRows(sqlmock.NewRows([]string{"sender_jid"}).
			AddRow("5511888888888").
			AddRow("5511777777777"))

	got, err := repo.DistinctSenders("inst-1", "123@g.us")
	if err != nil {
		t.Fatalf("DistinctSenders: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("DistinctSenders = %v, want 2", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDistinctSendersEmptyChatIsNoop(t *testing.T) {
	repo, mock := newMockRepo(t)
	got, err := repo.DistinctSenders("inst-1", "")
	if err != nil || got != nil {
		t.Fatalf("DistinctSenders(inst, \"\") = %v, %v; want nil, nil", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
