package benchmarks

import (
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// Scenario: the cost of the Idempotency-Key layer apime ships and WaMux does
// not. Two store backends are compared: Postgres (apime's default) and Redis
// (apime when REDIS_ENABLED), plus an in-process map as the lower bound.
//
//	EVO_BENCH_POSTGRES_DSN=... EVO_BENCH_REDIS_ADDR=127.0.0.1:6379 \
//	  go test -run=^$ -bench='Idempotency' ./benchmarks/ -benchtime=2000x

var errIdempotencyConflict = errors.New("idempotency: key reused with a different payload")

const benchIdempotencyTTL = 24 * time.Hour

func benchIdempotencyDB(b *testing.B) *sql.DB {
	b.Helper()
	dsn := os.Getenv("EVO_BENCH_POSTGRES_DSN")
	if dsn == "" {
		b.Skip("set EVO_BENCH_POSTGRES_DSN to run the idempotency benchmarks")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		b.Fatalf("open sql: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS bench_idempotency (
		key text PRIMARY KEY,
		request_hash text NOT NULL,
		status text NOT NULL,
		response_code int NOT NULL DEFAULT 0,
		response_body bytea,
		expires_at timestamptz NOT NULL)`); err != nil {
		b.Fatalf("create table: %v", err)
	}
	b.Cleanup(func() {
		db.Exec("DELETE FROM bench_idempotency")
		_ = db.Close()
	})
	return db
}

// idempotencyPostgresFirst is a miss: SELECT then INSERT.
func idempotencyPostgresFirst(db *sql.DB, key, hash string, body []byte) error {
	var existing string
	err := db.QueryRow("SELECT request_hash FROM bench_idempotency WHERE key=$1", key).Scan(&existing)
	switch {
	case err == nil:
		if existing != hash {
			return errIdempotencyConflict
		}
		return nil
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	_, err = db.Exec(`INSERT INTO bench_idempotency (key, request_hash, status, response_code, response_body, expires_at)
		VALUES ($1,$2,'done',200,$3, now() + interval '24 hours')
		ON CONFLICT (key) DO NOTHING`, key, hash, body)
	return err
}

// idempotencyPostgresReplay is a hit: one SELECT.
func idempotencyPostgresReplay(db *sql.DB, key string) (int, []byte, bool, error) {
	var code int
	var body []byte
	err := db.QueryRow(`SELECT response_code, response_body FROM bench_idempotency
		WHERE key=$1 AND expires_at > now()`, key).Scan(&code, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, false, nil
	}
	if err != nil {
		return 0, nil, false, err
	}
	return code, body, true, nil
}

func BenchmarkIdempotencyPostgresFirst(b *testing.B) {
	db := benchIdempotencyDB(b)
	body := []byte(`{"ok":true}`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "bench-" + itoa(i)
		if err := idempotencyPostgresFirst(db, key, "hash", body); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIdempotencyPostgresReplay(b *testing.B) {
	db := benchIdempotencyDB(b)
	const key = "bench-replay"
	if err := idempotencyPostgresFirst(db, key, "hash", []byte(`{"ok":true}`)); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, ok, err := idempotencyPostgresReplay(db, key); err != nil || !ok {
			b.Fatalf("replay: ok=%v err=%v", ok, err)
		}
	}
}

// memIdempotency is the in-process lower bound (what a single-node cache costs).
type memIdempotency struct {
	mu sync.Mutex
	m  map[string]string
}

func newMemIdempotency() *memIdempotency { return &memIdempotency{m: make(map[string]string)} }

func (x *memIdempotency) first(key, hash string) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	if existing, ok := x.m[key]; ok {
		if existing != hash {
			return errIdempotencyConflict
		}
		return nil
	}
	x.m[key] = hash
	return nil
}

func (x *memIdempotency) replay(key string) (bool, error) {
	x.mu.Lock()
	defer x.mu.Unlock()
	_, ok := x.m[key]
	return ok, nil
}

func BenchmarkIdempotencyInMemory(b *testing.B) {
	x := newMemIdempotency()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := x.first("bench-"+itoa(i), "hash"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIdempotencyRedisFirst(b *testing.B) {
	rc := dialBenchRedis(b)
	defer rc.close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := rc.setNX("bench-idem-"+itoa(i), "hash", benchIdempotencyTTL); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIdempotencyRedisReplay(b *testing.B) {
	rc := dialBenchRedis(b)
	defer rc.close()
	const key = "bench-idem-replay"
	if _, err := rc.setNX(key, "hash", benchIdempotencyTTL); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok, err := rc.get(key); err != nil || !ok {
			b.Fatalf("redis replay: ok=%v err=%v", ok, err)
		}
	}
}

// TestIdempotencySemantics validates first/replay/conflict/expiry for the
// Postgres path and the in-memory path.
func TestIdempotencySemantics(t *testing.T) {
	// in-memory
	x := newMemIdempotency()
	if err := x.first("k", "h1"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := x.first("k", "h1"); err != nil {
		t.Fatalf("same hash should be a no-op: %v", err)
	}
	if err := x.first("k", "h2"); !errors.Is(err, errIdempotencyConflict) {
		t.Fatalf("different hash should conflict, got %v", err)
	}

	dsn := os.Getenv("EVO_BENCH_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set EVO_BENCH_POSTGRES_DSN for the Postgres half")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS bench_idempotency (
		key text PRIMARY KEY, request_hash text NOT NULL, status text NOT NULL,
		response_code int NOT NULL DEFAULT 0, response_body bytea, expires_at timestamptz NOT NULL)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer db.Exec("DELETE FROM bench_idempotency WHERE key LIKE 'bench-test-%'")

	if err := idempotencyPostgresFirst(db, "bench-test-1", "h1", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := idempotencyPostgresFirst(db, "bench-test-1", "h2", []byte(`{"ok":true}`)); !errors.Is(err, errIdempotencyConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	code, _, ok, err := idempotencyPostgresReplay(db, "bench-test-1")
	if err != nil || !ok || code != 200 {
		t.Fatalf("replay: code=%d ok=%v err=%v", code, ok, err)
	}
	// Expired keys are a miss.
	if _, err := db.Exec(`UPDATE bench_idempotency SET expires_at = now() - interval '1 hour' WHERE key='bench-test-1'`); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := idempotencyPostgresReplay(db, "bench-test-1"); ok {
		t.Fatal("expired key should be a miss")
	}
}
