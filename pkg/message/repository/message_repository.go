package message_repository

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	applog "github.com/felipeestevanatto/wamux/pkg/applog"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	"github.com/patrickmn/go-cache"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MessageRepository interface {
	InsertMessage(message message_model.Message) error
	// InsertMessages writes a batch in as few round trips as possible. Foreign
	// keys are not used, so rows can be grouped and inserted together; a batch
	// with mixed update-column sets is split into homogeneous groups so the
	// upsert semantics of InsertMessage are preserved.
	InsertMessages(messages []message_model.Message) error
	GetMessageByID(messageID string) (*message_model.Message, error)
	// GetMessageByIDForInstance is the tenant-safe lookup: it returns the message
	// only when it belongs to instanceId. Handlers that serve one instance must
	// use this, never GetMessageByID, so a message id from tenant A cannot be
	// read with tenant B's token.
	GetMessageByIDForInstance(instanceId, messageID string) (*message_model.Message, error)
	DeleteAllMessages() (int64, error)
	GetLatestMessageID(source string) (string, string, error)
	GetStats() (*MessageStats, error)
	CountByInstance(instanceId string) (int64, error)
	CountChatsByInstance(instanceId string) (int64, error)
	DatabaseSizeBytes() (totalBytes int64, messagesBytes int64, err error)
	DeleteMessagesOlderThan(cutoff string) (int64, error)

	// History readback (GET /chat/history and GET /chat/chats).
	ListMessages(instanceId, chatJid, before string, limit int) ([]message_model.Message, error)
	ListChats(instanceId string, limit int) ([]ChatSummary, error)

	// DistinctSenders lists the bare `sender_jid` values that authored messages
	// in one conversation, for labelling group members.
	DistinctSenders(instanceId, chatJid string) ([]string, error)
}

// ChatSummary is the per-conversation row returned by ListChats: the newest
// message in the chat plus its total message count.
type ChatSummary struct {
	ChatJid      string `json:"chat_jid" gorm:"column:chat_jid"`
	MessageID    string `json:"last_message_id" gorm:"column:message_id"`
	Timestamp    string `json:"last_timestamp" gorm:"column:timestamp"`
	MessageType  string `json:"last_message_type" gorm:"column:message_type"`
	TextContent  string `json:"last_text_content" gorm:"column:text_content"`
	Status       string `json:"last_status" gorm:"column:status"`
	MediaUrl     string `json:"last_media_url" gorm:"column:media_url"`
	SenderJid    string `json:"last_sender_jid" gorm:"column:sender_jid"`
	IsFromMe     bool   `json:"last_from_me" gorm:"column:is_from_me"`
	MessageCount int64  `json:"message_count" gorm:"column:message_count"`

	// Name is the resolved display name (group subject or contact name). It is
	// not a column: the service fills it after the query, so the repository stays
	// free of WhatsApp lookups.
	Name string `json:"name,omitempty" gorm:"-"`
}

// History pagination bounds. A request without a limit returns the most recent
// page; an oversized limit is clamped so a single call cannot pull an entire
// conversation into memory.
const (
	defaultHistoryLimit = 50
	maxHistoryLimit     = 500
)

func clampHistoryLimit(limit int) int {
	if limit <= 0 {
		return defaultHistoryLimit
	}
	if limit > maxHistoryLimit {
		return maxHistoryLimit
	}
	return limit
}

// StatKV is a label/count pair used by the dashboard aggregations.
type StatKV struct {
	Key   string `json:"key" gorm:"column:label"`
	Count int64  `json:"count" gorm:"column:total"`
}

// MessageStats aggregates the messages table for the dashboard.
type MessageStats struct {
	Total      int64    `json:"total"`
	ByStatus   []StatKV `json:"byStatus"`
	ByDay      []StatKV `json:"byDay"`
	TopSources []StatKV `json:"topSources"`
}

