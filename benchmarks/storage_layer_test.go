package benchmarks

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Scenario: the storage layer. WaMux uses GORM; apime uses hand-written SQL
// (pgx/lib/pq). These benchmarks run the same upsert and read against the same
// Postgres table, once through the repository (GORM) and once through
// database/sql, to quantify the ORM overhead on the hot message path.
//
//	EVO_BENCH_POSTGRES_DSN='postgresql://user:pass@localhost:55432/bench?sslmode=disable' \
//	  go test -run=^$ -bench='Message.*(GORM|RawSQL)' ./benchmarks/ -benchtime=300x

const rawInsertMessageSQL = `
INSERT INTO messages
  (id, message_id, instance_id, timestamp, status, source, chat_jid, sender_jid,
   message_type, text_content, media_url, media_mimetype, quoted_message_id, is_from_me)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (message_id) DO UPDATE SET
  timestamp=EXCLUDED.timestamp, status=EXCLUDED.status, source=EXCLUDED.source,
  chat_jid=EXCLUDED.chat_jid, sender_jid=EXCLUDED.sender_jid,
  message_type=EXCLUDED.message_type, text_content=EXCLUDED.text_content,
  media_url=EXCLUDED.media_url, media_mimetype=EXCLUDED.media_mimetype,
  quoted_message_id=EXCLUDED.quoted_message_id, is_from_me=EXCLUDED.is_from_me`

const rawReadMessageSQL = `
SELECT id, message_id, instance_id, timestamp, status, source, chat_jid, sender_jid,
       message_type, text_content, media_url, media_mimetype, quoted_message_id, is_from_me
FROM messages WHERE instance_id=$1 AND message_id=$2`

// rawBatchColumnTypes mirrors the column order in the insert above; the casts
// are required for multi-row VALUES with lib/pq.
var rawBatchColumnTypes = []string{
	"uuid", "text", "text", "text", "text", "text", "text",
	"text", "text", "text", "text", "text", "text", "boolean",
}

func benchRawSQL(b *testing.B) *sql.DB {
	b.Helper()
	dsn := os.Getenv("EVO_BENCH_POSTGRES_DSN")
	if dsn == "" {
		b.Skip("set EVO_BENCH_POSTGRES_DSN to run the storage-layer benchmarks")
	}
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		b.Fatalf("open gorm: %v", err)
	}
	if err := gdb.AutoMigrate(&message_model.Message{}); err != nil {
		b.Fatalf("migrate: %v", err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		b.Fatalf("open sql: %v", err)
	}
	db.SetMaxOpenConns(10)
	b.Cleanup(func() {
		gdb.Exec("DELETE FROM messages WHERE message_id LIKE 'bench-%'")
		_ = db.Close()
	})
	return db
}

func rawInsertMessage(db *sql.DB, m message_model.Message) error {
	if m.Id == "" {
		m.Id = uuid.NewString()
	}
	_, err := db.Exec(rawInsertMessageSQL,
		m.Id, m.MessageID, m.InstanceId, m.Timestamp, m.Status, m.Source,
		m.ChatJid, m.SenderJid, m.MessageType, m.TextContent,
		m.MediaUrl, m.MediaMimetype, m.QuotedMessageID, m.IsFromMe)
	return err
}

