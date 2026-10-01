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

// A content insert must overwrite the content columns (and referral when
// present); a status-only receipt must move only the status, so it can neither
// reorder the message to the receipt time nor erase what it said.
func TestInsertMessageUpsertColumns(t *testing.T) {
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

	content := message_model.Message{
		MessageID:       "msg-1",
		Timestamp:       "2026-05-09 10:00:00",
		Status:          "Received",
		Source:          "1551999999999",
		ChatJid:         "1551999999999@s.whatsapp.net",
		SenderJid:       "1551999999999@s.whatsapp.net",
		MessageType:     "text",
		TextContent:     "hello",
		QuotedMessageID: "msg-0",
		Referral:        referral,
	}

	if err := repo.InsertMessage(content); err != nil {
		t.Fatalf("insert content message: %v", err)
	}

	contentSQL := logBuffer.String()
	for _, want := range []string{
		`"referral"="excluded"."referral"`,
		`"chat_jid"="excluded"."chat_jid"`,
		`"text_content"="excluded"."text_content"`,
		`"message_type"="excluded"."message_type"`,
	} {
		if !strings.Contains(contentSQL, want) {
			t.Fatalf("content upsert SQL missing %s, got %q", want, contentSQL)
		}
	}

	logBuffer.Reset()

	// A receipt carries no content; it must only move the status.
	receipt := message_model.Message{
		MessageID: "msg-1",
		Timestamp: "2026-05-09 10:05:00",
		Status:    "Read",
		Source:    "1551999999999",
	}
	if err := repo.InsertMessage(receipt); err != nil {
		t.Fatalf("insert receipt: %v", err)
	}

	receiptSQL := logBuffer.String()
	if !strings.Contains(receiptSQL, `"status"="excluded"."status"`) {
		t.Fatalf("receipt upsert SQL should update status, got %q", receiptSQL)
	}
	for _, unwanted := range []string{
		`"timestamp"="excluded"."timestamp"`,
		`"text_content"="excluded"."text_content"`,
		`"chat_jid"="excluded"."chat_jid"`,
		`"referral"="excluded"."referral"`,
	} {
		if strings.Contains(receiptSQL, unwanted) {
			t.Fatalf("receipt upsert SQL must not update %s, got %q", unwanted, receiptSQL)
		}
	}

	logBuffer.Reset()

	// A content row without referral must not try to set referral.
	contentNoReferral := content
	contentNoReferral.Referral = nil
	contentNoReferral.TextContent = "edited text"
	if err := repo.InsertMessage(contentNoReferral); err != nil {
		t.Fatalf("insert content without referral: %v", err)
	}
	noReferralSQL := logBuffer.String()
	if strings.Contains(noReferralSQL, `"referral"="excluded"."referral"`) {
		t.Fatalf("content upsert without referral should omit it, got %q", noReferralSQL)
	}
	if !strings.Contains(noReferralSQL, `"text_content"="excluded"."text_content"`) {
		t.Fatalf("content upsert should still update text, got %q", noReferralSQL)
	}
}

