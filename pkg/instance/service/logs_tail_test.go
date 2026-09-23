package instance_service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
)

func writeLogFile(t *testing.T, dir, id string, n int, base time.Time) {
	t.Helper()
	logDir := filepath.Join(dir, id)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		entry := map[string]any{
			"timestamp":   base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339Nano),
			"level":       "INFO",
			"instance_id": id,
			"message":     fmt.Sprintf("line %d", i),
		}
		raw, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(logDir, "instance.log"), []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
}

func newLogService(t *testing.T, dir string) instances {
	t.Helper()
	cfg := &config.Config{LogDirectory: dir}
	return instances{
		config:        cfg,
		loggerWrapper: logger_wrapper.NewLoggerManager(cfg),
	}
}

// The viewer must return the most recent entries, newest first, not the oldest
// ones at the start of the window.
func TestGetLogsReturnsNewestFirst(t *testing.T) {
	dir := t.TempDir()
	id := "11111111-1111-1111-1111-111111111111"
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	writeLogFile(t, dir, id, 500, base)

	svc := newLogService(t, dir)
	logs, err := svc.GetLogs(id, base.Add(-24*time.Hour), base.Add(24*time.Hour), "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 10 {
		t.Fatalf("expected 10 entries, got %d", len(logs))
	}
	if logs[0].Message != "line 499" {
		t.Fatalf("expected the newest line first, got %q", logs[0].Message)
	}
	if logs[9].Message != "line 490" {
		t.Fatalf("expected line 490 last, got %q", logs[9].Message)
	}
	for i := 1; i < len(logs); i++ {
		if logs[i].Timestamp.After(logs[i-1].Timestamp) {
			t.Fatalf("entries not in descending order at %d", i)
		}
	}
}

// A level filter must still return the newest matching entries.
func TestGetLogsLevelFilterNewestFirst(t *testing.T) {
	dir := t.TempDir()
	id := "22222222-2222-2222-2222-222222222222"
	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	logDir := filepath.Join(dir, id)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for i := 0; i < 100; i++ {
		level := "INFO"
		if i%2 == 0 {
			level = "ERROR"
		}
		entry := map[string]any{
			"timestamp":   base.Add(time.Duration(i) * time.Minute).Format(time.RFC3339Nano),
			"level":       level,
			"instance_id": id,
			"message":     fmt.Sprintf("line %d", i),
		}
		raw, _ := json.Marshal(entry)
		b.Write(raw)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(logDir, "instance.log"), []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}

	svc := newLogService(t, dir)
	logs, err := svc.GetLogs(id, base.Add(-24*time.Hour), base.Add(24*time.Hour), "ERROR", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(logs))
	}
	// Even indices are ERROR; the newest are 98, 96, 94, 92, 90.
	want := []string{"line 98", "line 96", "line 94", "line 92", "line 90"}
	for i, w := range want {
		if logs[i].Message != w {
			t.Fatalf("entry %d = %q, want %q", i, logs[i].Message, w)
		}
	}
}

// A missing log file must still return an empty slice, not an error.
func TestGetLogsMissingFile(t *testing.T) {
	dir := t.TempDir()
	id := "33333333-3333-3333-3333-333333333333"
	svc := newLogService(t, dir)
	logs, err := svc.GetLogs(id, time.Now().Add(-24*time.Hour), time.Now(), "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected no logs, got %d", len(logs))
	}
}