func BenchmarkMessageInsertGORM(b *testing.B) {
	repo := benchMessageRepo(b)
	msgs := benchSampleMessages(b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := repo.InsertMessage(msgs[i]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMessageInsertRawSQL(b *testing.B) {
	db := benchRawSQL(b)
	msgs := benchSampleMessages(b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := rawInsertMessage(db, msgs[i]); err != nil {
			b.Fatal(err)
		}
	}
}

// rawBatchInsert builds one multi-row upsert, the way a hand-written batch path
// would.
func rawBatchInsert(db *sql.DB, msgs []message_model.Message) error {
	var sb strings.Builder
	sb.WriteString(`INSERT INTO messages
  (id, message_id, instance_id, timestamp, status, source, chat_jid, sender_jid,
   message_type, text_content, media_url, media_mimetype, quoted_message_id, is_from_me) VALUES `)
	args := make([]any, 0, len(msgs)*14)
	for i, m := range msgs {
		if m.Id == "" {
			m.Id = uuid.NewString()
		}
		if i > 0 {
			sb.WriteByte(',')
		}
		n := i * 14
		sb.WriteString("(")
		for j := 1; j <= 14; j++ {
			if j > 1 {
				sb.WriteByte(',')
			}
			sb.WriteByte('$')
			sb.WriteString(itoa(n + j))
			// Postgres cannot infer parameter types inside a multi-row VALUES,
			// so every placeholder is cast to its column type.
			sb.WriteString("::")
			sb.WriteString(rawBatchColumnTypes[j-1])
		}
		sb.WriteByte(')')
		args = append(args, m.Id, m.MessageID, m.InstanceId, m.Timestamp, m.Status, m.Source,
			m.ChatJid, m.SenderJid, m.MessageType, m.TextContent,
			m.MediaUrl, m.MediaMimetype, m.QuotedMessageID, m.IsFromMe)
	}
	sb.WriteString(` ON CONFLICT (message_id) DO UPDATE SET
  timestamp=EXCLUDED.timestamp, status=EXCLUDED.status, source=EXCLUDED.source,
  chat_jid=EXCLUDED.chat_jid, sender_jid=EXCLUDED.sender_jid,
  message_type=EXCLUDED.message_type, text_content=EXCLUDED.text_content,
  media_url=EXCLUDED.media_url, media_mimetype=EXCLUDED.media_mimetype,
  quoted_message_id=EXCLUDED.quoted_message_id, is_from_me=EXCLUDED.is_from_me`)
	_, err := db.Exec(sb.String(), args...)
	return err
}

func BenchmarkMessageBatchGORM(b *testing.B) {
	repo := benchMessageRepo(b)
	total := b.N * benchBatchSize
	msgs := benchSampleMessages(total)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := i * benchBatchSize
		if err := repo.InsertMessages(msgs[start : start+benchBatchSize]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMessageBatchRawSQL(b *testing.B) {
	db := benchRawSQL(b)
	total := b.N * benchBatchSize
	msgs := benchSampleMessages(total)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := i * benchBatchSize
		if err := rawBatchInsert(db, msgs[start:start+benchBatchSize]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMessageReadGORM(b *testing.B) {
	repo := benchMessageRepo(b)
	msg := benchSampleMessages(1)[0]
	if err := repo.InsertMessage(msg); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := repo.GetMessageByIDForInstance(benchInstanceID, msg.MessageID); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMessageReadRawSQL(b *testing.B) {
	db := benchRawSQL(b)
	msg := benchSampleMessages(1)[0]
	if err := rawInsertMessage(db, msg); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var m message_model.Message
		err := db.QueryRow(rawReadMessageSQL, benchInstanceID, msg.MessageID).Scan(
			&m.Id, &m.MessageID, &m.InstanceId, &m.Timestamp, &m.Status, &m.Source,
			&m.ChatJid, &m.SenderJid, &m.MessageType, &m.TextContent,
			&m.MediaUrl, &m.MediaMimetype, &m.QuotedMessageID, &m.IsFromMe)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestStorageLayerEquivalence validates that the raw SQL path and the GORM
// repository read and write the same rows.
func TestStorageLayerEquivalence(t *testing.T) {
	dsn := os.Getenv("EVO_BENCH_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set EVO_BENCH_POSTGRES_DSN to run the storage-layer validation")
	}
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	if err := gdb.AutoMigrate(&message_model.Message{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open sql: %v", err)
	}
	t.Cleanup(func() {
		gdb.Exec("DELETE FROM messages WHERE message_id LIKE 'bench-%'")
		_ = db.Close()
	})
	repo := message_repository.NewMessageRepository(gdb)

	viaGorm := benchSampleMessages(1)[0]
	viaGorm.MessageID = "bench-eq-gorm"
	if err := repo.InsertMessage(viaGorm); err != nil {
		t.Fatalf("gorm insert: %v", err)
	}
	var read message_model.Message
	if err := db.QueryRow(rawReadMessageSQL, viaGorm.InstanceId, viaGorm.MessageID).Scan(
		&read.Id, &read.MessageID, &read.InstanceId, &read.Timestamp, &read.Status, &read.Source,
		&read.ChatJid, &read.SenderJid, &read.MessageType, &read.TextContent,
		&read.MediaUrl, &read.MediaMimetype, &read.QuotedMessageID, &read.IsFromMe); err != nil {
		t.Fatalf("raw read of gorm row: %v", err)
	}
	if read.TextContent != viaGorm.TextContent {
		t.Fatalf("content mismatch: %q vs %q", read.TextContent, viaGorm.TextContent)
	}

	viaRaw := benchSampleMessages(1)[0]
	viaRaw.MessageID = "bench-eq-raw"
	if err := rawInsertMessage(db, viaRaw); err != nil {
		t.Fatalf("raw insert: %v", err)
	}
	got, err := repo.GetMessageByIDForInstance(viaRaw.InstanceId, viaRaw.MessageID)
	if err != nil {
		t.Fatalf("gorm read of raw row: %v", err)
	}
	if got == nil || got.TextContent != viaRaw.TextContent {
		t.Fatalf("raw row not visible through the repository: %#v", got)
	}
}
