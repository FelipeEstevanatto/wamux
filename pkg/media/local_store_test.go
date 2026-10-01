package media

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStoreSaveOpenRemove(t *testing.T) {
	root := t.TempDir()
	if err := Configure(filepath.Join(root, "media")); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	want := []byte("hello attachment")
	if err := Save("instance-1", "msg-1", want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !Exists("instance-1", "msg-1") {
		t.Fatalf("expected stored media to exist")
	}

	f, info, err := Open("instance-1", "msg-1")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	if info.Size() != int64(len(want)) {
		t.Fatalf("size = %d, want %d", info.Size(), len(want))
	}
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("content = %q, want %q", got, want)
	}

	if err := Remove("instance-1", "msg-1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if Exists("instance-1", "msg-1") {
		t.Fatalf("expected media to be removed")
	}
}

func TestLocalStoreRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	if err := Configure(filepath.Join(root, "media")); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if err := Save("../escape", "msg", []byte("x")); err == nil {
		t.Fatalf("expected traversal in instance id to be rejected")
	}
	if err := Save("instance", "../escape", []byte("x")); err == nil {
		t.Fatalf("expected traversal in message id to be rejected")
	}

	// The escape path must not have been created.
	if _, err := os.Stat(filepath.Join(root, "escape")); err == nil {
		t.Fatalf("traversal wrote outside the store root")
	}
}

func TestOpenMissingFile(t *testing.T) {
	root := t.TempDir()
	if err := Configure(filepath.Join(root, "media")); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if _, _, err := Open("instance", "missing"); err == nil {
		t.Fatalf("expected an error for a missing file")
	}
}

func TestURLPath(t *testing.T) {
	if got := URLPath("abc123"); got != "/chat/media/abc123" {
		t.Fatalf("URLPath = %q", got)
	}
}
