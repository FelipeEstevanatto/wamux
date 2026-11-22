// Package message_cleanup prunes persisted messages past the retention window.
//
// The messages table has no natural bound: with DATABASE_SAVE_MESSAGES on, every
// message is stored forever, which grows the disk without limit and makes the
// dashboard's whole-table aggregates progressively slower. This runs a periodic
// batched delete instead.
package message_cleanup

import (
	"context"
	"fmt"
	"time"

	applog "github.com/felipeestevanatto/wamux/pkg/applog"
	"gorm.io/gorm"
)

const (
	// defaultRunInterval is how often the sweep repeats.
	defaultRunInterval = 6 * time.Hour

	// orphanReceiptAge is how long a receipt waits for its message to arrive.
	orphanReceiptAge = 24 * time.Hour

	// defaultStartDelay lets the process finish starting (database, instances)
	// before the first sweep touches the messages table.
	defaultStartDelay = time.Minute

	// timeLayout matches how timestamps are stored ("YYYY-MM-DD HH:MM:SS"), so
	// the cutoff can be compared lexicographically in SQL.
	timeLayout = "2006-01-02 15:04:05"
)

// MessageDeleter is the slice of the message repository the cleaner needs.
// Declared here, at the consumer, so the cleaner is not tied to the whole
// repository.
type MessageDeleter interface {
	DeleteMessagesOlderThan(cutoff string) (int64, error)
	DeleteOrphanReceipts(cutoff string) (int64, error)
}

// Cleaner deletes messages older than the configured retention.
type Cleaner interface {
	// Start begins the periodic sweep and returns immediately. It stops when ctx
	// is cancelled. A non-positive retention keeps messages, but receipts whose
	// message never arrived are still removed.
	Start(ctx context.Context)
}

// MediaPruner deletes stored media files older than cutoff. localmedia.RemoveOlderThan
// is the real one.
type MediaPruner func(cutoff time.Time) (int, error)

// TableRetention prunes one auxiliary table. Table and column are compile-time
// constants from the caller, never user input; the deleter still guards them.
type TableRetention struct {
	Table  string
	Column string
	Days   int
	// TextColumn means the column is a "YYYY-MM-DD HH:MM:SS" text column (for
	// example chat_reads.read_at), not a timestamp type.
	TextColumn bool
}

// TableDeleter deletes rows older than cutoff. Implemented by GormTableDeleter;
// an interface so the sweep is testable without a database.
type TableDeleter interface {
	DeleteOlderThan(table, column string, cutoff time.Time, textColumn bool) (int64, error)
}

// MetricsSink receives cleanup counters. Optional; nil disables the metric.
type MetricsSink interface {
	AddCleanupDeleted(table string, n int64)
}

// GormTableDeleter is the production TableDeleter.
type GormTableDeleter struct{ DB *gorm.DB }

func (g GormTableDeleter) DeleteOlderThan(table, column string, cutoff time.Time, textColumn bool) (int64, error) {
	if g.DB == nil {
		return 0, nil
	}
	if !safeIdentifier(table) || !safeIdentifier(column) {
		return 0, fmt.Errorf("cleanup: unsafe table/column %q.%q", table, column)
	}
	value := any(cutoff)
	if textColumn {
		value = cutoff.Format(timeLayout)
	}
	res := g.DB.Exec("DELETE FROM "+table+" WHERE "+column+" < ?", value)
	return res.RowsAffected, res.Error
}

func safeIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r >= 'a' && r <= 'z' || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

// Option configures a cleaner.
type Option func(*cleaner)

// WithTableRetention adds one auxiliary-table retention rule.
func WithTableRetention(deleter TableDeleter, rule TableRetention) Option {
	return func(c *cleaner) {
		if deleter != nil && rule.Days > 0 && safeIdentifier(rule.Table) && safeIdentifier(rule.Column) {
			c.tableRetentions = append(c.tableRetentions, tableRetention{TableRetention: rule, deleter: deleter})
		}
	}
}

// WithCleanupMetrics reports deleted rows to a metrics sink.
func WithCleanupMetrics(sink MetricsSink) Option {
	return func(c *cleaner) { c.metrics = sink }
}

// WithMediaRetention also deletes stored media older than days. Message rows and
// files are pruned independently: a row can outlive its file, which just reads as
// media no longer available.
func WithMediaRetention(days int, prune MediaPruner) Option {
	return func(c *cleaner) {
		c.mediaRetentionDays = days
		c.pruneMedia = prune
	}
}

