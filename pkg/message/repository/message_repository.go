package message_repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	applog "github.com/felipeestevanatto/wamux/pkg/applog"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	"github.com/google/uuid"
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
	// InsertHistoryMessages stores rows recovered from a history sync. A row that
	// already exists is left as it is. A failed chunk is retried once and then
	// written row by row, so one poison row cannot drop the rest of the batch.
	InsertHistoryMessages(messages []message_model.Message) (HistoryInsertReport, error)
	// MarkMessageDeleted turns a revoked message into a tombstone. See the method.
	MarkMessageDeleted(row message_model.Message, chats []string, direct bool) (bool, error)
	// LIDJids lists the LID-addressed chats and senders still stored.
	LIDJids() ([]string, error)
	// RewriteJid moves every row from a LID to its phone number.
	RewriteJid(lid, phone, source string) (int64, error)
	// MarkChatsRead moves the read position of chats forward to at, never back.
	MarkChatsRead(instanceId string, chats []string, at string) error
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
	DeleteOrphanReceipts(cutoff string) (int64, error)

	// History readback (GET /chat/history and GET /chat/chats).
	ListMessages(instanceId, chatJid, before string, limit int) ([]message_model.Message, error)
	// ListMessagesPage is ListMessages with a keyset cursor. beforeID is the id
	// of the last row of the previous page; with it the page is exactly limit
	// rows, even when many messages share one timestamp.
	ListMessagesPage(instanceId, chatJid, before, beforeID string, limit int) ([]message_model.Message, error)
	SearchMessages(instanceId, chatJid, text, before string, limit int) ([]message_model.Message, error)
	// SearchMessagesPage is SearchMessages with the same keyset cursor.
	SearchMessagesPage(instanceId, chatJid, text, before, beforeID string, limit int) ([]message_model.Message, error)
	// Filtered variants add timestamp range, message type and direction.
	ListMessagesFiltered(instanceId, chatJid string, filter MessageFilter) ([]message_model.Message, error)
	SearchMessagesFiltered(instanceId, chatJid, text string, filter MessageFilter) ([]message_model.Message, error)

	// Send-operation records for the Idempotency-Key contract.
	BeginSendOperation(ctx context.Context, instanceId, key, payloadHash string) (*message_model.SendOperation, bool, error)
	CompleteSendOperation(ctx context.Context, id, state string, statusCode int, response []byte, messageID, errText string) error
	GetSendOperation(ctx context.Context, instanceId, id string) (*message_model.SendOperation, error)
	ListSendOperations(ctx context.Context, instanceId, state string, limit int) ([]message_model.SendOperation, error)
	ListChats(instanceId string, limit int) ([]ChatSummary, error)

	// DistinctSenders lists the bare `sender_jid` values that authored messages
	// in one conversation, for labelling group members.
	DistinctSenders(instanceId, chatJid string) ([]string, error)

	// Chat state synced from WhatsApp app-state and history.
	SetChatState(instanceId, chatJid string, patch message_model.ChatStatePatch) error
	ChatStates(instanceId string) (map[string]message_model.ChatState, error)
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
	// UnreadCount is the inbound messages newer than LastReadAt, the read position.
	UnreadCount  int64  `json:"unread_count" gorm:"column:unread_count"`
	LastReadAt   string `json:"last_read_at" gorm:"column:last_read_at"`
	Archived     bool   `json:"archived" gorm:"column:archived"`
	Pinned       bool   `json:"pinned" gorm:"column:pinned"`
	MutedUntil   int64  `json:"muted_until" gorm:"column:muted_until"`
	MarkedUnread bool   `json:"marked_unread" gorm:"column:marked_unread"`

	// Name is the resolved display name (group subject or contact name). It is
	// not a column: the service fills it after the query, so the repository stays
	// free of WhatsApp lookups.
	Name string `json:"name,omitempty" gorm:"-"`
}