type messageRepository struct {
	db *gorm.DB

	// rollup reports whether the message_counters table is installed, so the
	// dashboard can read the (fresh, small) rollup instead of aggregating
	// `messages`. Cleared if a rollup query fails, which degrades to the live
	// aggregation rather than failing requests.
	rollup atomic.Bool

	// aggCache memoizes the live aggregations. With the rollup installed they are
	// not used at all; without it they are whole-table scans (measured at 2M rows:
	// /server/stats ~0.5s, CountChatsByInstance ~0.9s) re-run on every 15s poll of
	// every open dashboard.
	//
	// nil when caching is disabled (ttl <= 0).
	aggCache *cache.Cache

	// statsMu serialises the cold-cache aggregation. The cache Get/Set is not
	// atomic, so without this a burst of dashboard polls arriving on an expired
	// cache would each run all four whole-table scans at once.
	statsMu sync.Mutex
}

// Option configures the repository at construction time.
type Option func(*messageRepository)

// WithAggregateCacheTTL caches the live dashboard aggregations for ttl. Zero (the
// default) disables the cache, which is what tests want.
func WithAggregateCacheTTL(ttl time.Duration) Option {
	return func(m *messageRepository) {
		if ttl > 0 {
			m.aggCache = cache.New(ttl, 2*ttl)
		}
	}
}

// WithRollup tells the repository that the message_counters rollup is installed,
// so it should read the dashboard aggregates from it instead of scanning
// `messages`. See EnsureMessageCounters.
func WithRollup(enabled bool) Option {
	return func(m *messageRepository) {
		m.rollup.Store(enabled)
	}
}

func messageUpdateColumns(message message_model.Message) []string {
	// A status-only row (a read/delivered receipt) must move only the delivery
	// state. Writing timestamp/source/content for it would reorder the message
	// to the receipt time and erase what it said. Content rows carry the
	// message and overwrite everything.
	if message.MessageType == "" && message.ChatJid == "" && message.TextContent == "" {
		return []string{"status"}
	}

	updates := []string{
		"timestamp", "status", "source",
		"chat_jid", "sender_jid", "message_type", "text_content",
		"media_url", "media_mimetype", "quoted_message_id", "is_from_me",
	}
	if len(message.Referral) > 0 {
		updates = append(updates, "referral")
	}

	return updates
}

// cacheKeyStatus etc. are the cache keys for the aggregate queries.
const (
	cacheKeyStats  = "stats"
	cacheKeyDBSize = "dbsize"
	cacheKeyChats  = "chats:" // + instanceId
	cacheKeyCount  = "count:" // + instanceId
)

// cached returns the cached value for key, if any.
func (m *messageRepository) cached(key string) (any, bool) {
	if m.aggCache == nil {
		return nil, false
	}
	return m.aggCache.Get(key)
}

// store caches value under key for the configured TTL.
func (m *messageRepository) store(key string, value any) {
	if m.aggCache != nil {
		m.aggCache.Set(key, value, cache.DefaultExpiration)
	}
}

// invalidateAggregates drops every cached aggregation. Called when rows are
// removed, where a stale count would be wrong rather than merely late.
func (m *messageRepository) invalidateAggregates() {
	if m.aggCache != nil {
		m.aggCache.Flush()
	}
}

// cloneStats returns a copy so callers cannot mutate what is shared through the
// cache.
func cloneStats(s *MessageStats) *MessageStats {
	c := *s
	c.ByStatus = append([]StatKV(nil), s.ByStatus...)
	c.ByDay = append([]StatKV(nil), s.ByDay...)
	c.TopSources = append([]StatKV(nil), s.TopSources...)
	return &c
}

func (m *messageRepository) InsertMessage(message message_model.Message) error {
	return m.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "message_id"}},
		DoUpdates: clause.AssignmentColumns(messageUpdateColumns(message)),
	}).Create(&message).Error
}

// batchInsertChunk caps how many rows go in one statement. Very large INSERTs
// hit Postgres's parameter limit (65535); a message has ~11 columns, so 500 rows
// is ~5500 parameters, comfortably inside the limit.
const batchInsertChunk = 500

