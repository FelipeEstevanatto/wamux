package media

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// Adopt is what keeps a large file out of memory, so it must land exactly where
// Open looks.
func TestAdoptMovesTempFileIntoPlace(t *testing.T) {
	if err := Configure(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetStore)

	file, err := CreateTemp()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("video bytes"); err != nil {
		t.Fatal(err)
	}
	file.Close()

	if err := Adopt("inst", "msg", file.Name()); err != nil {
		t.Fatalf("Adopt: %v", err)
	}
	opened, info, err := Open("inst", "msg")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer opened.Close()
	if info.Size() != int64(len("video bytes")) {
		t.Fatalf("size = %d", info.Size())
	}
	if _, err := os.Stat(file.Name()); !os.IsNotExist(err) {
		t.Fatal("temp file still exists after Adopt")
	}
}

func TestAdoptRejectsTraversal(t *testing.T) {
	if err := Configure(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetStore)

	file, err := CreateTemp()
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if err := Adopt("inst", "../escape", file.Name()); err == nil {
		t.Fatal("expected a traversal id to be refused")
	}
}

func TestRemoveOlderThanKeepsRecentFiles(t *testing.T) {
	if err := Configure(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetStore)

	if err := Save("inst", "old", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := Save("inst", "new", []byte("x")); err != nil {
		t.Fatal(err)
	}
	stale, err := CreateTemp() // a crash left this behind
	if err != nil {
		t.Fatal(err)
	}
	stale.Close()

	past := time.Now().Add(-48 * time.Hour)
	old, _, err := Open("inst", "old")
	if err != nil {
		t.Fatal(err)
	}
	old.Close()
	if err := os.Chtimes(old.Name(), past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(stale.Name(), past, past); err != nil {
		t.Fatal(err)
	}

	removed, err := RemoveOlderThan(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("removed %d, want 2", removed)
	}
	if Exists("inst", "old") || !Exists("inst", "new") {
		t.Fatalf("old=%v new=%v", Exists("inst", "old"), Exists("inst", "new"))
	}
}

func TestRemoveInstanceDeletesOnlyThatInstance(t *testing.T) {
	if err := Configure(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resetStore)

	if err := Save("gone", "m", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := Save("kept", "m", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveInstance("gone"); err != nil {
		t.Fatal(err)
	}
	if err := RemoveInstance(".."); err != nil {
		t.Fatal(err)
	}
	if Exists("gone", "m") || !Exists("kept", "m") {
		t.Fatalf("gone=%v kept=%v", Exists("gone", "m"), Exists("kept", "m"))
	}
}

func resetStore() {
	mu.Lock()
	store = nil
	mu.Unlock()
}

func TestSaveFromStoresAndLeavesNoTempFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")
	if err := Configure(root); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if err := SaveFrom("instance-1", "big", strings.NewReader("large file")); err != nil {
		t.Fatalf("SaveFrom: %v", err)
	}

	f, info, err := Open("instance-1", "big")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()
	if info.Size() != int64(len("large file")) {
		t.Errorf("size = %d", info.Size())
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("left behind: %v", entries)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("disk error") }

func TestSaveFromCleansUpAfterAFailure(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")
	if err := Configure(root); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	if err := SaveFrom("instance-1", "big", failingReader{}); err == nil {
		t.Fatal("want an error")
	}

	if Exists("instance-1", "big") {
		t.Error("stored a partial file")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
}
