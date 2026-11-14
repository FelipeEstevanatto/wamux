package message_model

import "time"

// ChatState is the WhatsApp app-state for one conversation: archived, pinned,
// muted, or explicitly marked unread. It is separate from chat_reads (which
// tracks where this manager has read to).
type ChatState struct {
	InstanceId   string    `json:"instance_id" gorm:"column:instance_id;primaryKey"`
	ChatJid      string    `json:"chat_jid" gorm:"column:chat_jid;primaryKey"`
	Archived     bool      `json:"archived" gorm:"column:archived"`
	Pinned       bool      `json:"pinned" gorm:"column:pinned"`
	MutedUntil   int64     `json:"muted_until" gorm:"column:muted_until"` // Unix seconds; -1 = forever; 0 = unmuted
	MarkedUnread bool      `json:"marked_unread" gorm:"column:marked_unread"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (ChatState) TableName() string { return "chat_states" }

// ChatStatePatch carries only the fields an app-state event supplied. A nil
// pointer means "leave the stored value alone".
type ChatStatePatch struct {
	Archived     *bool
	Pinned       *bool
	MutedUntil   *int64
	MarkedUnread *bool
}

func (p ChatStatePatch) Empty() bool {
	return p.Archived == nil && p.Pinned == nil && p.MutedUntil == nil && p.MarkedUnread == nil
}