// InsertMessages upserts a batch with the same conflict semantics as
// InsertMessage, but grouped so each group is a single statement.
//
// Why groups: the ON CONFLICT SET list differs between a content row (updates
// everything) and a status-only receipt row (updates only the status). Rows are
// therefore partitioned by their update-column signature, and each partition is
// inserted in chunks. This keeps the receipt-reordering guarantee while cutting
// N round trips down to (number of groups) statements.
func (m *messageRepository) InsertMessages(messages []message_model.Message) error {
	if len(messages) == 0 {
		return nil
	}

	// Group by update-column signature (there are only two possibilities).
	groups := make(map[string][]message_model.Message, 2)
	var order []string
	for _, msg := range messages {
		cols := messageUpdateColumns(msg)
		key := strings.Join(cols, ",")
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], msg)
	}

	for _, key := range order {
		group := groups[key]
		cols := strings.Split(key, ",")
		for start := 0; start < len(group); start += batchInsertChunk {
			end := start + batchInsertChunk
			if end > len(group) {
				end = len(group)
			}
			chunk := group[start:end]
			if err := m.db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "message_id"}},
				DoUpdates: clause.AssignmentColumns(cols),
			}).Create(&chunk).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *messageRepository) GetMessageByID(messageID string) (*message_model.Message, error) {
	var message message_model.Message
	err := m.db.Where("message_id = ?", messageID).First(&message).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &message, nil
}

// GetMessageByIDForInstance returns the message only when its instance_id
// matches. A blank instanceId is rejected outright — an unscoped lookup is the
// exact mistake this method exists to prevent, so it must never silently fall
// back to "return anything".
func (m *messageRepository) GetMessageByIDForInstance(instanceId, messageID string) (*message_model.Message, error) {
	if instanceId == "" || messageID == "" {
		return nil, nil
	}
	var message message_model.Message
	err := m.db.Where("instance_id = ? AND message_id = ?", instanceId, messageID).First(&message).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &message, nil
}

func (m *messageRepository) DeleteAllMessages() (int64, error) {
	result := m.db.Exec("DELETE FROM messages")
	m.invalidateAggregates()
	return result.RowsAffected, result.Error
}

// ListMessages returns a conversation's stored messages, newest first.
// `before` (a "YYYY-MM-DD HH:MM:SS" timestamp) pages backwards: it returns the
// messages strictly older than the newest one already seen.
func (m *messageRepository) ListMessages(instanceId, chatJid, before string, limit int) ([]message_model.Message, error) {
	messages := make([]message_model.Message, 0)
	if instanceId == "" || chatJid == "" {
		return messages, nil
	}

	query := m.db.Where("instance_id = ? AND chat_jid = ?", instanceId, chatJid)
	if before != "" {
		query = query.Where(`"timestamp" < ?`, before)
	}

	err := query.
		Order(`"timestamp" DESC`).
		Order("id DESC").
		Limit(clampHistoryLimit(limit)).
		Find(&messages).Error
	return messages, err
}

// ListChats returns each conversation's newest message plus its total count,
// ordered by most recent activity. Postgres-only (DISTINCT ON), like the rest
// of the repository's raw SQL.
func (m *messageRepository) ListChats(instanceId string, limit int) ([]ChatSummary, error) {
	summaries := make([]ChatSummary, 0)
	if instanceId == "" {
		return summaries, nil
	}

	// DISTINCT ON keeps the newest row per chat_jid. The window count runs over
	// the whole partition before the distinct is applied, so it is the chat's
	// full message count.
	const query = `
SELECT * FROM (
    SELECT DISTINCT ON (chat_jid)
        chat_jid, message_id, "timestamp", message_type, text_content, status,
        media_url, sender_jid, is_from_me,
        count(*) OVER (PARTITION BY chat_jid) AS message_count
    FROM messages
    WHERE instance_id = ? AND chat_jid <> ''
    ORDER BY chat_jid, "timestamp" DESC
) t
ORDER BY "timestamp" DESC
LIMIT ?`

	err := m.db.Raw(query, instanceId, clampHistoryLimit(limit)).Scan(&summaries).Error
	return summaries, err
}

// distinctValues runs a `SELECT DISTINCT <column>` scoped to one instance and
// returns the non-empty strings. Errors are returned so callers can decide
// whether a failed name-enrichment is worth surfacing (it never is: the raw
// JID is a usable label).
func (m *messageRepository) distinctValues(query string, args ...any) ([]string, error) {
	rows, err := m.db.Raw(query, args...).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0, 64)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		if v != "" {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}

