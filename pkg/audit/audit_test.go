package audit

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func newAuditDB(t *testing.T) (*Logger, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{SkipDefaultTransaction: true})
	if err != nil {
		t.Fatalf("gorm: %v", err)
	}
	return &Logger{db: db}, mock
}

func TestRecordAndList(t *testing.T) {
	log, mock := newAuditDB(t)
	mock.ExpectExec(`INSERT INTO audit_logs`).
		WithArgs(sqlmock.AnyArg(), "admin", "instance.create", "inst-1", `{"name":"sales"}`, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	log.record("admin", "instance.create", "inst-1", map[string]any{"name": "sales"})

	now := time.Now().UTC()
	mock.ExpectQuery(`SELECT id, actor, action, target, COALESCE\(detail, ''\) AS detail, created_at`).
		WithArgs(10).
		WillReturnRows(sqlmock.NewRows([]string{"id", "actor", "action", "target", "detail", "created_at"}).
			AddRow("a1", "admin", "instance.create", "inst-1", `{"name":"sales"}`, now))

	entries, err := log.list(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 1 || entries[0].Action != "instance.create" || entries[0].Target != "inst-1" {
		t.Fatalf("entries = %+v", entries)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestRecordWithoutDatabaseIsBestEffort(t *testing.T) {
	log := &Logger{}
	log.record("admin", "instance.delete", "inst-1", nil)
	if _, err := log.list(10); err == nil {
		t.Fatal("list without a database should report that it is not configured")
	}
}

func TestRecordIgnoresEmptyAction(t *testing.T) {
	log, mock := newAuditDB(t)
	log.record("admin", "  ", "inst-1", nil)
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("an empty action must not write: %v", err)
	}
}

func TestTokenReadsAreDedupedPerMinute(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	log := &Logger{recent: map[string]time.Time{}, now: func() time.Time { return now }}

	if log.duplicateRead("admin", "all") {
		t.Fatal("first read must be recorded")
	}
	if !log.duplicateRead("admin", "all") {
		t.Fatal("a second read inside the window must be deduped")
	}
	if log.duplicateRead("admin", "inst-1") {
		t.Fatal("a different target must be recorded")
	}
	now = now.Add(readDedupeTTL)
	if log.duplicateRead("admin", "all") {
		t.Fatal("a read after the window must be recorded")
	}
}
