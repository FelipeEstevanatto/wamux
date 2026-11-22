// Package audit records administrative actions locally.
//
// It is deliberately small: a row in the users database (or a structured log
// line when no database is wired, e.g. in tests) with actor, action, target and
// a JSON detail object. Secrets are never accepted in detail; callers pass only
// non-sensitive fields such as a name, a bucket or a boolean.
//
// Record is asynchronous: an audit write must never add latency to the admin
// request that triggered it. The queue is bounded; a flood drops entries with a
// warning rather than blocking. Token reads are deduplicated per actor/target
// for one minute so a manager polling /instance/all does not create a row on
// every poll.
package audit

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	applog "github.com/felipeestevanatto/wamux/pkg/applog"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Entry is one audit row.
type Entry struct {
	ID        string    `json:"id"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// readDedupeTTL collapses repeated token reads from the same actor/target.
const readDedupeTTL = time.Minute

const queueSize = 1024

// Logger writes audit entries. The zero value logs only (no database).
type Logger struct {
	db *gorm.DB
	// queue and recent are only used by the process-wide logger configured with
	// Configure. A Logger built directly in a test writes synchronously.
	queue  chan Entry
	mu     sync.Mutex
	recent map[string]time.Time
	now    func() time.Time
}

var defaultLogger = &Logger{}

// Configure installs the process-wide logger. A nil db keeps the structured-log
// fallback, which is what tests and a database-less boot use.
func Configure(db *gorm.DB) {
	l := &Logger{db: db, now: time.Now}
	if db != nil {
		l.queue = make(chan Entry, queueSize)
		l.recent = make(map[string]time.Time)
		go l.worker()
	}
	defaultLogger = l
}

// Record writes one entry. It never returns an error to the caller: an audit
// write must not fail the operation it describes. Detail values must not contain
// secrets.
func Record(actor, action, target string, detail map[string]any) {
	defaultLogger.enqueue(actor, action, target, detail)
}

// List returns the newest entries, up to limit (default 100, max 1000).
func List(limit int) ([]Entry, error) {
	return defaultLogger.list(limit)
}

func (l *Logger) enqueue(actor, action, target string, detail map[string]any) {
	if l == nil || strings.TrimSpace(action) == "" {
		return
	}
	if l.db == nil || l.queue == nil {
		l.record(actor, action, target, detail)
		return
	}
	if action == "instance.token.read" && l.duplicateRead(actor, target) {
		return
	}
	entry := l.entry(actor, action, target, detail)
	select {
	case l.queue <- entry:
	default:
		applog.Logger.LogWarn("[AUDIT] queue full; dropping %s on %s", action, target)
	}
}

func (l *Logger) duplicateRead(actor, target string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	key := actor + "|" + target
	if at, found := l.recent[key]; found && now.Sub(at) < readDedupeTTL {
		return true
	}
	l.recent[key] = now
	// Bound the map on a long-running process. Expired keys are cheap to drop.
	if len(l.recent) > 4096 {
		for k, at := range l.recent {
			if now.Sub(at) >= readDedupeTTL {
				delete(l.recent, k)
			}
		}
	}
	return false
}

func (l *Logger) worker() {
	for entry := range l.queue {
		l.insert(entry)
	}
}

func (l *Logger) entry(actor, action, target string, detail map[string]any) Entry {
	if actor == "" {
		actor = "unknown"
	}
	detailJSON := ""
	if len(detail) > 0 {
		if b, err := json.Marshal(detail); err == nil {
			detailJSON = string(b)
		}
	}
	return Entry{
		ID:        uuid.New().String(),
		Actor:     actor,
		Action:    action,
		Target:    target,
		Detail:    detailJSON,
		CreatedAt: time.Now().UTC(),
	}
}

// record writes synchronously. It is used by the no-database fallback and by
// tests that build a Logger directly.
func (l *Logger) record(actor, action, target string, detail map[string]any) {
	if l == nil || strings.TrimSpace(action) == "" {
		return
	}
	entry := l.entry(actor, action, target, detail)
	if l.db == nil {
		applog.Logger.LogInfo("[AUDIT] actor=%q action=%q target=%q detail=%s", entry.Actor, entry.Action, entry.Target, entry.Detail)
		return
	}
	l.insert(entry)
}

func (l *Logger) insert(entry Entry) {
	if l.db == nil {
		return
	}
	err := l.db.Exec(
		`INSERT INTO audit_logs (id, actor, action, target, detail, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.Actor, entry.Action, entry.Target, entry.Detail, entry.CreatedAt,
	).Error
	if err != nil {
		applog.Logger.LogWarn("[AUDIT] could not persist %s on %s: %v", entry.Action, entry.Target, err)
	}
}

func (l *Logger) list(limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	if l == nil || l.db == nil {
		return nil, errors.New("audit log is not configured")
	}
	var out []Entry
	err := l.db.Raw(
		`SELECT id, actor, action, target, COALESCE(detail, '') AS detail, created_at
		 FROM audit_logs ORDER BY created_at DESC LIMIT ?`, limit,
	).Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("read audit log: %w", err)
	}
	return out, nil
}