// DistinctSenders lists the distinct authors of messages in one conversation.
// A blank sender_jid (rows written before the column existed, or a receipt) is
// excluded so the caller does not try to resolve "".
func (m *messageRepository) DistinctSenders(instanceId, chatJid string) ([]string, error) {
	if instanceId == "" || chatJid == "" {
		return nil, nil
	}
	return m.distinctValues(
		`SELECT DISTINCT sender_jid FROM messages
		 WHERE instance_id = ? AND chat_jid = ? AND sender_jid <> ''`,
		instanceId, chatJid,
	)
}

func (m *messageRepository) GetLatestMessageID(source string) (string, string, error) {
	var message message_model.Message
	err := m.db.Where("source = ?", source).Order("timestamp DESC").First(&message).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", "", nil
		}
		return "", "", err
	}

	return message.MessageID, message.Timestamp, nil
}

// NewMessageRepository builds the repository. Pass WithRollup to read the
// dashboard aggregates from the message_counters rollup, and/or
// WithAggregateCacheTTL to memoize the live aggregations.
func NewMessageRepository(db *gorm.DB, opts ...Option) MessageRepository {
	m := &messageRepository{db: db}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// rollupFailed disables the rollup for the rest of the process and logs why. The
// live aggregation is correct, just slower, so a broken rollup must not turn
// every dashboard request into an error.
func (m *messageRepository) rollupFailed(op string, err error) {
	applog.Logger.LogError("[stats] %s failed on the message rollup, falling back to the live aggregation: %v", op, err)
	m.rollup.Store(false)
}

// CountByInstance counts persisted messages attributed to one instance. Only
// rows written after the instance_id column was added carry it, so older rows
// are not counted.
func (m *messageRepository) CountByInstance(instanceId string) (int64, error) {
	if m.rollup.Load() {
		total, err := m.countByInstanceFromCounters(instanceId)
		if err == nil {
			return total, nil
		}
		m.rollupFailed("CountByInstance", err)
	}
	return m.countByInstanceLive(instanceId)
}

func (m *messageRepository) countByInstanceLive(instanceId string) (int64, error) {
	key := cacheKeyCount + instanceId
	if v, ok := m.cached(key); ok {
		if total, ok := v.(int64); ok {
			return total, nil
		}
	}

	var total int64
	err := m.db.Model(&message_model.Message{}).
		Where("instance_id = ?", instanceId).
		Count(&total).Error
	if err != nil {
		return 0, err
	}
	m.store(key, total)
	return total, nil
}

// CountChatsByInstance counts distinct conversations for an instance. The Go
// fork does not persist a chat list, so "chats" is the number of distinct
// contacts that have at least one persisted message with this instance.
func (m *messageRepository) CountChatsByInstance(instanceId string) (int64, error) {
	if m.rollup.Load() {
		total, err := m.countChatsByInstanceFromCounters(instanceId)
		if err == nil {
			return total, nil
		}
		m.rollupFailed("CountChatsByInstance", err)
	}
	return m.countChatsByInstanceLive(instanceId)
}

func (m *messageRepository) countChatsByInstanceLive(instanceId string) (int64, error) {
	key := cacheKeyChats + instanceId
	if v, ok := m.cached(key); ok {
		if total, ok := v.(int64); ok {
			return total, nil
		}
	}

	var total int64
	row := m.db.Model(&message_model.Message{}).
		Where("instance_id = ?", instanceId).
		Select("COUNT(DISTINCT source)").
		Row()
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	m.store(key, total)
	return total, nil
}

// GetStats aggregates the messages table: total, breakdown by status, by day
// (last 14 days, newest first) and top contacts by volume. Note: only received
// messages are persisted today (Status="Received", Source=contact number), so
// byStatus is usually dominated by "Received".
//
// TopSources is deliberately over-fetched (25 rows): the HTTP layer merges rows
// that are the same conversation (a contact stored once under a LID and once
// under its phone) and then trims to the display limit, so a generous raw limit
// keeps the merged ranking accurate.
func (m *messageRepository) GetStats() (*MessageStats, error) {
	if m.rollup.Load() {
		stats, err := m.statsFromCounters()
		if err == nil {
			return stats, nil
		}
		m.rollupFailed("GetStats", err)
	}
	return m.statsLive()
}

// statsLive aggregates the messages table directly. It is the fallback when the
// rollup is unavailable, and is memoized by aggCache because it is expensive.
func (m *messageRepository) statsLive() (*MessageStats, error) {
	if v, ok := m.cached(cacheKeyStats); ok {
		if stats, ok := v.(*MessageStats); ok {
			return cloneStats(stats), nil
		}
	}

	// Serialise cold-cache aggregation so concurrent dashboard polls run the
	// scans once, then re-check the cache the waiters now hit.
	m.statsMu.Lock()
	defer m.statsMu.Unlock()
	if v, ok := m.cached(cacheKeyStats); ok {
		if stats, ok := v.(*MessageStats); ok {
			return cloneStats(stats), nil
		}
	}

	stats := &MessageStats{ByStatus: []StatKV{}, ByDay: []StatKV{}, TopSources: []StatKV{}}

	if err := m.db.Model(&message_model.Message{}).Count(&stats.Total).Error; err != nil {
		return nil, err
	}

	m.db.Model(&message_model.Message{}).
		Select("status as label, count(*) as total").
		Group("status").Order("total desc").
		Scan(&stats.ByStatus)

	m.db.Model(&message_model.Message{}).
		Select(`substr("timestamp", 1, 10) as label, count(*) as total`).
		Group(`substr("timestamp", 1, 10)`).Order("label desc").
		Limit(14).
		Scan(&stats.ByDay)

	m.db.Model(&message_model.Message{}).
		Select("source as label, count(*) as total").
		Group("source").Order("total desc").
		Limit(25).
		Scan(&stats.TopSources)

	m.store(cacheKeyStats, stats)
	return stats, nil
}

// DatabaseSizeBytes returns the size of the current database and of the messages
// table, in bytes, for the dashboard's storage panel. The query is
// Postgres-specific; callers treat an error as "size unknown".
func (m *messageRepository) DatabaseSizeBytes() (int64, int64, error) {
	type dbSize struct{ total, table int64 }
	if v, ok := m.cached(cacheKeyDBSize); ok {
		if s, ok := v.(dbSize); ok {
			return s.total, s.table, nil
		}
	}

	var total int64
	if err := m.db.Raw("SELECT pg_database_size(current_database())").Row().Scan(&total); err != nil {
		return 0, 0, err
	}
	// The table size is secondary: if it fails, still report the database total.
	var table int64
	if err := m.db.Raw("SELECT pg_total_relation_size('messages')").Row().Scan(&table); err != nil {
		table = 0
	}
	m.store(cacheKeyDBSize, dbSize{total: total, table: table})
	return total, table, nil
}

// deleteBatchSize bounds one retention delete. Deleting a year of backlog in a
// single statement would hold locks for a long time and bloat the transaction
// log; batches keep each statement short and let the sweep be interrupted.
const deleteBatchSize = 5000

// DeleteMessagesOlderThan removes every message whose timestamp is before cutoff
// ("YYYY-MM-DD HH:MM:SS", compared lexicographically as Postgres text) and
// returns how many rows were deleted. It works in batches until nothing is left.
//
// The aggregate cache is flushed afterwards: rows disappeared, so a cached count
// would be wrong rather than merely late.
func (m *messageRepository) DeleteMessagesOlderThan(cutoff string) (int64, error) {
	var total int64
	for {
		result := m.db.Exec(
			`DELETE FROM messages WHERE id IN (
				SELECT id FROM messages WHERE "timestamp" < ? ORDER BY "timestamp" LIMIT ?
			)`, cutoff, deleteBatchSize)
		if result.Error != nil {
			return total, result.Error
		}
		total += result.RowsAffected
		if result.RowsAffected < deleteBatchSize {
			break
		}
	}

	if total > 0 {
		m.invalidateAggregates()
	}
	return total, nil
}
