package message_model

import (
	"encoding/json"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

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
}

func (m *Message) BeforeCreate(tx *gorm.DB) (err error) {
	m.Id = uuid.New().String()
	return
}
