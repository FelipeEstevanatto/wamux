package message_repository

import (
	"fmt"
	"os"
	"testing"

	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Persistence is the hottest database path: every inbound message is one upsert,
// and a history sync produces thousands in a burst. These benchmarks measure the
// single vs batched write against a REAL Postgres (sqlmock cannot show
// throughput). They skip unless EVO_BENCH_POSTGRES_DSN is set, so CI without a
// database still passes.
//
// Run:
//
//	EVO_BENCH_POSTGRES_DSN='postgresql://evolution_user:evolution_pass@localhost:55432/evogo_users?sslmode=disable' \
//	  go test -run=^$ -bench='InsertMessage' -benchmem ./pkg/message/repository/ -benchtime=300x
func benchRepo(b *testing.B) MessageRepository {
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
	b.Cleanup(func() {
		db.Exec("DELETE FROM messages WHERE message_id LIKE 'bench-%'")
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
	})
	return NewMessageRepository(db)
}

func sampleMessages(b *testing.B, n int) []message_model.Message {
	out := make([]message_model.Message, n)
	for i := 0; i < n; i++ {
		out[i] = message_model.Message{
			MessageID:   fmt.Sprintf("bench-%d-%d", b.N, i),
			InstanceId:  "bench-instance",
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

// BenchmarkInsertMessageSingle is the old path: one upsert per message.
func BenchmarkInsertMessageSingle(b *testing.B) {
	repo := benchRepo(b)
	msgs := sampleMessages(b, b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := repo.InsertMessage(msgs[i]); err != nil {
			b.Fatalf("insert: %v", err)
		}
	}
}

// batchSize matches persistedBatchSize in the pool.
const benchBatchSize = 100

// BenchmarkInsertMessagesBatch writes benchBatchSize rows per iteration;
// divide ns/op by benchBatchSize to get per-message cost and compare.
func BenchmarkInsertMessagesBatch(b *testing.B) {
	repo := benchRepo(b)
	total := b.N * benchBatchSize
	msgs := sampleMessages(b, total)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := i * benchBatchSize
		if err := repo.InsertMessages(msgs[start : start+benchBatchSize]); err != nil {
			b.Fatalf("batch insert: %v", err)
		}
	}
}
