// Package model contains the Cloud API persistence models. They are separate
// from the instance model so Cloud credentials and event state never leak into
// the existing Baileys instance JSON.
package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// App is one Meta app: app id, encrypted app secret, webhook verify-token hash
// and Graph version. A deployment can have one app initially and more later.
type App struct {
	AppID           string    `json:"appId" gorm:"column:app_id;primaryKey"`
	AppSecretEnc    string    `json:"-" gorm:"column:app_secret_enc;type:text;not null"`
	VerifyTokenHash string    `json:"-" gorm:"column:verify_token_hash;type:text;not null;uniqueIndex"`
	GraphVersion    string    `json:"graphVersion" gorm:"column:graph_version;not null;default:v27.0"`
	CreatedAt       time.Time `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt       time.Time `json:"updatedAt" gorm:"autoUpdateTime"`
}

func (App) TableName() string { return "cloud_apps" }

// Credentials is the per-instance Meta configuration. The access token is
// encrypted at rest. The registration PIN is never stored.
type Credentials struct {
	InstanceID          string     `json:"instanceId" gorm:"column:instance_id;type:uuid;primaryKey"`
	AppID               string     `json:"appId" gorm:"column:app_id;not null"`
	WABAID              string     `json:"wabaId" gorm:"column:waba_id;not null"`
	PhoneNumberID       string     `json:"phoneNumberId" gorm:"column:phone_number_id;not null;uniqueIndex"`
	BusinessID          string     `json:"businessId" gorm:"column:business_id"`
	DisplayPhoneNumber  string     `json:"displayPhoneNumber" gorm:"column:display_phone_number"`
	VerifiedName        string     `json:"verifiedName" gorm:"column:verified_name"`
	AccessTokenEnc      string     `json:"-" gorm:"column:access_token_enc;type:text;not null"`
	TokenExpiresAt      *time.Time `json:"tokenExpiresAt,omitempty" gorm:"column:token_expires_at"`
	LastVerifiedAt      *time.Time `json:"lastVerifiedAt,omitempty" gorm:"column:last_verified_at"`
	LastVerifyError     string     `json:"lastVerifyError,omitempty" gorm:"column:last_verify_error;type:text"`
	Registered          bool       `json:"registered" gorm:"column:registered;not null;default:false"`
	RegisteredAt        *time.Time `json:"registeredAt,omitempty" gorm:"column:registered_at"`
	RegistrationStatus  string     `json:"registrationStatus,omitempty" gorm:"column:registration_status"`
	LastRegistrationErr string     `json:"lastRegistrationError,omitempty" gorm:"column:last_registration_error;type:text"`
	QualityRating       string     `json:"qualityRating,omitempty" gorm:"column:quality_rating"`
	MessagingLimit      string     `json:"messagingLimit,omitempty" gorm:"column:messaging_limit"`
	EnforceWindow       bool       `json:"enforceWindow" gorm:"column:enforce_window;not null"`
	CreatedAt           time.Time  `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt           time.Time  `json:"updatedAt" gorm:"autoUpdateTime"`
}

func (Credentials) TableName() string { return "cloud_api_credentials" }

// Contact is the BSUID-first identity record. Phone is optional.
type Contact struct {
	InstanceID  string    `json:"instanceId" gorm:"column:instance_id;type:uuid;primaryKey"`
	ContactID   string    `json:"contactId" gorm:"column:contact_id;primaryKey"`
	BSUID       string    `json:"bsuid,omitempty" gorm:"column:bsuid"`
	ParentBSUID string    `json:"parentBsuid,omitempty" gorm:"column:parent_bsuid"`
	WAID        string    `json:"waId,omitempty" gorm:"column:wa_id"`
	Username    string    `json:"username,omitempty" gorm:"column:username"`
	DisplayName string    `json:"displayName,omitempty" gorm:"column:display_name"`
	FirstSeenAt time.Time `json:"firstSeenAt" gorm:"autoCreateTime"`
	LastSeenAt  time.Time `json:"lastSeenAt" gorm:"autoUpdateTime"`
}

func (Contact) TableName() string { return "cloud_contacts" }

// ContactIdentity is the current and historical identity mapping for one
// contact. BSUID continuity is scoped to an instance/WABA.
type ContactIdentity struct {
	ID            string     `json:"id" gorm:"column:id;type:uuid;primaryKey"`
	InstanceID    string     `json:"instanceId" gorm:"column:instance_id;type:uuid;not null;index"`
	ContactID     string     `json:"contactId" gorm:"column:contact_id;not null;index"`
	IdentityType  string     `json:"identityType" gorm:"column:identity_type;not null"`
	IdentityValue string     `json:"identityValue" gorm:"column:identity_value;not null"`
	ValidFrom     time.Time  `json:"validFrom" gorm:"column:valid_from;not null"`
	ValidTo       *time.Time `json:"validTo,omitempty" gorm:"column:valid_to"`
	SourceEventID string     `json:"sourceEventId,omitempty" gorm:"column:source_event_id"`
}

