package cloud

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// APIError is the stable, public error shape returned by the Cloud layer. It
// never contains an access token or a raw Graph request body.
type APIError struct {
	Status     int            `json:"-"`
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Details    map[string]any `json:"details,omitempty"`
	RetryAfter int            `json:"-"`
}

func (e *APIError) Error() string { return e.Message }

// MapGraphError turns Meta's code/subcode into an actionable WaMux error.
func MapGraphError(err error) *APIError {
	var ge *GraphError
	if !errors.As(err, &ge) {
		return &APIError{Status: http.StatusServiceUnavailable, Code: "cloud_upstream_unavailable", Message: "the Cloud API request failed"}
	}
	msg := ge.UserMsg
	if msg == "" {
		msg = ge.Details
	}
	if msg == "" {
		msg = ge.Message
	}
	details := map[string]any{"metaCode": ge.Code, "metaSubcode": ge.Subcode}
	if ge.TraceID != "" {
		details["fbtraceId"] = ge.TraceID
	}
	switch ge.Code {
	case 190:
		return &APIError{Status: http.StatusBadGateway, Code: "cloud_auth_failed", Message: "the Meta access token is invalid or expired", Details: details}
	case 131009:
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "cloud_invalid_message_id", Message: "the quoted or reacted-to message id is invalid or has expired", Details: details}
	case 131026:
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "cloud_undeliverable", Message: "the message is undeliverable; the recipient may not be on WhatsApp or the client is too old", Details: details}
	case 131030:
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "cloud_recipient_not_allowed", Message: "the recipient is not in the test-number allowlist", Details: details}
	case 131031:
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "cloud_account_locked", Message: "the WhatsApp account is locked; check WhatsApp Manager", Details: details}
	case 131042:
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "cloud_payment_issue", Message: "the WhatsApp Business account has a payment issue", Details: details}
	case 131047:
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "cloud_window_expired", Message: "the 24-hour customer service window is closed; send an approved template instead", Details: details}
	case 131048:
		return &APIError{Status: http.StatusTooManyRequests, Code: "cloud_rate_limited", Message: "Meta rate limit reached; retry later", Details: details, RetryAfter: retryAfterFromGraph(ge)}
	case 131051:
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "unsupported_on_channel", Message: msg, Details: details}
	case 131056:
		return &APIError{Status: http.StatusTooManyRequests, Code: "cloud_pair_rate_limited", Message: "too many messages to this recipient; pause and retry later", Details: details}
	case 131062:
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "cloud_bsuid_unsupported", Message: "this message type does not support a BSUID recipient; use a phone number", Details: details}
	case 132000, 132001, 132005, 132007, 132012, 132015:
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "cloud_template_error", Message: "the template is missing, rejected, paused, or its parameters do not match", Details: details}
	case 133010:
		return &APIError{Status: http.StatusFailedDependency, Code: "cloud_phone_not_registered", Message: "the business phone number is not registered for Cloud API; register it with its six-digit PIN", Details: details}
	case 133016:
		return &APIError{Status: http.StatusTooManyRequests, Code: "cloud_registration_limit", Message: "registration is limited to 10 attempts per 72 hours; wait before retrying", Details: details}
	case 147005:
		return &APIError{Status: http.StatusUnprocessableEntity, Code: "cloud_username_transfer_required", Message: "the business username is in use on another number; use force_transfer or choose another name", Details: details}
	case 368:
		return &APIError{Status: http.StatusServiceUnavailable, Code: "cloud_temporarily_blocked", Message: "the business number is temporarily blocked by Meta policy", Details: details, RetryAfter: retryAfterFromGraph(ge)}
	case 80007, 130429:
		return &APIError{Status: http.StatusTooManyRequests, Code: "cloud_rate_limited", Message: "Meta messaging rate limit reached; retry later", Details: details, RetryAfter: retryAfterFromGraph(ge)}
	}
	if ge.StatusCode == http.StatusTooManyRequests {
		return &APIError{Status: http.StatusTooManyRequests, Code: "cloud_rate_limited", Message: "Meta rate limit reached; retry later", Details: details}
	}
	if ge.StatusCode >= 500 {
		return &APIError{Status: http.StatusServiceUnavailable, Code: "cloud_upstream_unavailable", Message: "Meta is temporarily unavailable", Details: details}
	}
	return &APIError{Status: http.StatusBadGateway, Code: "cloud_upstream_error", Message: msg, Details: details}
}

func retryAfterFromGraph(ge *GraphError) int {
	if ge != nil && ge.RetryAfter > 0 {
		return int(ge.RetryAfter / time.Second)
	}
	return 0
}

// IsWindowExpired reports the local or Meta-side 24-hour-window error.
func IsWindowExpired(err error) bool {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae.Code == "cloud_window_expired"
	}
	var ge *GraphError
	if errors.As(err, &ge) {
		return ge.Code == 131047
	}
	return false
}

// WindowExpiredError is returned by the local pre-check before a Graph call.
type WindowExpiredError struct {
	ContactID string
	ExpiresAt time.Time
}

func (e *WindowExpiredError) Error() string {
	return fmt.Sprintf("the 24-hour customer service window for %s closed at %s; send an approved template instead",
		e.ContactID, e.ExpiresAt.UTC().Format(time.RFC3339))
}

// IsAPIError checks a code without string matching in handlers.
func IsAPIError(err error, code string) bool {
	var ae *APIError
	return errors.As(err, &ae) && strings.EqualFold(ae.Code, code)
}
