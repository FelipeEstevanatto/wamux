package message_model

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TypeDeleted is the message_type of a message revoked for everyone. The row stays
// as a tombstone, so the conversation keeps its place, but none of the content does.
const TypeDeleted = "deleted"

// Message is one row of the message history. Historically it stored only
// enough for dashboard counts (status/source/timestamp); the content columns
// exist so callers can read a conversation back through GET /chat/history.
//
// The same message_id is written more than once as its state changes
// (Received -> Read/Delivered); the upsert in the repository only moves the
// status for those updates and never clobbers the content (see
// messageUpdateColumns).
type Message struct {
	Id string `json:"id" gorm:"type:uuid;primaryKey"`
	// A WhatsApp message id is only unique per account: two instances on this
	// server in the same conversation receive the same id, each with its own view
	// of the message (direction, chat, status). Uniqueness is therefore per
	// instance, or one tenant's row would overwrite the other's.
	MessageID  string `json:"message_id" gorm:"uniqueIndex:uni_messages_instance_message,priority:2"`
	InstanceId string `json:"instance_id" gorm:"column:instance_id;uniqueIndex:uni_messages_instance_message,priority:1"`
	// Timestamp is "YYYY-MM-DD HH:MM:SS" text, so it sorts and compares
	// lexicographically. The index is what lets the retention job find the rows
	// to delete with a range scan instead of a full table scan per batch.
	Timestamp string          `json:"timestamp" gorm:"index"`
	Status    string          `json:"status"`
	Source    string          `json:"source"`
	Referral  json.RawMessage `json:"referral,omitempty" gorm:"type:jsonb"`
	// Channel is "baileys" for the existing WhatsApp Web path and "cloud" for
	// the official Cloud API. Existing rows default to baileys.
	Channel string `json:"channel" gorm:"column:channel;not null;default:baileys;index"`
	// ContactID is the Cloud BSUID-first identity key. It is empty for Baileys.
	ContactID       string `json:"contact_id,omitempty" gorm:"column:contact_id;index"`
	SenderUserID    string `json:"sender_user_id,omitempty" gorm:"column:sender_user_id"`
	RecipientUserID string `json:"recipient_user_id,omitempty" gorm:"column:recipient_user_id"`

	// ChatJid is the conversation (peer for a direct chat, the group JID for a
	// group). SenderJid is the author of the message. Both are canonical,
	// non-device JIDs, so they join with the history endpoint's `chat` filter.
	ChatJid   string `json:"chat_jid" gorm:"column:chat_jid"`
	SenderJid string `json:"sender_jid"`

	// MessageType is the coarse type (text/image/video/...). TextContent holds
	// the body or caption. MediaUrl/MediaMimetype are set when the media was
	// stored (e.g. MinIO). QuotedMessageID references a replied-to/reacted-to
	// message. IsFromMe distinguishes sent from received.
	MessageType     string `json:"message_type"`
	TextContent     string `json:"text_content" gorm:"type:text"`
	MediaUrl        string `json:"media_url" gorm:"type:text"`
	MediaMimetype   string `json:"media_mimetype"`
	QuotedMessageID string `json:"quoted_message_id"`
	IsFromMe        bool   `json:"is_from_me"`

	// FileName and FileSize describe an attachment as WhatsApp reports it, so they
	// are known even when the file itself was not stored.
	FileName    string `json:"file_name"`
	FileSize    int64  `json:"file_size"`
	IsForwarded bool   `json:"is_forwarded"`
	// PageCount and FileThumbnail come with the message, so a document can show its
	// length and a preview without its file being downloaded.
	PageCount     uint32 `json:"page_count,omitempty"`
	FileThumbnail []byte `json:"file_thumbnail,omitempty"`
	// AlbumID is the WhatsApp album container id; AlbumIndex is the item's
	// position. Both are empty/zero for non-album messages.
	AlbumID    string `json:"album_id,omitempty" gorm:"column:album_id"`
	AlbumIndex int    `json:"album_index,omitempty" gorm:"column:album_index"`
}

// ChatRead is how far a conversation has been read in the manager: inbound messages
// newer than ReadAt count as unread. It is separate from WhatsApp's own read state,
// which auto-read would settle before anyone looked. ReadAt has Timestamp's format.
type ChatRead struct {
	InstanceId string `gorm:"column:instance_id;primaryKey"`
	ChatJid    string `gorm:"column:chat_jid;primaryKey"`
	ReadAt     string `gorm:"column:read_at"`
}

func (m *Message) BeforeCreate(tx *gorm.DB) (err error) {
	m.Id = uuid.New().String()
	return
}

func (ChatRead) TableName() string { return "chat_reads" }
