package cloud

import "time"

// SendResponse is the relevant part of Graph's message response.
type SendResponse struct {
	MessagingProduct string `json:"messaging_product"`
	Contacts         []struct {
		Input        string `json:"input"`
		WAID         string `json:"wa_id"`
		UserID       string `json:"user_id"`
		ParentUserID string `json:"parent_user_id"`
	} `json:"contacts"`
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

func (r SendResponse) MessageID() string {
	if len(r.Messages) > 0 {
		return r.Messages[0].ID
	}
	return ""
}

type PhoneInfo struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	VerifiedName       string `json:"verified_name"`
	QualityRating      string `json:"quality_rating"`
	Status             string `json:"status"`
}

type MediaInfo struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	MimeType  string `json:"mime_type"`
	SHA256    string `json:"sha256"`
	FileSize  int64  `json:"file_size"`
	Messaging string `json:"messaging_product"`
}

type TemplateInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Language       string `json:"language"`
	Status         string `json:"status"`
	Category       string `json:"category"`
	Components     any    `json:"components"`
	RejectedReason string `json:"rejected_reason"`
	QualityScore   any    `json:"quality_score"`
}

// CredentialsInput is the write-only credential payload accepted by the API.
type CredentialsInput struct {
	AppID              string     `json:"appId"`
	WABAID             string     `json:"wabaId"`
	PhoneNumberID      string     `json:"phoneNumberId"`
	BusinessID         string     `json:"businessId"`
	AccessToken        string     `json:"accessToken"`
	DisplayPhoneNumber string     `json:"displayPhoneNumber"`
	EnforceWindow      *bool      `json:"enforceWindow,omitempty"`
	TokenExpiresAt     *time.Time `json:"tokenExpiresAt,omitempty"`
}

// CloudStatus is the non-secret status returned to the manager/API.
type CloudStatus struct {
	DisplayPhoneNumber string     `json:"displayPhoneNumber,omitempty"`
	VerifiedName       string     `json:"verifiedName,omitempty"`
	LastVerifiedAt     *time.Time `json:"lastVerifiedAt,omitempty"`
	LastWebhookAt      *time.Time `json:"lastWebhookAt,omitempty"`
	Subscribed         bool       `json:"subscribed"`
	Registered         bool       `json:"registered"`
	RegistrationStatus string     `json:"registrationStatus,omitempty"`
	QualityRating      string     `json:"qualityRating,omitempty"`
	MessagingLimit     string     `json:"messagingLimit,omitempty"`
	WindowHours        int        `json:"windowHours"`
	Capabilities       any        `json:"capabilities"`
	CapabilityNotes    any        `json:"capabilityNotes,omitempty"`
}

// Diagnostics is the doctor response.
type Diagnostics struct {
	TokenValid       bool       `json:"tokenValid"`
	AppSecretSet     bool       `json:"appSecretSet"`
	PhoneRegistered  bool       `json:"phoneRegistered"`
	Subscribed       bool       `json:"subscribed"`
	DisplayPhone     string     `json:"displayPhoneNumber,omitempty"`
	VerifiedName     string     `json:"verifiedName,omitempty"`
	QualityRating    string     `json:"qualityRating,omitempty"`
	MessagingLimit   string     `json:"messagingLimit,omitempty"`
	LastGraphError   string     `json:"lastGraphError,omitempty"`
	LastWebhookAt    *time.Time `json:"lastWebhookAt,omitempty"`
	EffectiveRPS     float64    `json:"effectiveRPS,omitempty"`
	TemplateApproved int        `json:"templatesApproved"`
	TemplatePending  int        `json:"templatesPending"`
	TemplateRejected int        `json:"templatesRejected"`
}
