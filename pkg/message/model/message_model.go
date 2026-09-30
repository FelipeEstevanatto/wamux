package message_model

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Message is one row of the message history. Historically it stored only
// enough for dashboard counts (status/source/timestamp); the content columns
// exist so callers can read a conversation back through GET /chat/history.
//
// The same message_id is written more than once as its state changes
// (Received -> Read/Delivered); the upsert in the repository only moves the
// status for those updates and never clobbers the content (see
// messageUpdateColumns).
type Message struct {
	Id         string `json:"id" gorm:"type:uuid;primaryKey"`
	MessageID  string `json:"message_id" gorm:"unique"`
	InstanceId string `json:"instance_id" gorm:"column:instance_id;index"`
	// Timestamp is "YYYY-MM-DD HH:MM:SS" text, so it sorts and compares
	// lexicographically. The index is what lets the retention job find the rows
	// to delete with a range scan instead of a full table scan per batch.
	Timestamp string          `json:"timestamp" gorm:"index"`
	Status    string          `json:"status"`
	Source    string          `json:"source"`
	Referral  json.RawMessage `json:"referral,omitempty" gorm:"type:jsonb"`

	// ChatJid is the conversation (peer for a direct chat, the group JID for a
	// group). SenderJid is the author of the message. Both are canonical,
	// non-device JIDs, so they join with the history endpoint's `chat` filter.
	ChatJid   string `json:"chat_jid" gorm:"column:chat_jid;index"`
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
}

func (m *Message) BeforeCreate(tx *gorm.DB) (err error) {
	m.Id = uuid.New().String()
	return
}
