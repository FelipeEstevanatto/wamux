// Package message_cleanup prunes persisted messages past the retention window.
//
// The messages table has no natural bound: with DATABASE_SAVE_MESSAGES on, every
// message is stored forever, which grows the disk without limit and makes the
// dashboard's whole-table aggregates progressively slower. This runs a periodic
// batched delete instead.
package message_cleanup

import (
	"context"
	"time"

	applog "github.com/felipeestevanatto/wamux/pkg/applog"
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

// Option configures a cleaner.
type Option func(*cleaner)

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

// prunesMedia reports whether stored media has a retention to enforce.
func (c *cleaner) prunesMedia() bool {
	return c.pruneMedia != nil && c.mediaRetentionDays > 0
}

func (c *cleaner) Start(ctx context.Context) {
	if c.retentionDays <= 0 && !c.prunesMedia() {
		applog.Logger.LogInfo("[cleanup] message retention disabled (MESSAGE_RETENTION_DAYS=0); messages are kept forever")
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
	if c.prunesMedia() {
		c.sweepMedia()
	}
	if c.retentionDays > 0 {
		c.sweepMessages()
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
