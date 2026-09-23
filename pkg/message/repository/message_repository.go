package message_repository

import (
	"time"

	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	"github.com/patrickmn/go-cache"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MessageRepository interface {
	InsertMessage(message message_model.Message) error
	GetMessageByID(messageID string) (*message_model.Message, error)
	DeleteAllMessages() (int64, error)
	GetLatestMessageID(source string) (string, string, error)
	GetStats() (*MessageStats, error)
	CountByInstance(instanceId string) (int64, error)
	CountChatsByInstance(instanceId string) (int64, error)
	DatabaseSizeBytes() (totalBytes int64, messagesBytes int64, err error)
	DeleteMessagesOlderThan(cutoff string) (int64, error)
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

	// aggCache memoizes the dashboard's aggregations. They are whole-table
	// scans: measured at 2M rows, /server/stats costs ~0.5s and
	// CountChatsByInstance ~0.9s, and both are re-run on every 15s poll of every
	// open dashboard. The values they show change slowly, so a short TTL is
	// invisible in the UI while making the DB cost independent of client count.
	//
	// nil when caching is disabled (ttl <= 0).
	aggCache *cache.Cache
}

// Option configures the repository at construction time.
type Option func(*messageRepository)

// WithAggregateCacheTTL caches the dashboard aggregations for ttl. Zero (the
// default) disables the cache, which is what tests want.
func WithAggregateCacheTTL(ttl time.Duration) Option {
	return func(m *messageRepository) {
		if ttl > 0 {
			m.aggCache = cache.New(ttl, 2*ttl)
		}
	}
}

func messageUpdateColumns(message message_model.Message) []string {
	updates := []string{"timestamp", "status", "source"}
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

func (m *messageRepository) DeleteAllMessages() (int64, error) {
	result := m.db.Exec("DELETE FROM messages")
	m.invalidateAggregates()
	return result.RowsAffected, result.Error
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

// NewMessageRepository builds the repository. Pass WithAggregateCacheTTL to
// memoize the dashboard aggregations; without it every call hits the database.
func NewMessageRepository(db *gorm.DB, opts ...Option) MessageRepository {
	m := &messageRepository{db: db}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// CountByInstance counts persisted messages attributed to one instance. Only
// rows written after the instance_id column was added carry it, so older rows
// are not counted.
func (m *messageRepository) CountByInstance(instanceId string) (int64, error) {
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