// MessageFilter is the optional filter set for history/search pages. Zero
// strings and a nil FromMe mean "no filter". Before/BeforeID are the keyset
// cursor; Exact controls whether a full page is trimmed exactly (keyset) or
// padded to a whole second (legacy timestamp-only cursor).
type MessageFilter struct {
	Before      string
	BeforeID    string
	After       string
	Until       string
	MessageType string
	FromMe      *bool
	Limit       int
	Exact       bool
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
		"file_name", "file_size", "is_forwarded", "page_count", "file_thumbnail",
		"album_id", "album_index",
	}
	if len(message.Referral) > 0 {
		updates = append(updates, "referral")
	}

	return updates
}

// statusRankSQL ranks a delivery state. Anything else, including Sent and Received,
// is the starting point.
const statusRankSQL = `COALESCE(array_position(ARRAY['Delivered','Read','Played']::text[], %s::text), 0)`

// keepHighestStatus is the status assignment of every upsert. Delivery state only
// moves forward: receipts can arrive out of order, and a send can record "Sent"
// after its "Delivered" receipt has already landed.
var keepHighestStatus = clause.Assignment{
	Column: clause.Column{Name: "status"},
	Value: gorm.Expr("CASE WHEN " + fmt.Sprintf(statusRankSQL, "excluded.status") + " >= " +
		fmt.Sprintf(statusRankSQL, "messages.status") + " THEN excluded.status ELSE messages.status END"),
}

// statusRank is statusRankSQL for rows that have not reached the database.
func statusRank(status string) int {
	switch status {
	case "Played":
		return 3
	case "Read":
		return 2
	case "Delivered":
		return 1
	}
	return 0
}

// upsertAssignments is the DO UPDATE list for cols.
func upsertAssignments(cols []string) clause.Set {
	others := make([]string, 0, len(cols))
	for _, col := range cols {
		if col != "status" {
			others = append(others, col)
		}
	}
	return append(clause.AssignmentColumns(others), keepHighestStatus)
}