func (ContactIdentity) TableName() string { return "cloud_contact_identities" }

func (c *ContactIdentity) BeforeCreate(_ *gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	return nil
}

// IdentityEvent stores the raw Meta identity transition even when it cannot be
// applied (for example an unknown WABA/contact).
type IdentityEvent struct {
	ID          string     `json:"id" gorm:"column:id;type:uuid;primaryKey"`
	InstanceID  string     `json:"instanceId" gorm:"column:instance_id;type:uuid;not null;index"`
	EventType   string     `json:"eventType" gorm:"column:event_type;not null"`
	WaID        string     `json:"waId,omitempty" gorm:"column:wa_id"`
	OldBSUID    string     `json:"oldBsuid,omitempty" gorm:"column:old_bsuid"`
	NewBSUID    string     `json:"newBsuid,omitempty" gorm:"column:new_bsuid"`
	ParentBSUID string     `json:"parentBsuid,omitempty" gorm:"column:parent_bsuid"`
	EventTime   time.Time  `json:"eventTime" gorm:"column:event_time"`
	ReceivedAt  time.Time  `json:"receivedAt" gorm:"column:received_at;autoCreateTime"`
	Payload     string     `json:"payload" gorm:"column:payload;type:jsonb;not null"`
	AppliedAt   *time.Time `json:"appliedAt,omitempty" gorm:"column:applied_at"`
	Error       string     `json:"error,omitempty" gorm:"column:error;type:text"`
}

func (IdentityEvent) TableName() string { return "cloud_identity_events" }

func (i *IdentityEvent) BeforeCreate(_ *gorm.DB) error {
	if i.ID == "" {
		i.ID = uuid.New().String()
	}
	return nil
}

// RateBucket is the per-phone-number token bucket used by the shared limiter.
type RateBucket struct {
	PhoneNumberID string     `json:"phoneNumberId" gorm:"column:phone_number_id;primaryKey"`
	Tokens        float64    `json:"tokens" gorm:"column:tokens;not null"`
	EffectiveRPS  float64    `json:"effectiveRps" gorm:"column:effective_rps;not null"`
	UpdatedAt     time.Time  `json:"updatedAt" gorm:"column:updated_at;not null"`
	Last429At     *time.Time `json:"last429At,omitempty" gorm:"column:last_429_at"`
	PausedUntil   *time.Time `json:"pausedUntil,omitempty" gorm:"column:paused_until"`
}

func (RateBucket) TableName() string { return "cloud_rate_buckets" }

// Template is the local cache of a Meta message template.
type Template struct {
	ID             string     `json:"id" gorm:"column:id;type:uuid;primaryKey"`
	InstanceID     string     `json:"instanceId" gorm:"column:instance_id;type:uuid;not null;uniqueIndex:uni_cloud_template"`
	TemplateID     string     `json:"templateId,omitempty" gorm:"column:template_id"`
	Name           string     `json:"name" gorm:"column:name;not null;uniqueIndex:uni_cloud_template"`
	Language       string     `json:"language" gorm:"column:language;not null;uniqueIndex:uni_cloud_template"`
	Category       string     `json:"category,omitempty" gorm:"column:category"`
	Status         string     `json:"status,omitempty" gorm:"column:status"`
	Components     string     `json:"components" gorm:"column:components;type:jsonb;not null"`
	RejectedReason string     `json:"rejectedReason,omitempty" gorm:"column:rejected_reason;type:text"`
	QualityScore   string     `json:"qualityScore,omitempty" gorm:"column:quality_score;type:text"`
	LastSyncedAt   *time.Time `json:"lastSyncedAt,omitempty" gorm:"column:last_synced_at"`
	CreatedAt      time.Time  `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `json:"updatedAt" gorm:"autoUpdateTime"`
}

func (Template) TableName() string { return "cloud_templates" }

func (t *Template) BeforeCreate(_ *gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	return nil
}

// ChatWindow is the 24-hour customer-service window keyed by Cloud contact.
type ChatWindow struct {
	InstanceID    string    `json:"instanceId" gorm:"column:instance_id;type:uuid;primaryKey"`
	ContactID     string    `json:"contactId" gorm:"column:contact_id;primaryKey"`
	ChatJID       string    `json:"chatJid,omitempty" gorm:"column:chat_jid"`
	LastInboundAt time.Time `json:"lastInboundAt" gorm:"column:last_inbound_at;not null"`
	ExpiresAt     time.Time `json:"expiresAt" gorm:"column:expires_at;not null"`
	LastMessageID string    `json:"lastMessageId,omitempty" gorm:"column:last_message_id"`
}

func (ChatWindow) TableName() string { return "cloud_chat_windows" }

// WebhookEvent is the webhook dedupe/replay claim.
type WebhookEvent struct {
	EventKey    string     `json:"eventKey" gorm:"column:event_key;primaryKey"`
	InstanceID  string     `json:"instanceId,omitempty" gorm:"column:instance_id;type:uuid"`
	EventType   string     `json:"eventType" gorm:"column:event_type;not null"`
	ReceivedAt  time.Time  `json:"receivedAt" gorm:"column:received_at;autoCreateTime"`
	ProcessedAt *time.Time `json:"processedAt,omitempty" gorm:"column:processed_at"`
	Attempts    int        `json:"attempts" gorm:"column:attempts;not null;default:0"`
	LastError   string     `json:"lastError,omitempty" gorm:"column:last_error;type:text"`
}

func (WebhookEvent) TableName() string { return "cloud_webhook_events" }

// MediaJob is the durable inbound media download queue.
type MediaJob struct {
	ID            string     `json:"id" gorm:"column:id;type:uuid;primaryKey"`
	InstanceID    string     `json:"instanceId" gorm:"column:instance_id;type:uuid;not null;uniqueIndex:uni_cloud_media_job"`
	MessageID     string     `json:"messageId" gorm:"column:message_id;not null;uniqueIndex:uni_cloud_media_job"`
	MediaID       string     `json:"mediaId" gorm:"column:media_id;not null"`
	MimeType      string     `json:"mimeType,omitempty" gorm:"column:mime_type"`
	Filename      string     `json:"filename,omitempty" gorm:"column:filename"`
	DeclaredSize  int64      `json:"declaredSize,omitempty" gorm:"column:declared_size"`
	Status        string     `json:"status" gorm:"column:status;not null;default:pending"`
	Attempts      int        `json:"attempts" gorm:"column:attempts;not null;default:0"`
	NextAttemptAt time.Time  `json:"nextAttemptAt" gorm:"column:next_attempt_at"`
	LastError     string     `json:"lastError,omitempty" gorm:"column:last_error;type:text"`
	CreatedAt     time.Time  `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt     time.Time  `json:"updatedAt" gorm:"autoUpdateTime"`
	CompletedAt   *time.Time `json:"completedAt,omitempty" gorm:"column:completed_at"`
}

