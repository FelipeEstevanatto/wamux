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