// collapseDuplicates leaves one row per message. Postgres refuses an upsert that
// touches a row twice, which would fail the whole batch: a Delivered and a Read
// for one message often share the batching window. The latest row wins, keeping
// the furthest status seen.
func collapseDuplicates(rows []message_model.Message) []message_model.Message {
	seen := make(map[[2]string]int, len(rows))
	out := rows[:0:0]
	for _, row := range rows {
		key := [2]string{row.InstanceId, row.MessageID}
		at, dup := seen[key]
		if !dup {
			seen[key] = len(out)
			out = append(out, row)
			continue
		}
		if statusRank(out[at].Status) > statusRank(row.Status) {
			row.Status = out[at].Status
		}
		out[at] = row
	}
	return out
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

// messageConflictColumns is the upsert key. It must match the unique index
// uni_messages_instance_message: see message_model.Message.
var messageConflictColumns = []clause.Column{{Name: "instance_id"}, {Name: "message_id"}}

// writer skips GORM's implicit BEGIN/COMMIT. Each write here is one statement, so
// the transaction bought nothing and cost two round trips: 1.15 ms down to 0.50 ms
// for a single row, about what hand-written SQL measured.
func (m *messageRepository) writer() *gorm.DB {
	return m.db.Session(&gorm.Session{SkipDefaultTransaction: true})
}

func (m *messageRepository) InsertMessage(message message_model.Message) error {
	return m.writer().Clauses(upsertClause(messageUpdateColumns(message))).Create(&message).Error
}

// upsertClause is the conflict handling of a write. A content row must not bring
// back a revoked message: the persist pool can land the original after its revoke,
// since the two run on different workers. Receipts still move a tombstone's status.
func upsertClause(cols []string) clause.OnConflict {
	conflict := clause.OnConflict{Columns: messageConflictColumns, DoUpdates: upsertAssignments(cols)}
	if len(cols) > 1 {
		conflict.Where = clause.Where{Exprs: []clause.Expression{
			clause.Expr{SQL: "messages.message_type IS DISTINCT FROM ?", Vars: []any{message_model.TypeDeleted}},
		}}
	}
	return conflict
}

// tombstoneAssignments empties everything a revoked message said. Its place in the
// conversation (timestamp, chat, sender, direction) and its delivery state stay.
var tombstoneAssignments = clause.Set{
	{Column: clause.Column{Name: "message_type"}, Value: message_model.TypeDeleted},
	{Column: clause.Column{Name: "text_content"}, Value: ""},
	{Column: clause.Column{Name: "media_url"}, Value: ""},
	{Column: clause.Column{Name: "media_mimetype"}, Value: ""},
	{Column: clause.Column{Name: "quoted_message_id"}, Value: ""},
	{Column: clause.Column{Name: "file_name"}, Value: ""},
	{Column: clause.Column{Name: "file_size"}, Value: 0},
	{Column: clause.Column{Name: "page_count"}, Value: 0},
	{Column: clause.Column{Name: "is_forwarded"}, Value: false},
	{Column: clause.Column{Name: "file_thumbnail"}, Value: nil},
	{Column: clause.Column{Name: "referral"}, Value: nil},
}

// MarkMessageDeleted empties the message row.MessageID, reporting whether it did.
// row is what gets stored when the original is not there, which also covers a
// revoke that overtook its message.
//
// A revoke is just a message someone sent, so it is only obeyed inside the chat it
// arrived in (chats holds that chat's JIDs). In a direct chat each side can revoke
// only what it wrote, so direct also requires row.IsFromMe to match. In a group an
// admin may revoke anyone's message, which the stored row cannot tell apart.
//
// Edits of the message hold its text in their own rows, so they go with it.
func (m *messageRepository) MarkMessageDeleted(row message_model.Message, chats []string, direct bool) (bool, error) {
	row.MessageType = message_model.TypeDeleted

	scope := []clause.Expression{clause.Expr{SQL: "messages.chat_jid IN ?", Vars: []any{chats}}}
	if direct {
		scope = append(scope, clause.Expr{SQL: "messages.is_from_me = ?", Vars: []any{row.IsFromMe}})
	}
	result := m.writer().Clauses(clause.OnConflict{
		Columns:   messageConflictColumns,
		DoUpdates: tombstoneAssignments,
		Where:     clause.Where{Exprs: scope},
	}).Create(&row)
	if result.Error != nil || result.RowsAffected == 0 {
		return false, result.Error
	}

	err := m.writer().Model(&message_model.Message{}).
		Where("instance_id = ? AND quoted_message_id = ? AND message_type = ?", row.InstanceId, row.MessageID, "edit").
		Updates(map[string]any{"message_type": "delete", "text_content": ""}).Error
	return true, err
}

// sortForCounterLocks orders rows by the message_counters key the rollup trigger
// updates: instance, day, status, source. The trigger locks one counter row per
// message for the whole statement, so two concurrent batches touching the same
// counters in different orders deadlock; measured at saturation, that failed 94%
// of batches. A shared order means they queue instead.
//
// The day must be compared on its own, not via the full timestamp, or two rows
// from the same day would order by time and take their counters in an order
// that disagrees with another batch. message_id then orders the message rows
// themselves, for batches that update the same ids.
func sortForCounterLocks(rows []message_model.Message) {
	sort.Slice(rows, func(a, b int) bool {
		x, y := &rows[a], &rows[b]
		if x.InstanceId != y.InstanceId {
			return x.InstanceId < y.InstanceId
		}
		if dx, dy := counterDay(x.Timestamp), counterDay(y.Timestamp); dx != dy {
			return dx < dy
		}
		if x.Status != y.Status {
			return x.Status < y.Status
		}
		if x.Source != y.Source {
			return x.Source < y.Source
		}
		return x.MessageID < y.MessageID
	})
}

// counterDay is the day part of a "YYYY-MM-DD HH:MM:SS" timestamp, matching the
// substr(timestamp, 1, 10) in the rollup trigger.
func counterDay(timestamp string) string {
	if len(timestamp) > 10 {
		return timestamp[:10]
	}
	return timestamp
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
		group := collapseDuplicates(groups[key])
		sortForCounterLocks(group)
		cols := strings.Split(key, ",")
		for start := 0; start < len(group); start += batchInsertChunk {
			end := start + batchInsertChunk
			if end > len(group) {
				end = len(group)
			}
			chunk := group[start:end]
			if err := m.writer().Clauses(upsertClause(cols)).Create(&chunk).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *messageRepository) LIDJids() ([]string, error) {
	var jids []string
	err := m.db.Raw(`SELECT chat_jid FROM messages WHERE chat_jid LIKE '%@lid'
		UNION SELECT sender_jid FROM messages WHERE sender_jid LIKE '%@lid'`).Scan(&jids).Error
	return jids, err
}

// RewriteJid runs in one transaction: a chat half moved would show as two. source
// changes with the chat, and the rollup trigger keeps the counters right.
func (m *messageRepository) RewriteJid(lid, phone, source string) (int64, error) {
	var rewritten int64
	err := m.db.Transaction(func(tx *gorm.DB) error {
		chats := tx.Exec(`UPDATE messages SET chat_jid = ?, source = ? WHERE chat_jid = ?`, phone, source, lid)
		if chats.Error != nil {
			return chats.Error
		}
		senders := tx.Exec(`UPDATE messages SET sender_jid = ? WHERE sender_jid = ?`, phone, lid)
		if senders.Error != nil {
			return senders.Error
		}
		rewritten = chats.RowsAffected + senders.RowsAffected

		err := tx.Exec(`INSERT INTO chat_reads (instance_id, chat_jid, read_at)
			SELECT instance_id, ?, read_at FROM chat_reads WHERE chat_jid = ?
			ON CONFLICT (instance_id, chat_jid) DO UPDATE SET read_at =
				CASE WHEN excluded.read_at > chat_reads.read_at THEN excluded.read_at ELSE chat_reads.read_at END`,
			phone, lid).Error
		if err != nil {
			return err
		}
		return tx.Exec(`DELETE FROM chat_reads WHERE chat_jid = ?`, lid).Error
	})
	return rewritten, err
}

func (m *messageRepository) MarkChatsRead(instanceId string, chats []string, at string) error {
	rows := make([]message_model.ChatRead, 0, len(chats))
	for _, chat := range chats {
		rows = append(rows, message_model.ChatRead{InstanceId: instanceId, ChatJid: chat, ReadAt: at})
	}
	if len(rows) == 0 {
		return nil
	}
	return m.writer().Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "instance_id"}, {Name: "chat_jid"}},
		DoUpdates: clause.Set{{
			Column: clause.Column{Name: "read_at"},
			Value:  gorm.Expr("CASE WHEN excluded.read_at > chat_reads.read_at THEN excluded.read_at ELSE chat_reads.read_at END"),
		}},
	}).Create(&rows).Error
}

