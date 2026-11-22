// Package audit records administrative actions locally.
//
// It is deliberately small: a row in the users database (or a structured log
// line when no database is wired, e.g. in tests) with actor, action, target and
// a JSON detail object. Secrets are never accepted in detail; callers pass only
// non-sensitive fields such as a name, a bucket or a boolean.
package audit

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

// Logger writes audit entries. The zero value logs only (no database).
type Logger struct {
	db *gorm.DB
}

var defaultLogger = &Logger{}

// Configure installs the process-wide logger. A nil db keeps the structured-log
// fallback, which is what tests and a database-less boot use.
func Configure(db *gorm.DB) {
	defaultLogger = &Logger{db: db}
}

// Record writes one entry. It never returns an error to the caller: an audit
// write must not fail the operation it describes. Detail values must not contain
// secrets.
func Record(actor, action, target string, detail map[string]any) {
	defaultLogger.record(actor, action, target, detail)
}

// List returns the newest entries, up to limit (default 100, max 1000).
func List(limit int) ([]Entry, error) {
	return defaultLogger.list(limit)
}

func (l *Logger) record(actor, action, target string, detail map[string]any) {
	if l == nil || strings.TrimSpace(action) == "" {
		return
	}
	if actor == "" {
		actor = "unknown"
	}
	detailJSON := ""
	if len(detail) > 0 {
		if b, err := json.Marshal(detail); err == nil {
			detailJSON = string(b)
		}
	}
	if l.db == nil {
		applog.Logger.LogInfo("[AUDIT] actor=%q action=%q target=%q detail=%s", actor, action, target, detailJSON)
		return
	}
	err := l.db.Exec(
		`INSERT INTO audit_logs (id, actor, action, target, detail, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), actor, action, target, detailJSON, time.Now().UTC(),
	).Error
	if err != nil {
		applog.Logger.LogWarn("[AUDIT] could not persist %s on %s: %v", action, target, err)
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
