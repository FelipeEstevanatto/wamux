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
	"io"
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

// messagePath builds the one path every read/write of a message's media goes
// through. Ids must be single path components; the resolved path is then
// checked to be inside the store root. Both checks are deliberately explicit
// so static analysis can follow them.
func messagePath(s *LocalStore, instanceID, messageID string) (string, error) {
	for _, part := range []string{instanceID, messageID} {
		if part == "" || part == "." || part == ".." ||
			strings.Contains(part, "/") || strings.Contains(part, "\\") || strings.Contains(part, "..") {
			return "", errors.New("invalid media id")
		}
	}
	root, err := filepath.Abs(s.root)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(root, instanceID, messageID))
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(path, root+string(os.PathSeparator)) {
		return "", errors.New("invalid media id")
	}
	return path, nil
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
	path, err := messagePath(s, instanceID, messageID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o751); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o640)
}

// Open returns the stored file for one message plus its stat info.
func Open(instanceID, messageID string) (*os.File, fs.FileInfo, error) {
	s := current()
	if s == nil {
		return nil, nil, ErrNotConfigured
	}
	path, err := messagePath(s, instanceID, messageID)
	if err != nil {
		return nil, nil, err
	}
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
	if s == nil {
		return false
	}
	path, err := messagePath(s, instanceID, messageID)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Remove deletes a message's stored media, if present.
func Remove(instanceID, messageID string) error {
	s := current()
	if s == nil {
		return nil
	}
	path, err := messagePath(s, instanceID, messageID)
	if err != nil {
		return nil
	}
	_ = os.Remove(path + previewSuffix) // Most files never had one.
	err = os.Remove(path)
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

// SaveFrom stores a file too large to hold in memory.
func SaveFrom(instanceID, messageID string, r io.Reader) error {
	if current() == nil {
		return ErrNotConfigured
	}
	temp, err := CreateTemp()
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name()) // A no-op once Adopt has moved it.
	defer temp.Close()
	if _, err := io.Copy(temp, r); err != nil {
		return err
	}
	return Adopt(instanceID, messageID, temp.Name())
}

// Adopt moves a file made by CreateTemp into place as a message's media. The bytes
// never pass through memory, which is the point: Save needs them all at once.
func Adopt(instanceID, messageID, tempPath string) error {
	s := current()
	if s == nil {
		return ErrNotConfigured
	}
	path, err := messagePath(s, instanceID, messageID)
	if err != nil {
		return err
	}
	// tempPath is supplied by SaveFrom, but Adopt is exported: never let a
	// caller rename an arbitrary file into the store as a message.
	absRoot, err := filepath.Abs(s.root)
	if err != nil {
		return err
	}
	absTemp, err := filepath.Abs(tempPath)
	if err != nil {
		return err
	}
	if filepath.Dir(absTemp) != absRoot || !strings.HasPrefix(absTemp, absRoot+string(os.PathSeparator)) {
		return errors.New("invalid temp path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o751); err != nil {
		return err
	}
	if err := os.Chmod(absTemp, 0o640); err != nil {
		return err
	}
	return os.Rename(absTemp, path)
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
