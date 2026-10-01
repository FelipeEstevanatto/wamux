package instance_model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Instance struct {
	Id    string `json:"id" gorm:"type:uuid;primaryKey"`
	Name  string `json:"name"`
	Token string `json:"token" gorm:"unique"`
	// TokenHash is HMAC-SHA256(Token) under the server key: deterministic and
	// indexable, so authentication looks the instance up by it. TokenEnc is
	// AES-256-GCM(Token) for recovery/display. Together they replace storing the
	// plaintext credential. Token itself is retained for backwards compatibility
	// during migration but is cleared once encrypted (see the token migration).
	TokenHash        string    `json:"-" gorm:"column:token_hash;index"`
	TokenEnc         string    `json:"-" gorm:"column:token_enc;type:text"`
	Webhook          string    `json:"webhook"`
	RabbitmqEnable   string    `json:"rabbitmqEnable"`
	WebSocketEnable  string    `json:"websocketEnable"`
	NatsEnable       string    `json:"natsEnable"`
	Jid              string    `json:"jid" gorm:"column:jid"`
	Qrcode           string    `json:"qrcode" gorm:"type:text"`
	Connected        bool      `json:"connected"`
	Expiration       int64     `json:"expiration"`
	DisconnectReason string    `json:"disconnect_reason"`
	Events           string    `json:"events"`
	OsName           string    `json:"os_name"`
	Proxy            string    `json:"proxy"`
	ClientName       string    `json:"client_name"`
	CreatedAt        time.Time `json:"createdAt" gorm:"autoCreateTime"`

	// HmacKey is the per-instance webhook signing key, AES-256-GCM encrypted
	// with the process encryption key. It is never serialized: it is only used
	// server-side to sign outbound webhook deliveries. Empty means "use the
	// global WEBHOOK_HMAC_KEY, if configured".
	HmacKey string `json:"-" gorm:"type:text"`

	// Advanced Settings
	AlwaysOnline  bool   `json:"alwaysOnline" gorm:"default:false"`
	RejectCall    bool   `json:"rejectCall" gorm:"default:false"`
	MsgRejectCall string `json:"msgRejectCall" gorm:"default:''"`
	ReadMessages  bool   `json:"readMessages" gorm:"default:false"`
	IgnoreGroups  bool   `json:"ignoreGroups" gorm:"default:false"`
	IgnoreStatus  bool   `json:"ignoreStatus" gorm:"default:false"`

	// Per-instance S3-compatible media storage. When enabled it takes precedence
	// over the global MinIO config for this instance's inbound media webhooks.
	// S3SecretKey is AES-256-GCM encrypted at rest and never serialized.
	S3Enabled       bool   `json:"s3Enabled" gorm:"default:false"`
	S3Endpoint      string `json:"s3Endpoint"`
	S3Region        string `json:"s3Region"`
	S3Bucket        string `json:"s3Bucket"`
	S3AccessKey     string `json:"s3AccessKey"`
	S3SecretKey     string `json:"-"`
	S3PathStyle     bool   `json:"s3PathStyle"`
	S3PublicURL     string `json:"s3PublicUrl"`
	S3MediaDelivery string `json:"s3MediaDelivery"` // base64 | s3 | both
}

// AdvancedSettings representa as configurações avançadas de uma instância.
// Bool fields are pointers so omitted JSON keys are not written as false on PUT.
type AdvancedSettings struct {
	AlwaysOnline  *bool  `json:"alwaysOnline" example:"false"`
	RejectCall    *bool  `json:"rejectCall" example:"false"`
	MsgRejectCall string `json:"msgRejectCall" example:"Chamada recusada, envie uma mensagem."`
	ReadMessages  *bool  `json:"readMessages" example:"false"`
	IgnoreGroups  *bool  `json:"ignoreGroups" example:"false"`
	IgnoreStatus  *bool  `json:"ignoreStatus" example:"false"`
}

func (m *Instance) BeforeCreate(tx *gorm.DB) (err error) {
	if m.Id == "" {
		m.Id = uuid.New().String()
	}
	return
}

// BoolPtr returns a pointer to v (helper for AdvancedSettings responses/tests).
func BoolPtr(v bool) *bool {
	return &v
}
