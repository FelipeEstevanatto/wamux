package benchmarks

import (
	"fmt"
	"os"
	"testing"

	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Scenario: message persistence. Every inbound message is one upsert and a
// history sync produces thousands in a burst, so the single vs batched write
// cost is the difference between keeping up and falling behind.
//
// These need a REAL Postgres: sqlmock cannot show throughput. They skip unless
// EVO_BENCH_POSTGRES_DSN is set, matching pkg/message/repository.
//
//	EVO_BENCH_POSTGRES_DSN='postgresql://wamux_user:wamux_pass@localhost:55432/wamux_users?sslmode=disable' \
//	  go test -run=^$ -bench='MessageInsert' -benchmem ./benchmarks/ -benchtime=300x

const benchBatchSize = 100

func benchMessageRepo(b *testing.B) message_repository.MessageRepository {
	b.Helper()
	dsn := os.Getenv("EVO_BENCH_POSTGRES_DSN")
	if dsn == "" {
		b.Skip("set EVO_BENCH_POSTGRES_DSN to run persistence benchmarks")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		b.Fatalf("open postgres: %v", err)
	}
	if err := db.AutoMigrate(&message_model.Message{}); err != nil {
		b.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	b.Cleanup(func() {
		db.Exec("DELETE FROM messages WHERE message_id LIKE 'bench-%'")
		if sqlDB != nil {
			sqlDB.Close()
		}
	})
	return message_repository.NewMessageRepository(db)
}

func benchSampleMessages(n int) []message_model.Message {
	out := make([]message_model.Message, n)
	for i := 0; i < n; i++ {
		out[i] = message_model.Message{
			MessageID:   fmt.Sprintf("bench-%d", i),
			InstanceId:  benchInstanceID,
			Timestamp:   "2026-10-01 12:00:00",
			Status:      "Received",
			Source:      "5514991421911",
			ChatJid:     "5514991421911@s.whatsapp.net",
			SenderJid:   "5514991421911@s.whatsapp.net",
			MessageType: "text",
			TextContent: "hello there, this is a benchmark message body",
		}
	}
	return out
}

func BenchmarkMessageInsertSingle(b *testing.B) {
	repo := benchMessageRepo(b)
	msgs := benchSampleMessages(b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := repo.InsertMessage(msgs[i]); err != nil {
			b.Fatalf("insert: %v", err)
		}
	}
}

// BenchmarkMessageInsertBatch writes benchBatchSize rows per iteration; divide
// ns/op by benchBatchSize for the per-message cost.
func BenchmarkMessageInsertBatch(b *testing.B) {
	repo := benchMessageRepo(b)
	total := b.N * benchBatchSize
	msgs := benchSampleMessages(total)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := i * benchBatchSize
		if err := repo.InsertMessages(msgs[start : start+benchBatchSize]); err != nil {
			b.Fatalf("batch insert: %v", err)
		}
	}
}

// TestMessagePersistenceRoundTrip validates the repo writes and reads the same
// row the benchmarks exercise.
func TestMessagePersistenceRoundTrip(t *testing.T) {
	dsn := os.Getenv("EVO_BENCH_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set EVO_BENCH_POSTGRES_DSN to run persistence validation")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	if err := db.AutoMigrate(&message_model.Message{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		db.Exec("DELETE FROM messages WHERE message_id LIKE 'bench-%'")
		if sqlDB != nil {
			sqlDB.Close()
		}
	})

	repo := message_repository.NewMessageRepository(db)
	msg := benchSampleMessages(1)[0]
	msg.MessageID = "bench-roundtrip"
	if err := repo.InsertMessage(msg); err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := repo.GetMessageByIDForInstance(benchInstanceID, msg.MessageID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got == nil || got.TextContent != msg.TextContent {
		t.Fatalf("round trip mismatch: got %#v", got)
	}
}