func (MediaJob) TableName() string { return "cloud_media_jobs" }

func (j *MediaJob) BeforeCreate(_ *gorm.DB) error {
	if j.ID == "" {
		j.ID = uuid.New().String()
	}
	return nil
}

// StatusEvent is the Cloud delivery/error history.
type StatusEvent struct {
	ID              string    `json:"id" gorm:"column:id;type:uuid;primaryKey"`
	InstanceID      string    `json:"instanceId" gorm:"column:instance_id;type:uuid;not null;uniqueIndex:uni_cloud_status_event"`
	MessageID       string    `json:"messageId" gorm:"column:message_id;not null;uniqueIndex:uni_cloud_status_event"`
	Status          string    `json:"status" gorm:"column:status;not null;uniqueIndex:uni_cloud_status_event"`
	RecipientUserID string    `json:"recipientUserId,omitempty" gorm:"column:recipient_user_id"`
	RecipientID     string    `json:"recipientId,omitempty" gorm:"column:recipient_id"`
	ErrorCode       string    `json:"errorCode,omitempty" gorm:"column:error_code"`
	ErrorTitle      string    `json:"errorTitle,omitempty" gorm:"column:error_title"`
	ErrorMessage    string    `json:"errorMessage,omitempty" gorm:"column:error_message;type:text"`
	OccurredAt      time.Time `json:"occurredAt" gorm:"column:occurred_at;not null;uniqueIndex:uni_cloud_status_event"`
	CreatedAt       time.Time `json:"createdAt" gorm:"autoCreateTime"`
}

func (StatusEvent) TableName() string { return "cloud_status_events" }

func (s *StatusEvent) BeforeCreate(_ *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.New().String()
	}
	return nil
}

// AccountEvent stores account/template/security/quality events.
type AccountEvent struct {
	ID         string    `json:"id" gorm:"column:id;type:uuid;primaryKey"`
	InstanceID string    `json:"instanceId" gorm:"column:instance_id;type:uuid;not null"`
	Field      string    `json:"field" gorm:"column:field;not null"`
	EventType  string    `json:"eventType,omitempty" gorm:"column:event_type"`
	Payload    string    `json:"payload" gorm:"column:payload;type:jsonb;not null"`
	ReceivedAt time.Time `json:"receivedAt" gorm:"column:received_at;autoCreateTime"`
}

func (AccountEvent) TableName() string { return "cloud_account_events" }

func (a *AccountEvent) BeforeCreate(_ *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}

// UserPreference stores Meta's marketing preference webhook state.
type UserPreference struct {
	InstanceID      string     `json:"instanceId" gorm:"column:instance_id;type:uuid;primaryKey"`
	ContactID       string     `json:"contactId" gorm:"column:contact_id;primaryKey"`
	MarketingOptOut bool       `json:"marketingOptOut" gorm:"column:marketing_opt_out;not null;default:false"`
	SourceEventAt   *time.Time `json:"sourceEventAt,omitempty" gorm:"column:source_event_at"`
	UpdatedAt       time.Time  `json:"updatedAt" gorm:"autoUpdateTime"`
}

func (UserPreference) TableName() string { return "cloud_user_preferences" }