type cleaner struct {
	repo          MessageDeleter
	retentionDays int

	mediaRetentionDays int
	pruneMedia         MediaPruner
	tableRetentions    []tableRetention
	metrics            MetricsSink

	// Overridable in tests so they do not have to wait hours.
	runInterval time.Duration
	startDelay  time.Duration

	// now is time.Now, overridable in tests to pin the cutoff.
	now func() time.Time
}

// NewCleaner builds a cleaner that keeps messages for retentionDays days. Pass 0
// to keep them forever (Start then only logs that it is disabled).
func NewCleaner(repo MessageDeleter, retentionDays int, opts ...Option) Cleaner {
	c := &cleaner{
		repo:          repo,
		retentionDays: retentionDays,
		runInterval:   defaultRunInterval,
		startDelay:    defaultStartDelay,
		now:           time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// tableRetention is one explicitly configured rule and its deleter.
type tableRetention struct {
	TableRetention
	deleter TableDeleter
}

// prunesMedia reports whether stored media has a retention to enforce.
func (c *cleaner) prunesMedia() bool {
	return c.pruneMedia != nil && c.mediaRetentionDays > 0
}

func (c *cleaner) prunesTables() bool {
	return len(c.tableRetentions) > 0
}

func (c *cleaner) Start(ctx context.Context) {
	if c.retentionDays <= 0 && !c.prunesMedia() && !c.prunesTables() {
		applog.Logger.LogInfo("[cleanup] retention disabled; data is kept forever")
	} else {
		applog.Logger.LogInfo("[cleanup] keeping messages for %d day(s); first sweep in %s, then every %s",
			c.retentionDays, c.startDelay, c.runInterval)
	}
	go c.loop(ctx)
}

func (c *cleaner) loop(ctx context.Context) {
	timer := time.NewTimer(c.startDelay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			c.sweep()
			timer.Reset(c.runInterval)
		}
	}
}

// sweep deletes one batch-run and logs the outcome.
func (c *cleaner) sweep() {
	c.sweepOrphanReceipts()
	c.sweepTables()
	if c.prunesMedia() {
		c.sweepMedia()
	}
	if c.retentionDays > 0 {
		c.sweepMessages()
	}
}

func (c *cleaner) sweepTables() {
	for _, rule := range c.tableRetentions {
		cutoff := c.now().AddDate(0, 0, -rule.Days)
		deleted, err := rule.deleter.DeleteOlderThan(rule.Table, rule.Column, cutoff, rule.TextColumn)
		if err != nil {
			applog.Logger.LogError("[cleanup] failed to prune %s.%s: %v", rule.Table, rule.Column, err)
			continue
		}
		if c.metrics != nil && deleted > 0 {
			c.metrics.AddCleanupDeleted(rule.Table, deleted)
		}
		if deleted > 0 {
			applog.Logger.LogInfo("[cleanup] deleted %d row(s) from %s older than %d day(s)", deleted, rule.Table, rule.Days)
		}
	}
}

// sweepOrphanReceipts runs whatever the retention: these rows hold nothing to keep.
func (c *cleaner) sweepOrphanReceipts() {
	deleted, err := c.repo.DeleteOrphanReceipts(c.now().Add(-orphanReceiptAge).Format(timeLayout))
	if err != nil {
		applog.Logger.LogError("[cleanup] failed to delete orphan receipts: %v", err)
		return
	}
	if deleted > 0 {
		applog.Logger.LogInfo("[cleanup] deleted %d receipt(s) whose message never arrived", deleted)
	}
}

func (c *cleaner) sweepMedia() {
	removed, err := c.pruneMedia(c.now().AddDate(0, 0, -c.mediaRetentionDays))
	if err != nil {
		applog.Logger.LogError("[cleanup] failed to prune stored media: %v", err)
	}
	if removed > 0 {
		applog.Logger.LogInfo("[cleanup] deleted %d stored media file(s) older than %d day(s)", removed, c.mediaRetentionDays)
	}
}

func (c *cleaner) sweepMessages() {
	cutoff := c.now().AddDate(0, 0, -c.retentionDays).Format(timeLayout)

	start := time.Now()
	deleted, err := c.repo.DeleteMessagesOlderThan(cutoff)
	if err != nil {
		applog.Logger.LogError("[cleanup] failed to delete messages older than %s: %v", cutoff, err)
		return
	}

	if deleted > 0 {
		applog.Logger.LogInfo("[cleanup] deleted %d message(s) older than %s in %s",
			deleted, cutoff, time.Since(start).Round(time.Millisecond))
		return
	}
	applog.Logger.LogDebug("[cleanup] no messages older than %s", cutoff)
}
