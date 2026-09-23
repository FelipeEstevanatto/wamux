package server_handler

import (
	"io/fs"
	"path/filepath"
	"sync"
	"time"
)

// dirUsage caches a directory walk. /server/stats is polled every ~15s by the
// dashboard and the data directory can hold a lot of log files, so re-walking it
// on every request would be wasteful; a one-minute TTL keeps the number fresh
// enough while staying cheap.
type dirUsage struct {
	mu       sync.Mutex
	at       time.Time
	path     string
	bytes    int64
	files    int64
	interval time.Duration
}

// get returns the cached size (bytes) and file count for path, recomputing it
// when the cache is stale.
func (d *dirUsage) get(path string) (int64, int64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	interval := d.interval
	if interval <= 0 {
		interval = time.Minute
	}
	if d.path == path && !d.at.IsZero() && time.Since(d.at) < interval {
		return d.bytes, d.files
	}

	bytes, files := walkDir(path)
	d.path, d.at, d.bytes, d.files = path, time.Now(), bytes, files
	return bytes, files
}

// walkDir sums the size and count of the regular files under root. It is
// best-effort: unreadable entries are skipped rather than failing the whole walk.
func walkDir(root string) (int64, int64) {
	var bytes, files int64
	_ = filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if info, ierr := entry.Info(); ierr == nil && info.Mode().IsRegular() {
			bytes += info.Size()
			files++
		}
		return nil
	})
	return bytes, files
}
