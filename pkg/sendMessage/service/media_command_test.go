package send_service

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func hungTool(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// The child keeps the output pipe open after its parent is killed, which is what
// WaitDelay is there for.
func TestMediaCommandKillsAHungTool(t *testing.T) {
	cmd, cancel := mediaCommand(100*time.Millisecond, hungTool(t))
	defer cancel()
	var out bytes.Buffer
	cmd.Stdout = &out

	start := time.Now()
	if err := cmd.Run(); err == nil {
		t.Fatal("expected an error")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("took %v", took)
	}
}

func TestHungPdfToolDoesNotBlockTheThumbnail(t *testing.T) {
	previousBin, previousTimeout := pdftoppmBin, mediaThumbnailTimeout
	pdftoppmBin, mediaThumbnailTimeout = hungTool(t), 100*time.Millisecond
	defer func() { pdftoppmBin, mediaThumbnailTimeout = previousBin, previousTimeout }()

	start := time.Now()
	if got := makePDFThumbnail([]byte("%PDF"), 100); got != nil {
		t.Fatalf("got %d bytes", len(got))
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("took %v", took)
	}
}
