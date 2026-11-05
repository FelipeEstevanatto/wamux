// Package media stores message attachments on local disk so the manager can
// preview them without an object store (MinIO/S3).
//
// Files live under <dataDir>/media/<instanceId>/<messageId>. The store is
// process-wide and configured once at startup; when it is not configured (or a
// save fails) callers simply fall back to the previous behaviour (base64 or a
// type placeholder in the UI).
package media

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	mu    sync.RWMutex
	store *LocalStore
)

// ErrNotConfigured is returned when Configure was never called.
var ErrNotConfigured = errors.New("media store not configured")

// LocalStore writes attachment bytes under a root directory.
type LocalStore struct {
	root string
}

// Configure initialises the process-wide store rooted at root. It is safe to
// call once at startup.
func Configure(root string) error {
	if err := os.MkdirAll(root, 0o751); err != nil {
		return err
	}
	mu.Lock()
	store = &LocalStore{root: root}
	mu.Unlock()
	return nil
}

func current() *LocalStore {
	mu.RLock()
	defer mu.RUnlock()
	return store
}

// safeName rejects path separators and traversal, so a crafted id cannot escape
// the store root.
func safeName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

// URLPath is the authenticated API path that serves a stored message's media.
func URLPath(messageID string) string {
	return "/chat/media/" + messageID
}

// Save writes data for one message. Overwrites any existing file.
func Save(instanceID, messageID string, data []byte) error {
	s := current()
	if s == nil {
		return ErrNotConfigured
	}
	if !safeName(instanceID) || !safeName(messageID) {
		return errors.New("invalid media id")
	}
	dir := filepath.Join(s.root, instanceID)
	if err := os.MkdirAll(dir, 0o751); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, messageID), data, 0o640)
}

// Open returns the stored file for one message plus its stat info.
func Open(instanceID, messageID string) (*os.File, fs.FileInfo, error) {
	s := current()
	if s == nil {
		return nil, nil, ErrNotConfigured
	}
	if !safeName(instanceID) || !safeName(messageID) {
		return nil, nil, errors.New("invalid media id")
	}
	path := filepath.Join(s.root, instanceID, messageID)
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	return f, info, nil
}

// Exists reports whether a message's media is stored.
func Exists(instanceID, messageID string) bool {
	s := current()
	if s == nil || !safeName(instanceID) || !safeName(messageID) {
		return false
	}
	_, err := os.Stat(filepath.Join(s.root, instanceID, messageID))
	return err == nil
}

// Remove deletes a message's stored media, if present.
func Remove(instanceID, messageID string) error {
	s := current()
	if s == nil {
		return nil
	}
	if !safeName(instanceID) || !safeName(messageID) {
		return nil
	}
	err := os.Remove(filepath.Join(s.root, instanceID, messageID))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Configured reports whether the store is active.
func Configured() bool {
	return current() != nil
}

// tempPattern names in-progress downloads, directly under the store root so that
// Adopt is a rename on the same filesystem.
const tempPattern = ".incoming-*"

// CreateTemp opens a file to download into. Without a store it uses the system
// temp directory, for media that is only passing through to an object store.
func CreateTemp() (*os.File, error) {
	dir := ""
	if s := current(); s != nil {
		dir = s.root
	}
	return os.CreateTemp(dir, tempPattern)
}

// Adopt moves a file made by CreateTemp into place as a message's media. The bytes
// never pass through memory, which is the point: Save needs them all at once.
func Adopt(instanceID, messageID, tempPath string) error {
	s := current()
	if s == nil {
		return ErrNotConfigured
	}
	if !safeName(instanceID) || !safeName(messageID) {
		return errors.New("invalid media id")
	}
	dir := filepath.Join(s.root, instanceID)
	if err := os.MkdirAll(dir, 0o751); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, 0o640); err != nil {
		return err
	}
	return os.Rename(tempPath, filepath.Join(dir, messageID))
}

// RemoveInstance deletes everything stored for one instance.
func RemoveInstance(instanceID string) error {
	s := current()
	if s == nil || !safeName(instanceID) {
		return nil
	}
	return os.RemoveAll(filepath.Join(s.root, instanceID))
}

// RemoveOlderThan deletes stored files last modified before cutoff and returns how
// many went. That covers temp files a crash left behind. It keeps going past a
// file it cannot remove and reports the first error.
func RemoveOlderThan(cutoff time.Time) (int, error) {
	s := current()
	if s == nil {
		return 0, nil
	}

	removed := 0
	var firstErr error
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if firstErr == nil {
				firstErr = walkErr
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			return nil
		}
		if err := os.Remove(path); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return nil
		}
		removed++
		return nil
	})
	if firstErr == nil {
		firstErr = err
	}
	return removed, firstErr
}
