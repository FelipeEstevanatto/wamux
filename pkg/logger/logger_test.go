package logger

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
)

func testLogger(t *testing.T, instanceId string) (*Logger, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{LogDirectory: dir, LogMaxSize: 100, LogMaxBackups: 1, LogMaxAge: 1}
	l := newLogger(instanceId, cfg)
	t.Cleanup(func() { _ = l.Close() })
	return l, filepath.Join(dir, instanceId, "instance.log")
}

func readLines(t *testing.T, path string) []LogEntry {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	var entries []LogEntry
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e LogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("invalid JSON log line %q: %v", line, err)
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan log: %v", err)
	}
	return entries
}

// Flush must make every line written before it visible in the file.
func TestLoggerFlushPersistsAllLines(t *testing.T) {
	l, path := testLogger(t, "11111111-1111-1111-1111-111111111111")

	const n = 500
	for i := 0; i < n; i++ {
		l.LogInfo("line %d", i)
	}
	l.Flush()

	entries := readLines(t, path)
	if len(entries) != n {
		t.Fatalf("expected %d lines after flush, got %d", n, len(entries))
	}
	if entries[0].Level != "INFO" || entries[0].InstanceId == "" {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
}

// Concurrent logging must not lose or corrupt lines.
func TestLoggerConcurrentNoLoss(t *testing.T) {
	l, path := testLogger(t, "22222222-2222-2222-2222-222222222222")

	const goroutines = 16
	const perGoroutine = 200

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				l.LogInfo("g%d line %d", g, i)
			}
		}(g)
	}
	wg.Wait()
	l.Flush()

	entries := readLines(t, path)
	if len(entries) != goroutines*perGoroutine {
		t.Fatalf("expected %d lines, got %d", goroutines*perGoroutine, len(entries))
	}
}

// Close must flush whatever is still queued.
func TestLoggerCloseFlushes(t *testing.T) {
	l, path := testLogger(t, "33333333-3333-3333-3333-333333333333")

	for i := 0; i < 100; i++ {
		l.LogInfo("line %d", i)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	entries := readLines(t, path)
	if len(entries) != 100 {
		t.Fatalf("expected 100 lines after close, got %d", len(entries))
	}
}

// Manager.Flush for an instance that never logged must not create a logger or
// panic.
func TestLoggerManagerFlushUnknownInstance(t *testing.T) {
	lm := NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	lm.Flush("does-not-exist")
}
