package message_repository

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newMockRepo builds a repository backed by sqlmock.
func newMockRepo(t *testing.T, opts ...Option) (MessageRepository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}
	return NewMessageRepository(gormDB, opts...), mock
}

// expectStatsQueries queues the four queries GetStats runs.
func expectStatsQueries(mock sqlmock.Sqlmock, total int64) {
	mock.ExpectQuery(`SELECT count\(\*\) FROM "messages"`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(total))
	mock.ExpectQuery(`SELECT status as label`).
		WillReturnRows(sqlmock.NewRows([]string{"label", "total"}).AddRow("Received", total))
	mock.ExpectQuery(`substr\("timestamp", 1, 10\) as label`).
		WillReturnRows(sqlmock.NewRows([]string{"label", "total"}).AddRow("2026-09-23", total))
	mock.ExpectQuery(`SELECT source as label`).
		WillReturnRows(sqlmock.NewRows([]string{"label", "total"}).AddRow("5514991421911", total))
}

// The dashboard polls these aggregations every 15s per client. With the cache on,
// a second call must not touch the database at all — sqlmock errors on any query
// that was not expected.
func TestGetStatsIsCached(t *testing.T) {
	repo, mock := newMockRepo(t, WithAggregateCacheTTL(time.Minute))
	expectStatsQueries(mock, 42)

	first, err := repo.GetStats()
	if err != nil {
		t.Fatalf("first GetStats: %v", err)
	}
	if first.Total != 42 {
		t.Fatalf("total = %d, want 42", first.Total)
	}

	second, err := repo.GetStats()
	if err != nil {
		t.Fatalf("second GetStats (should be cached): %v", err)
	}
	if second.Total != 42 || len(second.TopSources) != 1 {
		t.Fatalf("cached stats = %+v, want the first result", second)
	}
	// Mutating the returned value must not corrupt what is cached.
	second.TopSources[0].Count = 999
	third, _ := repo.GetStats()
	if third.TopSources[0].Count == 999 {
		t.Fatal("cached stats were mutated through the returned value")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// Without the option every call hits the database (the old behaviour), which is
// what unit tests and one-shot callers want.
func TestAggregateCacheDisabledByDefault(t *testing.T) {
	repo, mock := newMockRepo(t)
	expectStatsQueries(mock, 1)
	expectStatsQueries(mock, 1)

	for i := 0; i < 2; i++ {
		if _, err := repo.GetStats(); err != nil {
			t.Fatalf("GetStats #%d: %v", i+1, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expected two uncached round trips: %v", err)
	}
}

func TestCountChatsByInstanceIsCached(t *testing.T) {
	repo, mock := newMockRepo(t, WithAggregateCacheTTL(time.Minute))
	mock.ExpectQuery(`SELECT COUNT\(DISTINCT source\) FROM "messages" WHERE instance_id = \$1`).
		WithArgs("inst-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))

	for i := 0; i < 3; i++ {
		total, err := repo.CountChatsByInstance("inst-1")
		if err != nil {
			t.Fatalf("CountChatsByInstance #%d: %v", i+1, err)
		}
		if total != 4 {
			t.Fatalf("total = %d, want 4", total)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// Deleting rows makes a cached count wrong rather than merely late, so the cache
// must be dropped.
func TestDeleteAllMessagesInvalidatesCache(t *testing.T) {
	repo, mock := newMockRepo(t, WithAggregateCacheTTL(time.Minute))
	expectStatsQueries(mock, 42)
	if _, err := repo.GetStats(); err != nil {
		t.Fatalf("GetStats: %v", err)
	}

	mock.ExpectExec(`DELETE FROM messages`).WillReturnResult(sqlmock.NewResult(0, 42))
	if _, err := repo.DeleteAllMessages(); err != nil {
		t.Fatalf("DeleteAllMessages: %v", err)
	}

	// Must query again after the invalidation.
	expectStatsQueries(mock, 0)
	if stats, err := repo.GetStats(); err != nil || stats.Total != 0 {
		t.Fatalf("GetStats after delete = %+v, %v; want total 0 from a fresh query", stats, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInsertMessagePreservesReferralOnStatusUpdate(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	defer sqlDB.Close()

	var logBuffer bytes.Buffer
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn:             sqlDB,
		WithoutReturning: true,
	}), &gorm.Config{
		DryRun:                 true,
		SkipDefaultTransaction: true,
		Logger: gormlogger.New(
			log.New(&logBuffer, "", 0),
			gormlogger.Config{LogLevel: gormlogger.Info, Colorful: false},
		),
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}

	repo := NewMessageRepository(gormDB)
	referral := json.RawMessage(`{"ctwaClid":"abc123","showAdAttribution":true}`)

	initial := message_model.Message{
		MessageID: "msg-1",
		Timestamp: "2026-05-09 10:00:00",
		Status:    "Received",
		Source:    "1551999999999",
		Referral:  referral,
	}

	if err := repo.InsertMessage(initial); err != nil {
		t.Fatalf("insert initial message: %v", err)
	}

	initialSQL := logBuffer.String()
	if !strings.Contains(initialSQL, `"referral"="excluded"."referral"`) {
		t.Fatalf("expected initial upsert SQL to update referral, got %q", initialSQL)
	}

	logBuffer.Reset()

	updated := message_model.Message{
		MessageID: "msg-1",
		Timestamp: "2026-05-09 10:05:00",
		Status:    "Read",
		Source:    "1551999999999",
	}

	if err := repo.InsertMessage(updated); err != nil {
		t.Fatalf("insert updated message: %v", err)
	}

	updatedSQL := logBuffer.String()
	if strings.Contains(updatedSQL, `"referral"="excluded"."referral"`) {
		t.Fatalf("expected updated upsert SQL to omit referral update, got %q", updatedSQL)
	}
	if !strings.Contains(updatedSQL, `"timestamp"="excluded"."timestamp","status"="excluded"."status","source"="excluded"."source"`) {
		t.Fatalf("expected updated upsert SQL to keep core columns, got %q", updatedSQL)
	}
}

func TestCountByInstanceScopesToTheInstance(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	defer sqlDB.Close()

	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}

	repo := NewMessageRepository(gormDB)

	mock.ExpectQuery(`SELECT count\(\*\) FROM "messages" WHERE instance_id = \$1`).
		WithArgs("inst-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))

	total, err := repo.CountByInstance("inst-1")
	if err != nil {
		t.Fatalf("CountByInstance: %v", err)
	}
	if total != 7 {
		t.Fatalf("CountByInstance = %d, want 7", total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestCountChatsByInstanceCountsDistinctSources(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	defer sqlDB.Close()

	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}

	repo := NewMessageRepository(gormDB)

	mock.ExpectQuery(`SELECT COUNT\(DISTINCT source\) FROM "messages" WHERE instance_id = \$1`).
		WithArgs("inst-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))

	total, err := repo.CountChatsByInstance("inst-1")
	if err != nil {
		t.Fatalf("CountChatsByInstance: %v", err)
	}
	if total != 4 {
		t.Fatalf("CountChatsByInstance = %d, want 4", total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// expectCounterStatsQueries queues the four queries GetStats runs against the
// message_counters rollup.
func expectCounterStatsQueries(mock sqlmock.Sqlmock, total int64) {
	mock.ExpectQuery(`SELECT COALESCE\(sum\(total\), 0\)::bigint FROM message_counters`).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(total))
	mock.ExpectQuery(`SELECT status AS label, sum\(total\)::bigint AS total FROM message_counters`).
		WillReturnRows(sqlmock.NewRows([]string{"label", "total"}).AddRow("Received", total))
	mock.ExpectQuery(`SELECT day AS label, sum\(total\)::bigint AS total FROM message_counters`).
		WillReturnRows(sqlmock.NewRows([]string{"label", "total"}).AddRow("2026-09-23", total))
	mock.ExpectQuery(`SELECT source AS label, sum\(total\)::bigint AS total FROM message_counters`).
		WillReturnRows(sqlmock.NewRows([]string{"label", "total"}).AddRow("5514991421911", total))
}

// With the rollup installed the aggregates come from message_counters, and they
// are NOT cached: a second call must re-query, which is the whole point of the
// rollup (fresh numbers, no TTL staleness).
func TestGetStatsUsesTheRollupAndStaysFresh(t *testing.T) {
	repo, mock := newMockRepo(t, WithRollup(true), WithAggregateCacheTTL(time.Minute))

	expectCounterStatsQueries(mock, 42)
	first, err := repo.GetStats()
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if first.Total != 42 || len(first.TopSources) != 1 {
		t.Fatalf("rollup stats = %+v, want total 42 and one source", first)
	}

	// A different value proves the second call is not served from the cache.
	expectCounterStatsQueries(mock, 43)
	second, err := repo.GetStats()
	if err != nil {
		t.Fatalf("second GetStats: %v", err)
	}
	if second.Total != 43 {
		t.Fatalf("total = %d, want 43 (rollup results must not be cached)", second.Total)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// A rollup that fails must not break the dashboard: it degrades to the live
// aggregation and stops trying.
func TestRollupFailureFallsBackToLive(t *testing.T) {
	repo, mock := newMockRepo(t, WithRollup(true))

	mock.ExpectQuery(`SELECT COALESCE\(sum\(total\), 0\)::bigint FROM message_counters`).
		WillReturnError(errors.New(`relation "message_counters" does not exist`))
	expectStatsQueries(mock, 7)

	stats, err := repo.GetStats()
	if err != nil {
		t.Fatalf("GetStats should have fallen back: %v", err)
	}
	if stats.Total != 7 {
		t.Fatalf("total = %d, want 7 from the live fallback", stats.Total)
	}

	// The rollup is now disabled for the process, so this goes straight live
	// (no further attempt at the counter table).
	expectStatsQueries(mock, 7)
	if _, err := repo.GetStats(); err != nil {
		t.Fatalf("second GetStats: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestCountsUseTheRollup(t *testing.T) {
	repo, mock := newMockRepo(t, WithRollup(true))

	mock.ExpectQuery(`SELECT COALESCE\(sum\(total\), 0\)::bigint FROM message_counters WHERE instance_id = \$1`).
		WithArgs("inst-1").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(7))
	mock.ExpectQuery(`SELECT count\(DISTINCT source\)::bigint FROM message_counters WHERE instance_id = \$1`).
		WithArgs("inst-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	if total, err := repo.CountByInstance("inst-1"); err != nil || total != 7 {
		t.Fatalf("CountByInstance = %d, %v; want 7", total, err)
	}
	if total, err := repo.CountChatsByInstance("inst-1"); err != nil || total != 3 {
		t.Fatalf("CountChatsByInstance = %d, %v; want 3", total, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// for the whole sweep, and it must drop the cached aggregates.
// The retention delete runs in batches so a year of backlog cannot hold locks
// for the whole sweep, and it must drop the cached aggregates.
func TestDeleteMessagesOlderThanBatchesAndInvalidatesCache(t *testing.T) {
	repo, mock := newMockRepo(t, WithAggregateCacheTTL(time.Minute))

	// Prime the cache so we can prove the delete flushes it.
	expectStatsQueries(mock, 42)
	if _, err := repo.GetStats(); err != nil {
		t.Fatalf("GetStats: %v", err)
	}

	deleteSQL := `DELETE FROM messages WHERE id IN \(`
	// A full batch means "there may be more", so the loop runs again.
	mock.ExpectExec(deleteSQL).
		WithArgs("2025-09-23 00:00:00", int64(deleteBatchSize)).
		WillReturnResult(sqlmock.NewResult(0, deleteBatchSize))
	// A short batch ends the loop.
	mock.ExpectExec(deleteSQL).
		WithArgs("2025-09-23 00:00:00", int64(deleteBatchSize)).
		WillReturnResult(sqlmock.NewResult(0, 12))

	deleted, err := repo.DeleteMessagesOlderThan("2025-09-23 00:00:00")
	if err != nil {
		t.Fatalf("DeleteMessagesOlderThan: %v", err)
	}
	if want := int64(deleteBatchSize + 12); deleted != want {
		t.Fatalf("deleted = %d, want %d", deleted, want)
	}

	// The cache must have been flushed, so this has to query again.
	expectStatsQueries(mock, 0)
	if stats, err := repo.GetStats(); err != nil || stats.Total != 0 {
		t.Fatalf("GetStats after cleanup = %+v, %v; want a fresh query with total 0", stats, err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
