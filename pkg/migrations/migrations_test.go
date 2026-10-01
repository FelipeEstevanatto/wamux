package migrations

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// newSQLite returns an in-memory database with a minimal `messages` table so the
// index migrations have something to target.
func newSQLite(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE messages (
		id TEXT PRIMARY KEY,
		message_id TEXT UNIQUE,
		instance_id TEXT,
		chat_jid TEXT,
		"timestamp" TEXT
	)`)
	if err != nil {
		t.Fatalf("create messages: %v", err)
	}
	return db
}

func TestApplyRunsAllStepsOnce(t *testing.T) {
	ctx := context.Background()
	db := newSQLite(t)

	ran, err := Apply(ctx, db, "sqlite")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(ran) != len(Names()) {
		t.Fatalf("first run applied %d steps, want %d", len(ran), len(Names()))
	}

	// Second run must be a no-op: everything is already recorded.
	ran2, err := Apply(ctx, db, "sqlite")
	if err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if len(ran2) != 0 {
		t.Fatalf("second run applied %v, want nothing", ran2)
	}
}

func TestApplyRecordsVersions(t *testing.T) {
	ctx := context.Background()
	db := newSQLite(t)

	if _, err := Apply(ctx, db, "sqlite"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		t.Fatalf("appliedVersions: %v", err)
	}
	for _, name := range Names() {
		if !applied[name] {
			t.Fatalf("migration %s not recorded", name)
		}
	}
}

// The composite index must actually exist after Apply, proving the SQL ran and
// not just that the version row was written.
func TestApplyCreatesCompositeIndex(t *testing.T) {
	ctx := context.Background()
	db := newSQLite(t)

	if _, err := Apply(ctx, db, "sqlite"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	var count int
	err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_messages_instance_chat_ts'`,
	).Scan(&count)
	if err != nil {
		t.Fatalf("query index: %v", err)
	}
	if count != 1 {
		t.Fatalf("composite index not created (count=%d)", count)
	}
}

func TestApplyIsIdempotentOnPartialState(t *testing.T) {
	ctx := context.Background()
	db := newSQLite(t)

	// Apply once, then delete one version row to simulate a migration that ran
	// but was not recorded (e.g. a crash between the DDL and the insert). A
	// re-run must not fail on the already-created index because the statements
	// are idempotent (IF NOT EXISTS).
	if _, err := Apply(ctx, db, "sqlite"); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = ?`, Names()[0]); err != nil {
		t.Fatalf("delete version: %v", err)
	}

	ran, err := Apply(ctx, db, "sqlite")
	if err != nil {
		t.Fatalf("re-Apply after partial state: %v", err)
	}
	if len(ran) != 1 || ran[0] != Names()[0] {
		t.Fatalf("re-ran %v, want just %s", ran, Names()[0])
	}
}

func TestApplyRejectsNilDB(t *testing.T) {
	if _, err := Apply(context.Background(), nil, "postgres"); err == nil {
		t.Fatalf("expected an error for a nil database")
	}
}
