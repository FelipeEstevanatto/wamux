package message_model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SendOperation is the durable idempotency record for one POST /send request.
// It is deliberately separate from Message: a send can fail before a message
// row exists, and the same message can be replayed from the record.
//
// State is one of:
//   - pending: the send is in flight (or the process stopped while it was)
//   - sent:    the response was recorded and can be replayed
//   - failed:  the request failed; the recorded error is replayed
type SendOperation struct {
	Id             string `json:"id" gorm:"type:uuid;primaryKey"`
	InstanceId     string `json:"instance_id" gorm:"column:instance_id;uniqueIndex:uni_send_operations_instance_key,priority:1"`
	IdempotencyKey string `json:"idempotency_key" gorm:"column:idempotency_key;uniqueIndex:uni_send_operations_instance_key,priority:2"`
	// PayloadHash binds the key to the exact method/path/body. Reusing a key
	// with a different payload is a conflict, not a replay.
	PayloadHash string `json:"payload_hash" gorm:"column:payload_hash"`
	State       string `json:"state" gorm:"column:state"`
	StatusCode  int    `json:"status_code" gorm:"column:status_code"`
	// PlannedMessageID is the 32-character WhatsApp id chosen before transport.
	// It is what makes an interrupted send recoverable: a pending operation can
	// be checked against the stored message with this id.
	PlannedMessageID string `json:"planned_message_id" gorm:"column:planned_message_id"`
	// Response is the exact JSON body returned to the first caller. It is
	// capped by the middleware; a larger response replays without a body.
	Response  []byte    `json:"-" gorm:"column:response"`
	MessageID string    `json:"message_id" gorm:"column:message_id"`
	Error     string    `json:"error" gorm:"column:error"`
	CreatedAt time.Time `json:"created_at" gorm:"column:created_at"`
	UpdatedAt time.Time `json:"updated_at" gorm:"column:updated_at"`
}

func (SendOperation) TableName() string { return "send_operations" }

func (o *SendOperation) BeforeCreate(_ *gorm.DB) error {
	if o.Id == "" {
		o.Id = uuid.New().String()
	}
	if o.State == "" {
		o.State = "pending"
	}
	return nil
}