// HistoryInsertReport is the outcome of one history batch. Total counts the input
// rows; Duplicates were collapsed before writing; SkippedExisting are rows the
// database already had (ON CONFLICT DO NOTHING); Failed are rows that could not be
// written even one by one.
type HistoryInsertReport struct {
	Total           int `json:"total"`
	Inserted        int `json:"inserted"`
	SkippedExisting int `json:"skippedExisting"`
	Duplicates      int `json:"duplicates"`
	Failed          int `json:"failed"`
}

// InsertHistoryMessages never updates: what a history sync says about a message is
// older than what was recorded live, whether that is its status, its stored media
// or the fact that it was revoked.
//
// A batch is written in chunks. If a chunk fails (a malformed row, a constraint
// the import cannot satisfy), it is retried once and then every row is written
// alone, so one poison row does not discard hundreds of good ones. The caller
// logs the report and keeps the failed rows out of the "already stored" set.
func (m *messageRepository) InsertHistoryMessages(messages []message_model.Message) (HistoryInsertReport, error) {
	report := HistoryInsertReport{Total: len(messages)}
	rows := collapseDuplicates(messages)
	report.Duplicates = len(messages) - len(rows)
	sortForCounterLocks(rows)

	for start := 0; start < len(rows); start += batchInsertChunk {
		end := min(start+batchInsertChunk, len(rows))
		chunk := rows[start:end]
		res := m.writer().Clauses(clause.OnConflict{Columns: messageConflictColumns, DoNothing: true}).Create(&chunk)
		if res.Error == nil {
			report.Inserted += int(res.RowsAffected)
			report.SkippedExisting += len(chunk) - int(res.RowsAffected)
			continue
		}

		// The whole statement is atomic, so a failed chunk inserted nothing.
		// Retry once for a transient error, then fall back row by row.
		res = m.writer().Clauses(clause.OnConflict{Columns: messageConflictColumns, DoNothing: true}).Create(&chunk)
		if res.Error == nil {
			report.Inserted += int(res.RowsAffected)
			report.SkippedExisting += len(chunk) - int(res.RowsAffected)
			continue
		}
		for i := range chunk {
			row := chunk[i]
			one := m.writer().Clauses(clause.OnConflict{Columns: messageConflictColumns, DoNothing: true}).Create(&row)
			if one.Error != nil {
				report.Failed++
				continue
			}
			report.Inserted += int(one.RowsAffected)
			report.SkippedExisting += 1 - int(one.RowsAffected)
		}
	}

	if report.Failed > 0 && report.Inserted == 0 && report.SkippedExisting == 0 {
		return report, fmt.Errorf("history batch: all %d row(s) failed", report.Failed)
	}
	return report, nil
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

// ListMessages returns a conversation's stored messages, newest first. A full page
// can exceed limit: see the end of the function.
// `before` (a "YYYY-MM-DD HH:MM:SS" timestamp) pages backwards: it returns the
// messages strictly older than the newest one already seen.
func (m *messageRepository) ListMessages(instanceId, chatJid, before string, limit int) ([]message_model.Message, error) {
	return m.ListMessagesFiltered(instanceId, chatJid, MessageFilter{Before: before, Limit: limit})
}

// ListMessagesPage is ListMessages with a keyset cursor. When beforeID is set,
// the page is exactly limit rows even inside a timestamp shared by many
// messages; the cursor advances by (timestamp, id).
func (m *messageRepository) ListMessagesPage(instanceId, chatJid, before, beforeID string, limit int) ([]message_model.Message, error) {
	return m.ListMessagesFiltered(instanceId, chatJid, MessageFilter{
		Before: before, BeforeID: beforeID, Limit: limit, Exact: true,
	})
}

// likeEscaper keeps user input literal inside a LIKE pattern.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// SearchMessages finds text in one conversation, newest first, paged like
// ListMessages. It relies on the (instance, chat, timestamp) index to narrow the
// rows, so it stays cheap for a conversation but would not for a whole instance.
func (m *messageRepository) SearchMessages(instanceId, chatJid, text, before string, limit int) ([]message_model.Message, error) {
	return m.SearchMessagesFiltered(instanceId, chatJid, text, MessageFilter{Before: before, Limit: limit})
}

// SearchMessagesPage is SearchMessages with the keyset cursor.
func (m *messageRepository) SearchMessagesPage(instanceId, chatJid, text, before, beforeID string, limit int) ([]message_model.Message, error) {
	return m.SearchMessagesFiltered(instanceId, chatJid, text, MessageFilter{
		Before: before, BeforeID: beforeID, Limit: limit, Exact: true,
	})
}

func (m *messageRepository) ListMessagesFiltered(instanceId, chatJid string, filter MessageFilter) ([]message_model.Message, error) {
	if instanceId == "" || chatJid == "" {
		return make([]message_model.Message, 0), nil
	}
	return m.pageMessages(func() *gorm.DB {
		return m.db.Where("instance_id = ? AND chat_jid = ?", instanceId, chatJid)
	}, filter)
}

func (m *messageRepository) SearchMessagesFiltered(instanceId, chatJid, text string, filter MessageFilter) ([]message_model.Message, error) {
	if instanceId == "" || chatJid == "" || text == "" {
		return make([]message_model.Message, 0), nil
	}
	pattern := "%" + likeEscaper.Replace(text) + "%"
	return m.pageMessages(func() *gorm.DB {
		return m.db.Where(
			`instance_id = ? AND chat_jid = ? AND LOWER(text_content) LIKE LOWER(?) ESCAPE '\'`,
			instanceId, chatJid, pattern,
		)
	}, filter)
}

// pageMessages reads one page newest first. scoped builds the filter, and is a
// function because a GORM statement cannot be reused for the second query.
func (m *messageRepository) pageMessages(scoped func() *gorm.DB, filter MessageFilter) ([]message_model.Message, error) {
	messages := make([]message_model.Message, 0)
	query := scoped()
	if filter.Before != "" {
		if filter.BeforeID != "" {
			// Composite keyset: rows strictly before (timestamp, id). id is the
			// per-row UUID, which is stable and unique even when a whole second
			// of messages shares one timestamp.
			query = query.Where(`("timestamp" < ? OR ("timestamp" = ? AND id < ?))`, filter.Before, filter.Before, filter.BeforeID)
		} else {
			query = query.Where(`"timestamp" < ?`, filter.Before)
		}
	}
	query = applyNonCursorFilters(query, filter)

	limit := clampHistoryLimit(filter.Limit)
	err := query.
		Order(`"timestamp" DESC`).
		Order("id DESC").
		Limit(limit).
		Find(&messages).Error
	if err != nil || len(messages) < limit || filter.Exact {
		// A keyset page is exact by construction; no whole-second padding.
		return messages, err
	}

	// Legacy `before`-only cursor: it has one-second precision and is exclusive,
	// so a page ending partway through a second would lose the rest of that
	// second. A page therefore always ends on a whole second and may run
	// slightly over the limit. New callers send beforeID and get exact pages.
	last := messages[len(messages)-1].Timestamp
	var sameSecond []message_model.Message
	err = applyNonCursorFilters(scoped(), filter).Where(`"timestamp" = ?`, last).
		Order("id DESC").
		Find(&sameSecond).Error
	if err != nil {
		return nil, err
	}
	keep := len(messages)
	for keep > 0 && messages[keep-1].Timestamp == last {
		keep--
	}
	return append(messages[:keep], sameSecond...), nil
}

func applyNonCursorFilters(query *gorm.DB, filter MessageFilter) *gorm.DB {
	if filter.After != "" {
		query = query.Where(`"timestamp" >= ?`, filter.After)
	}
	if filter.Until != "" {
		query = query.Where(`"timestamp" <= ?`, filter.Until)
	}
	if filter.MessageType != "" {
		query = query.Where("message_type = ?", filter.MessageType)
	}
	if filter.FromMe != nil {
		query = query.Where("is_from_me = ?", *filter.FromMe)
	}
	return query
}

// ListChats returns each conversation's newest message plus its total count,
// ordered by most recent activity. Postgres-only (DISTINCT ON), like the rest
// of the repository's raw SQL.
func (m *messageRepository) ListChats(instanceId string, limit int) ([]ChatSummary, error) {
	summaries := make([]ChatSummary, 0)
	if instanceId == "" {
		return summaries, nil
	}

	// A skip scan over idx_messages_instance_chat_ts: the recursive term jumps from
	// one chat straight to the next, the lateral takes each chat's newest row, and
	// only the chats that made the page are counted. The cost follows the number of
	// chats, not messages. DISTINCT ON with a window count read every message the
	// instance had: 5.5 s at 1M rows, on a query the manager runs on every load.
	const query = `
WITH RECURSIVE chats AS (
    (SELECT chat_jid FROM messages
      WHERE instance_id = ? AND chat_jid <> ''
      ORDER BY chat_jid LIMIT 1)
    UNION ALL
    SELECT (SELECT m.chat_jid FROM messages m
             WHERE m.instance_id = ? AND m.chat_jid > c.chat_jid
             ORDER BY m.chat_jid LIMIT 1)
      FROM chats c
     WHERE c.chat_jid IS NOT NULL
), latest AS (
    SELECT l.*
      FROM chats c
     CROSS JOIN LATERAL (
        -- Prefer the newest message a person can read. A revoke keeps the
        -- original timestamp and clears the text, so ordering by time alone
        -- makes the tombstone the preview and hides the previous message.
        -- The second branch is only reached when the chat has nothing else.
        SELECT picked.chat_jid, picked.message_id, picked."timestamp", picked.message_type,
               picked.text_content, picked.status, picked.media_url, picked.sender_jid, picked.is_from_me
          FROM (
            (
                SELECT 0 AS preview_rank, x.chat_jid, x.message_id, x."timestamp", x.message_type,
                       x.text_content, x.status, x.media_url, x.sender_jid, x.is_from_me
                  FROM messages x
                 WHERE x.instance_id = ? AND x.chat_jid = c.chat_jid
                   AND x.message_type NOT IN ('edit', 'delete', 'deleted', 'reaction', 'poll_update', 'unknown')
                 ORDER BY x."timestamp" DESC
                 LIMIT 1
            )
            UNION ALL
            (
                SELECT 1 AS preview_rank, x.chat_jid, x.message_id, x."timestamp", x.message_type,
                       x.text_content, x.status, x.media_url, x.sender_jid, x.is_from_me
                  FROM messages x
                 WHERE x.instance_id = ? AND x.chat_jid = c.chat_jid
                 ORDER BY x."timestamp" DESC
                 LIMIT 1
            )
        ) picked
         ORDER BY picked.preview_rank
         LIMIT 1
     ) l
     WHERE c.chat_jid IS NOT NULL
     ORDER BY l."timestamp" DESC
     LIMIT ?
)
SELECT t.*,
       COALESCE(r.read_at, '') AS last_read_at,
       COALESCE(cs.archived, false) AS archived,
       COALESCE(cs.pinned, false) AS pinned,
       COALESCE(cs.muted_until, 0) AS muted_until,
       COALESCE(cs.marked_unread, false) AS marked_unread,
       (SELECT count(*) FROM messages n
         WHERE n.instance_id = ? AND n.chat_jid = t.chat_jid) AS message_count,
       (SELECT count(*) FROM messages n
         WHERE n.instance_id = ? AND n.chat_jid = t.chat_jid
           AND t.chat_jid <> 'status@broadcast'
           AND NOT n.is_from_me
           AND n."timestamp" > COALESCE(r.read_at, '')
           AND n.message_type NOT IN ('edit', 'delete', 'deleted', 'reaction', 'poll_update', 'unknown')
       ) AS unread_count
  FROM latest t
  LEFT JOIN chat_states cs ON cs.instance_id = ? AND cs.chat_jid = t.chat_jid
  LEFT JOIN chat_reads r ON r.instance_id = ? AND r.chat_jid = t.chat_jid
 ORDER BY t."timestamp" DESC`

	err := m.db.Raw(query, instanceId, instanceId, instanceId, instanceId, clampHistoryLimit(limit), instanceId, instanceId, instanceId, instanceId).Scan(&summaries).Error
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
		Where("instance_id = ? AND chat_jid <> ''", instanceId).
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
		Where("instance_id = ? AND chat_jid <> ''", instanceId).
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

	stored := func() *gorm.DB { return m.db.Model(&message_model.Message{}).Where("chat_jid <> ''") }

	if err := stored().Count(&stats.Total).Error; err != nil {
		return nil, err
	}

	stored().
		Select("status as label, count(*) as total").
		Group("status").Order("total desc").
		Scan(&stats.ByStatus)

	stored().
		Select(`substr("timestamp", 1, 10) as label, count(*) as total`).
		Group(`substr("timestamp", 1, 10)`).Order("label desc").
		Limit(14).
		Scan(&stats.ByDay)

	stored().
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

// DeleteOrphanReceipts removes receipts whose message never arrived. A receipt writes
// a row in case it beat its message here, so the status is not lost. Many never get
// one: receipts for status posts, or for messages sent before the instance was linked.
func (m *messageRepository) DeleteOrphanReceipts(cutoff string) (int64, error) {
	var total int64
	for {
		result := m.db.Exec(
			`DELETE FROM messages WHERE id IN (
				SELECT id FROM messages WHERE chat_jid = '' AND "timestamp" < ? ORDER BY "timestamp" LIMIT ?
			)`, cutoff, deleteBatchSize)
		if result.Error != nil {
			return total, result.Error
		}
		total += result.RowsAffected
		if result.RowsAffected < deleteBatchSize {
			return total, nil
		}
	}
}

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

// BeginSendOperation inserts a pending operation or returns the existing one
// for the same (instance, key). The unique index makes concurrent requests
// race safely: exactly one inserts, the rest read the winner's row.
func (m *messageRepository) BeginSendOperation(ctx context.Context, instanceId, key, payloadHash string) (*message_model.SendOperation, bool, error) {
	now := time.Now().UTC()
	// WhatsApp ids are 32 hex characters. Choosing one before transport is what
	// lets a crashed pending operation be matched against the stored message.
	plannedID := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))
	op := &message_model.SendOperation{
		InstanceId:       instanceId,
		IdempotencyKey:   key,
		PayloadHash:      payloadHash,
		State:            "pending",
		PlannedMessageID: plannedID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	result := m.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(op)
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.RowsAffected == 1 {
		return op, true, nil
	}

	var existing message_model.SendOperation
	if err := m.db.WithContext(ctx).
		Where("instance_id = ? AND idempotency_key = ?", instanceId, key).
		First(&existing).Error; err != nil {
		return nil, false, err
	}
	return &existing, false, nil
}

// CompleteSendOperation records the first caller's outcome so a replay returns
// exactly the same status and body.
func (m *messageRepository) CompleteSendOperation(ctx context.Context, id, state string, statusCode int, response []byte, messageID, errText string) error {
	return m.db.WithContext(ctx).
		Model(&message_model.SendOperation{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"state":       state,
			"status_code": statusCode,
			"response":    response,
			"message_id":  messageID,
			"error":       errText,
			"updated_at":  time.Now().UTC(),
		}).Error
}

// ListSendOperations reports operations for one instance, oldest first. An
// operator uses state=pending after a restart to find in-doubt sends.
func (m *messageRepository) ListSendOperations(ctx context.Context, instanceId, state string, limit int) ([]message_model.SendOperation, error) {
	if instanceId == "" {
		return []message_model.SendOperation{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	query := m.db.WithContext(ctx).Where("instance_id = ?", instanceId)
	if state != "" {
		query = query.Where("state = ?", state)
	}
	ops := make([]message_model.SendOperation, 0, limit)
	err := query.Order("created_at ASC").Limit(limit).Find(&ops).Error
	return ops, err
}

// SetChatState writes only the fields present in patch. A missing row is
// created with the other fields at their zero values, which matches WhatsApp
// sending partial app-state updates.
func (m *messageRepository) SetChatState(instanceId, chatJid string, patch message_model.ChatStatePatch) error {
	if instanceId == "" || chatJid == "" || patch.Empty() {
		return nil
	}
	now := time.Now().UTC()
	state := message_model.ChatState{InstanceId: instanceId, ChatJid: chatJid, UpdatedAt: now}
	updates := map[string]any{"updated_at": now}
	if patch.Archived != nil {
		state.Archived = *patch.Archived
		updates["archived"] = *patch.Archived
	}
	if patch.Pinned != nil {
		state.Pinned = *patch.Pinned
		updates["pinned"] = *patch.Pinned
	}
	if patch.MutedUntil != nil {
		state.MutedUntil = *patch.MutedUntil
		updates["muted_until"] = *patch.MutedUntil
	}
	if patch.MarkedUnread != nil {
		state.MarkedUnread = *patch.MarkedUnread
		updates["marked_unread"] = *patch.MarkedUnread
	}
	return m.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "instance_id"}, {Name: "chat_jid"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&state).Error
}

func (m *messageRepository) ChatStates(instanceId string) (map[string]message_model.ChatState, error) {
	out := map[string]message_model.ChatState{}
	if instanceId == "" {
		return out, nil
	}
	var rows []message_model.ChatState
	if err := m.db.Where("instance_id = ?", instanceId).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ChatJid] = row
	}
	return out, nil
}

// GetSendOperation scopes the lookup to the authenticated instance.
func (m *messageRepository) GetSendOperation(ctx context.Context, instanceId, id string) (*message_model.SendOperation, error) {
	var op message_model.SendOperation
	err := m.db.WithContext(ctx).
		Where("instance_id = ? AND id = ?", instanceId, id).
		First(&op).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &op, nil
}