func TestListMessagesScopesAndPaginates(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery(`SELECT \* FROM "messages" WHERE \(?instance_id = \$1 AND chat_jid = \$2\)? AND "timestamp" < \$3 ORDER BY "timestamp" DESC,id DESC LIMIT \$4`).
		WithArgs("inst-1", "chat@s.whatsapp.net", "2026-05-09 10:00:00", 25).
		WillReturnRows(sqlmock.NewRows([]string{"id", "message_id", "text_content"}).
			AddRow("uuid-1", "msg-2", "newer").
			AddRow("uuid-2", "msg-1", "older"))

	messages, err := repo.ListMessages("inst-1", "chat@s.whatsapp.net", "2026-05-09 10:00:00", 25)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 2 || messages[0].MessageID != "msg-2" {
		t.Fatalf("ListMessages = %+v, want 2 newest-first rows", messages)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestListMessagesClampsLimitAndSkipsEmptyScopes(t *testing.T) {
	repo, mock := newMockRepo(t)

	// No chat -> no query at all, not a table-wide scan.
	if got, err := repo.ListMessages("inst-1", "", "", 0); err != nil || len(got) != 0 {
		t.Fatalf("empty chat = %v, %v; want no rows and no error", got, err)
	}

	// A zero limit becomes the default, an oversized one is clamped.
	mock.ExpectQuery(`SELECT \* FROM "messages" WHERE \(?instance_id = \$1 AND chat_jid = \$2\)? ORDER BY "timestamp" DESC,id DESC LIMIT \$3`).
		WithArgs("inst-1", "chat@s.whatsapp.net", defaultHistoryLimit).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if _, err := repo.ListMessages("inst-1", "chat@s.whatsapp.net", "", 0); err != nil {
		t.Fatalf("default limit: %v", err)
	}

	mock.ExpectQuery(`SELECT \* FROM "messages" WHERE \(?instance_id = \$1 AND chat_jid = \$2\)? ORDER BY "timestamp" DESC,id DESC LIMIT \$3`).
		WithArgs("inst-1", "chat@s.whatsapp.net", maxHistoryLimit).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	if _, err := repo.ListMessages("inst-1", "chat@s.whatsapp.net", "", 100000); err != nil {
		t.Fatalf("clamped limit: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestListChatsReturnsNewestPerChat(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery(`DISTINCT ON \(chat_jid\)`).
		WithArgs("inst-1", defaultHistoryLimit).
		WillReturnRows(sqlmock.NewRows([]string{"chat_jid", "message_id", "timestamp", "message_type", "text_content", "status", "media_url", "sender_jid", "is_from_me", "message_count"}).
			AddRow("a@s.whatsapp.net", "m2", "2026-05-09 11:00:00", "text", "hi", "Received", "", "a@s.whatsapp.net", false, 3).
			AddRow("b@s.whatsapp.net", "m9", "2026-05-09 09:00:00", "image", "cap", "Sent", "http://x/y.jpg", "me@s.whatsapp.net", true, 5))

	chats, err := repo.ListChats("inst-1", 0)
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 2 {
		t.Fatalf("got %d chats, want 2", len(chats))
	}
	if chats[0].ChatJid != "a@s.whatsapp.net" || chats[0].MessageCount != 3 {
		t.Fatalf("first chat = %+v", chats[0])
	}
	if chats[1].MediaUrl != "http://x/y.jpg" || !chats[1].IsFromMe {
		t.Fatalf("second chat = %+v", chats[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
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

// Cross-tenant isolation: a message id alone must not be readable. The scoped
// lookup has to carry BOTH the instance_id and the message_id in its WHERE, so
// tenant B can never read tenant A's row even knowing the id.
func TestGetMessageByIDForInstanceScopesQuery(t *testing.T) {
	repo, mock := newMockRepo(t)

	mock.ExpectQuery(`SELECT \* FROM "messages" WHERE instance_id = \$1 AND message_id = \$2 ORDER BY "messages"\."id" LIMIT \$3`).
		WithArgs("tenant-b", "shared-id", 1).
		WillReturnRows(sqlmock.NewRows([]string{"message_id", "instance_id"}).
			AddRow("shared-id", "tenant-b"))

	msg, err := repo.GetMessageByIDForInstance("tenant-b", "shared-id")
	if err != nil {
		t.Fatalf("GetMessageByIDForInstance: %v", err)
	}
	if msg == nil || msg.InstanceId != "tenant-b" {
		t.Fatalf("got %+v, want tenant-b's row", msg)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

// A blank instance id must never fall through to an unscoped lookup: it would
// reintroduce exactly the leak this method exists to prevent.
func TestGetMessageByIDForInstanceRejectsBlankInstance(t *testing.T) {
	repo, _ := newMockRepo(t)

	// No query is queued: if the implementation queried, sqlmock would fail the
	// test on an unexpected call.
	for _, tc := range []struct{ instance, id string }{
		{"", "shared-id"},
		{"tenant-b", ""},
		{"", ""},
	} {
		msg, err := repo.GetMessageByIDForInstance(tc.instance, tc.id)
		if err != nil {
			t.Fatalf("blank input returned error: %v", err)
		}
		if msg != nil {
			t.Fatalf("blank instance/message should return nil, got %+v", msg)
		}
	}
}
