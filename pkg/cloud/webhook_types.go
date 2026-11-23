package cloud

// Typed payloads for the webhook fields Wamux understands. Keep unknown fields
// additive: Meta may add keys at any time, and the raw value is always retained
// in cloud_account_events.

type UserIDUpdateValue struct {
	WAID         string `json:"wa_id,omitempty"`
	OldUserID    string `json:"old_user_id,omitempty"`
	NewUserID    string `json:"new_user_id,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	ParentUserID string `json:"parent_user_id,omitempty"`
	Timestamp    int64  `json:"timestamp,omitempty"`
}

type TemplateStatusUpdateValue struct {
	MessageTemplateID       string `json:"message_template_id,omitempty"`
	MessageTemplateName     string `json:"message_template_name,omitempty"`
	MessageTemplateLanguage string `json:"message_template_language,omitempty"`
	Event                   string `json:"event,omitempty"`
	Reason                  string `json:"reason,omitempty"`
	Category                string `json:"category,omitempty"`
	QualityScore            any    `json:"quality_score,omitempty"`
}

type TemplateCategoryUpdateValue struct {
	MessageTemplateID       string `json:"message_template_id,omitempty"`
	MessageTemplateName     string `json:"message_template_name,omitempty"`
	MessageTemplateLanguage string `json:"message_template_language,omitempty"`
	Category                string `json:"category,omitempty"`
}

type PhoneQualityUpdateValue struct {
	DisplayPhoneNumber string `json:"display_phone_number,omitempty"`
	QualityRating      string `json:"quality_rating,omitempty"`
	Event              string `json:"event,omitempty"`
	CurrentLimit       string `json:"current_limit,omitempty"`
}

type UserPreferencesValue struct {
	UserID          string `json:"user_id,omitempty"`
	WAID            string `json:"wa_id,omitempty"`
	MarketingOptOut *bool  `json:"marketing_opt_out,omitempty"`
}

type AccountAlertValue struct {
	AlertSeverity  string `json:"alert_severity,omitempty"`
	AlertType      string `json:"alert_type,omitempty"`
	AlertMessage   string `json:"alert_message,omitempty"`
	EntityID       string `json:"entity_id,omitempty"`
	EntityType     string `json:"entity_type,omitempty"`
	AlertStatus    string `json:"alert_status,omitempty"`
	AlertTimestamp string `json:"alert_timestamp,omitempty"`
}

type AccountReviewUpdateValue struct {
	AccountReviewStatus string `json:"account_review_status,omitempty"`
	AccountID           string `json:"account_id,omitempty"`
}

type BusinessCapabilityUpdateValue struct {
	BusinessID string `json:"business_id,omitempty"`
	Capability string `json:"capability,omitempty"`
	Status     string `json:"status,omitempty"`
}

type SecurityValue struct {
	Event       string `json:"event,omitempty"`
	Description string `json:"description,omitempty"`
}

type BusinessUsernameUpdateValue struct {
	BusinessID string `json:"business_id,omitempty"`
	Username   string `json:"username,omitempty"`
	Status     string `json:"status,omitempty"`
}

type PaymentConfigurationUpdateValue struct {
	WABAID    string `json:"waba_id,omitempty"`
	Status    string `json:"status,omitempty"`
	Message   string `json:"message,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type MessageEchoValue struct {
	MessagingProduct string `json:"messaging_product,omitempty"`
	Metadata         struct {
		DisplayPhoneNumber string `json:"display_phone_number,omitempty"`
		PhoneNumberID      string `json:"phone_number_id,omitempty"`
	} `json:"metadata"`
	Messages []inboundMessage `json:"messages,omitempty"`
}

type HistoryValue struct {
	MessagingProduct string `json:"messaging_product,omitempty"`
	Metadata         struct {
		DisplayPhoneNumber string `json:"display_phone_number,omitempty"`
		PhoneNumberID      string `json:"phone_number_id,omitempty"`
	} `json:"metadata"`
}

// knownWebhookFields is the coverage table. Fields that are deliberately
// deferred to the Groups/Calling/Flows programs are still known so they are
// stored as account events rather than reported as unknown.
var knownWebhookFields = map[string]struct{}{
	"messages": {}, "user_id_update": {}, "customer_identity_changed": {},
	"customer_number_changed": {}, "message_template_status_update": {},
	"message_template_quality_update": {}, "template_category_update": {},
	"message_template_components_update": {}, "phone_number_quality_update": {},
	"account_alerts": {}, "account_review_update": {}, "business_capability_update": {},
	"security": {}, "business_username_updates": {}, "user_preferences": {},
	"payment_configuration_update": {}, "partner_solutions": {}, "history": {},
	"smb_message_echoes": {}, "calls": {},
}

func isKnownWebhookField(field string) bool {
	_, ok := knownWebhookFields[field]
	return ok
}
