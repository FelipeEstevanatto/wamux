package cloud

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cloud_model "github.com/felipeestevanatto/wamux/pkg/cloud/model"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
)

var (
	ErrInvalidSignature = errors.New("cloud: invalid webhook signature")
	ErrWebhookTooLarge  = errors.New("cloud: webhook body too large")
	ErrUnknownApp       = errors.New("cloud: webhook app is not configured")
)

type webhookEnvelope struct {
	Object string         `json:"object"`
	Entry  []webhookEntry `json:"entry"`
}

type webhookEntry struct {
	ID      string          `json:"id"`
	Time    int64           `json:"time"`
	Changes []webhookChange `json:"changes"`
}

type webhookChange struct {
	Field string          `json:"field"`
	Value json.RawMessage `json:"value"`
}

type messageValue struct {
	MessagingProduct string `json:"messaging_product"`
	Metadata         struct {
		DisplayPhoneNumber string `json:"display_phone_number"`
		PhoneNumberID      string `json:"phone_number_id"`
	} `json:"metadata"`
	Contacts []webhookContact `json:"contacts"`
	Messages []inboundMessage `json:"messages"`
	Statuses []inboundStatus  `json:"statuses"`
	Errors   []webhookError   `json:"errors"`
}

type webhookContact struct {
	Profile struct {
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"profile"`
	WAID         string `json:"wa_id"`
	UserID       string `json:"user_id"`
	ParentUserID string `json:"parent_user_id"`
}

type inboundMessage struct {
	From       string `json:"from"`
	FromUserID string `json:"from_user_id"`
	ID         string `json:"id"`
	Timestamp  string `json:"timestamp"`
	Type       string `json:"type"`

	Text *struct {
		Body string `json:"body"`
	} `json:"text"`
	Image    *mediaObject `json:"image"`
	Video    *mediaObject `json:"video"`
	Audio    *mediaObject `json:"audio"`
	Document *mediaObject `json:"document"`
	Sticker  *mediaObject `json:"sticker"`
	Reaction *struct {
		MessageID string `json:"message_id"`
		Emoji     string `json:"emoji"`
	} `json:"reaction"`
	Button *struct {
		Payload string `json:"payload"`
		Text    string `json:"text"`
	} `json:"button"`
	Interactive *struct {
		Type        string `json:"type"`
		ButtonReply *struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"button_reply"`
		ListReply *struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"list_reply"`
	} `json:"interactive"`
	Location *struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Name      string  `json:"name"`
		Address   string  `json:"address"`
	} `json:"location"`
}

type mediaObject struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	Filename string `json:"filename"`
	Caption  string `json:"caption"`
	FileSize int64  `json:"file_size"`
	Voice    bool   `json:"voice"`
}

type inboundStatus struct {
	ID              string         `json:"id"`
	Status          string         `json:"status"`
	Timestamp       string         `json:"timestamp"`
	RecipientID     string         `json:"recipient_id"`
	RecipientUserID string         `json:"recipient_user_id"`
	Errors          []webhookError `json:"errors"`
}

type webhookError struct {
	Code    int    `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
	Details string `json:"error_data"`
	Type    string `json:"type"`
	Subcode int    `json:"error_subcode"`
	TraceID string `json:"fbtrace_id"`
}

// VerifyToken reports whether a Meta GET challenge token is configured.
func (s *Service) VerifyToken(token string) bool {
	return s.verifyTokenMatches(token)
}

// ReceiveWebhook verifies the raw signature and processes a webhook payload.
func (s *Service) ReceiveWebhook(raw []byte, signature, appID string) error {
	if err := s.verifyWebhookSignature(raw, signature, appID); err != nil {
		return err
	}
	var env webhookEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("cloud: invalid webhook JSON: %w", err)
	}
	if env.Object != "whatsapp_business_account" {
		return nil
	}
	for _, entry := range env.Entry {
		for _, change := range entry.Changes {
			if err := s.processChange(entry, change); err != nil {
				// A malformed single change is logged and skipped; only
				// infrastructure failures propagate so Meta retries.
				if errors.Is(err, errSkipChange) {
					continue
				}
				return err
			}
		}
	}
	return nil
}

var errSkipChange = errors.New("cloud: skip webhook change")

func (s *Service) processChange(entry webhookEntry, change webhookChange) error {
	switch change.Field {
	case "messages":
		var value messageValue
		if err := json.Unmarshal(change.Value, &value); err != nil {
			return fmt.Errorf("%w: %v", errSkipChange, err)
		}
		creds, err := s.repo.GetCredentialsByPhone(value.Metadata.PhoneNumberID)
		if err != nil {
			return fmt.Errorf("%w: unknown phone number id", errSkipChange)
		}
		inst, err := s.instanceFor(creds.InstanceID)
		if err != nil {
			return fmt.Errorf("%w: instance not found", errSkipChange)
		}
		for _, msg := range value.Messages {
			if err := s.processInboundMessage(inst, creds, value, msg); err != nil {
				if errors.Is(err, errSkipChange) {
					continue
				}
				return err
			}
		}
		for _, status := range value.Statuses {
			if err := s.processStatus(inst, creds, status); err != nil {
				if errors.Is(err, errSkipChange) {
					continue
				}
				return err
			}
		}
		return nil
	case "message_template_status_update", "message_template_quality_update",
		"template_category_update", "message_template_components_update",
		"phone_number_quality_update", "account_alerts", "account_review_update",
		"business_capability_update", "security", "business_username_updates",
		"user_preferences":
		return s.processAccountChange(entry, change)
	default:
		// Unknown fields are acknowledged, not retried forever.
		return nil
	}
}

func (s *Service) instanceFor(instanceID string) (*instance_model.Instance, error) {
	if s.instances != nil {
		if inst, err := s.instances.GetInstanceByID(instanceID); err == nil && inst != nil {
			return inst, nil
		}
	}
	return &instance_model.Instance{Id: instanceID, Integration: "WHATSAPP-BUSINESS"}, nil
}

func (s *Service) processInboundMessage(inst *instance_model.Instance, creds *cloud_model.Credentials, value messageValue, msg inboundMessage) error {
	if strings.TrimSpace(msg.ID) == "" || strings.TrimSpace(msg.Type) == "" {
		return nil
	}
	contact := s.contactFrom(value, msg)
	contactID, err := s.resolveContact(inst.Id, contact, msg.From, msg.FromUserID)
	if err != nil {
		return err
	}
	key := eventKey(inst.Id, "message", msg.ID, "")
	claimed, err := s.repo.ClaimWebhookEvent(&cloud_model.WebhookEvent{
		EventKey: key, InstanceID: inst.Id, EventType: "message",
	})
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	ok := false
	defer func() {
		if !ok {
			_ = s.repo.ReleaseWebhookEvent(key, "processing failed")
		}
	}()

	ts := parseCloudTimestamp(msg.Timestamp)
	chatJID := phoneJID(msg.From)
	if chatJID == "" {
		chatJID = contactID
	}
	messageType, text, media := inboundContent(msg)
	row := message_model.Message{
		MessageID: msg.ID, InstanceId: inst.Id,
		Timestamp: ts.Format("2006-01-02 15:04:05"), Status: "Received",
		Source: sourceFor(msg.From, contactID), ChatJid: chatJID, SenderJid: chatJID,
		MessageType: messageType, TextContent: text, IsFromMe: false, Channel: "cloud",
		ContactID: contactID, SenderUserID: msg.FromUserID,
	}
	if media != nil {
		row.MediaMimetype = media.MimeType
		row.FileName = media.Filename
		row.FileSize = media.FileSize
	}
	if s.messages != nil {
		if err := s.messages.InsertMessage(row); err != nil {
			return err
		}
	}
	expires := ts.Add(s.windowDuration())
	if err := s.repo.UpsertWindow(&cloud_model.ChatWindow{
		InstanceID: inst.Id, ContactID: contactID, ChatJID: chatJID,
		LastInboundAt: ts, ExpiresAt: expires, LastMessageID: msg.ID,
	}); err != nil {
		return err
	}
	if media != nil && media.ID != "" {
		if err := s.repo.CreateMediaJob(&cloud_model.MediaJob{
			InstanceID: inst.Id, MessageID: msg.ID, MediaID: media.ID,
			MimeType: media.MimeType, Filename: safeMediaFilename(media.Filename, msg.ID),
			DeclaredSize: media.FileSize, Status: "pending", NextAttemptAt: time.Now(),
		}); err != nil {
			return err
		}
	}
	s.publishMessage(inst, value, msg, contactID, contact, ts, messageType, text, media)
	if s.typebot != nil && text != "" {
		go s.typebot.ProcessMessage(inst, chatJID, contact.Profile.Name, text, false)
	}
	s.markWebhook(inst.Id)
	_ = s.repo.MarkWebhookProcessed(key)
	ok = true
	return nil
}

func (s *Service) processStatus(inst *instance_model.Instance, creds *cloud_model.Credentials, status inboundStatus) error {
	if status.ID == "" || status.Status == "" {
		return nil
	}
	key := eventKey(inst.Id, "status", status.ID, status.Status+":"+status.Timestamp)
	claimed, err := s.repo.ClaimWebhookEvent(&cloud_model.WebhookEvent{
		EventKey: key, InstanceID: inst.Id, EventType: "status",
	})
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	ok := false
	defer func() {
		if !ok {
			_ = s.repo.ReleaseWebhookEvent(key, "status processing failed")
		}
	}()
	ts := parseCloudTimestamp(status.Timestamp)
	apiStatus, errCode, errTitle, errMsg := mapStatus(status)
	added, err := s.repo.AddStatusEvent(&cloud_model.StatusEvent{
		InstanceID: inst.Id, MessageID: status.ID, Status: status.Status,
		RecipientUserID: status.RecipientUserID, RecipientID: status.RecipientID,
		ErrorCode: errCode, ErrorTitle: errTitle, ErrorMessage: errMsg,
		OccurredAt: ts,
	})
	if err != nil {
		return err
	}
	if added && s.messages != nil {
		// Only update an existing row; a status for a message this process did
		// not persist must not create an orphan with an empty chat.
		if _, getErr := s.messages.GetMessageByIDForInstance(inst.Id, status.ID); getErr == nil {
			_ = s.messages.InsertMessage(message_model.Message{
				MessageID: status.ID, InstanceId: inst.Id, Status: apiStatus,
				Channel: "cloud", RecipientUserID: status.RecipientUserID,
			})
		}
	}
	if added {
		s.publishReceipt(inst, status, apiStatus, errCode, errMsg)
	}
	s.markWebhook(inst.Id)
	_ = s.repo.MarkWebhookProcessed(key)
	ok = true
	return nil
}

func (s *Service) processAccountChange(entry webhookEntry, change webhookChange) error {
	creds, err := s.repo.GetCredentialsByWABA(entry.ID)
	if err != nil {
		return fmt.Errorf("%w: unknown WABA", errSkipChange)
	}
	inst := &instance_model.Instance{Id: creds.InstanceID, Integration: "WHATSAPP-BUSINESS"}
	payload := string(change.Value)
	event := &cloud_model.AccountEvent{
		InstanceID: inst.Id, Field: change.Field, EventType: change.Field,
		Payload: payload,
	}
	_ = s.repo.AddAccountEvent(event)
	if strings.HasPrefix(change.Field, "message_template") || change.Field == "template_category_update" {
		s.applyTemplateUpdate(inst.Id, change.Value)
	}
	if change.Field == "user_preferences" {
		s.applyUserPreference(inst.Id, change.Value)
	}
	if change.Field == "phone_number_quality_update" {
		s.applyPhoneQuality(inst.Id, change.Value)
	}
	s.publishAccount(inst, change.Field, change.Value)
	s.markWebhook(inst.Id)
	return nil
}

func (s *Service) applyTemplateUpdate(instanceID string, raw json.RawMessage) {
	var v struct {
		MessageTemplateID       string `json:"message_template_id"`
		MessageTemplateName     string `json:"message_template_name"`
		MessageTemplateLanguage string `json:"message_template_language"`
		Event                   string `json:"event"`
		Reason                  string `json:"reason"`
		Category                string `json:"category"`
		QualityScore            any    `json:"quality_score"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return
	}
	status := v.Event
	quality, _ := json.Marshal(v.QualityScore)
	apply := func(t *cloud_model.Template) {
		if status != "" {
			t.Status = status
		}
		if v.Reason != "" {
			t.RejectedReason = v.Reason
		}
		if v.Category != "" {
			t.Category = v.Category
		}
		if len(quality) > 0 && string(quality) != "null" {
			t.QualityScore = string(quality)
		}
		_ = s.repo.UpsertTemplate(t)
	}
	if v.MessageTemplateID != "" {
		if t, err := s.repo.GetTemplateByID(instanceID, v.MessageTemplateID); err == nil {
			apply(t)
			return
		}
	}
	if v.MessageTemplateName != "" {
		if t, err := s.repo.GetTemplate(instanceID, v.MessageTemplateName, v.MessageTemplateLanguage); err == nil {
			apply(t)
		}
	}
}

func (s *Service) applyPhoneQuality(instanceID string, raw json.RawMessage) {
	var v struct {
		QualityRating string `json:"quality_rating"`
		DisplayPhone  string `json:"display_phone_number"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return
	}
	c, err := s.repo.GetCredentials(instanceID)
	if err != nil {
		return
	}
	if v.QualityRating != "" {
		c.QualityRating = v.QualityRating
	}
	_ = s.repo.SaveCredentials(c)
}

func (s *Service) applyUserPreference(instanceID string, raw json.RawMessage) {
	var v struct {
		UserID          string `json:"user_id"`
		WAID            string `json:"wa_id"`
		MarketingOptOut *bool  `json:"marketing_opt_out"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return
	}
	contactID := ""
	if v.UserID != "" {
		contactID = "bsuid:" + v.UserID
	} else if v.WAID != "" {
		contactID = "phone:" + digits(v.WAID)
	}
	if contactID == "" {
		return
	}
	out := false
	if v.MarketingOptOut != nil {
		out = *v.MarketingOptOut
	}
	now := time.Now()
	_ = s.repo.UpsertUserPreference(&cloud_model.UserPreference{
		InstanceID: instanceID, ContactID: contactID,
		MarketingOptOut: out, SourceEventAt: &now,
	})
}

func (s *Service) verifyWebhookSignature(raw []byte, signature, appID string) error {
	if strings.TrimSpace(signature) == "" {
		return ErrInvalidSignature
	}
	if appID != "" {
		secret, err := s.appSecret(appID)
		if err != nil {
			return ErrUnknownApp
		}
		if !verifyHMAC(secret, raw, signature) {
			return ErrInvalidSignature
		}
		return nil
	}
	var env webhookEnvelope
	_ = json.Unmarshal(raw, &env)
	var appIDs []string
	for _, entry := range env.Entry {
		for _, change := range entry.Changes {
			var value struct {
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
			}
			if json.Unmarshal(change.Value, &value) == nil && value.Metadata.PhoneNumberID != "" {
				if creds, err := s.repo.GetCredentialsByPhone(value.Metadata.PhoneNumberID); err == nil {
					appIDs = append(appIDs, creds.AppID)
				}
			}
		}
	}
	apps, err := s.repo.ListApps()
	if err != nil {
		return err
	}
	if len(appIDs) > 0 {
		for _, id := range appIDs {
			if secret, err := s.appSecret(id); err == nil && verifyHMAC(secret, raw, signature) {
				return nil
			}
		}
	}
	// Unknown WABA: a valid signature from a configured app is still accepted
	// so Meta gets 200 and does not retry an event we cannot route.
	for i, app := range apps {
		if i >= 10 {
			break
		}
		if secret, err := s.appSecret(app.AppID); err == nil && verifyHMAC(secret, raw, signature) {
			return nil
		}
	}
	return ErrInvalidSignature
}

func verifyHMAC(secret string, body []byte, header string) bool {
	raw, ok := strings.CutPrefix(header, "sha256=")
	if !ok {
		return false
	}
	expected, err := hex.DecodeString(raw)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(expected, mac.Sum(nil))
}

func eventKey(instanceID, kind, id, extra string) string {
	h := sha256.Sum256([]byte(instanceID + ":" + kind + ":" + id + ":" + extra))
	return hex.EncodeToString(h[:])
}

func parseCloudTimestamp(v string) time.Time {
	n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	if err != nil || n <= 0 {
		return time.Now()
	}
	return time.Unix(n, 0)
}

func (s *Service) windowDuration() time.Duration {
	hours := 24
	if s.cfg != nil && s.cfg.CloudAPIWindowHours > 0 {
		hours = s.cfg.CloudAPIWindowHours
	}
	return time.Duration(hours) * time.Hour
}

func (s *Service) contactFrom(value messageValue, msg inboundMessage) webhookContact {
	if len(value.Contacts) > 0 {
		return value.Contacts[0]
	}
	return webhookContact{WAID: msg.From, UserID: msg.FromUserID}
}

func (s *Service) resolveContact(instanceID string, c webhookContact, from, fromUserID string) (string, error) {
	bsuid := firstNonEmpty(c.UserID, fromUserID)
	waID := firstNonEmpty(c.WAID, from)
	if bsuid == "" && waID == "" {
		return "", nil
	}
	var existing *cloud_model.Contact
	if bsuid != "" {
		existing, _ = s.repo.FindContactByBSUID(instanceID, bsuid)
	}
	if existing == nil && waID != "" {
		existing, _ = s.repo.FindContactByWAID(instanceID, waID)
	}
	contactID := ""
	if existing != nil {
		contactID = existing.ContactID
	} else if bsuid != "" {
		contactID = "bsuid:" + bsuid
	} else {
		contactID = "phone:" + digits(waID)
	}
	row := &cloud_model.Contact{
		InstanceID: instanceID, ContactID: contactID, BSUID: bsuid,
		ParentBSUID: c.ParentUserID, WAID: digits(waID), Username: c.Profile.Username,
		DisplayName: c.Profile.Name,
	}
	if err := s.repo.UpsertContact(row); err != nil {
		return "", err
	}
	return contactID, nil
}

func inboundContent(msg inboundMessage) (string, string, *mediaObject) {
	switch msg.Type {
	case "text":
		if msg.Text != nil {
			return "text", msg.Text.Body, nil
		}
	case "image":
		return "image", caption(msg.Image), msg.Image
	case "video":
		return "video", caption(msg.Video), msg.Video
	case "audio":
		return "audio", "", msg.Audio
	case "document":
		return "document", caption(msg.Document), msg.Document
	case "sticker":
		return "sticker", "", msg.Sticker
	case "location":
		if msg.Location != nil {
			return "location", msg.Location.Name, nil
		}
	case "reaction":
		if msg.Reaction != nil {
			return "reaction", msg.Reaction.Emoji, nil
		}
	case "button":
		if msg.Button != nil {
			return "button_reply", msg.Button.Text, nil
		}
	case "interactive":
		if msg.Interactive != nil {
			if msg.Interactive.ButtonReply != nil {
				return "interactive_reply", msg.Interactive.ButtonReply.Title, nil
			}
			if msg.Interactive.ListReply != nil {
				return "interactive_reply", msg.Interactive.ListReply.Title, nil
			}
		}
	}
	return msg.Type, "", nil
}

func caption(m *mediaObject) string {
	if m == nil {
		return ""
	}
	return m.Caption
}

func sourceFor(phone, contactID string) string {
	if d := digits(phone); d != "" {
		return d
	}
	return contactID
}

func mapStatus(status inboundStatus) (apiStatus, code, title, message string) {
	normalized := strings.ToLower(strings.TrimSpace(status.Status))
	if normalized != "" {
		apiStatus = strings.ToUpper(normalized[:1]) + normalized[1:]
	}
	switch normalized {
	case "sent":
		apiStatus = "Sent"
	case "delivered":
		apiStatus = "Delivered"
	case "read":
		apiStatus = "Read"
	case "failed":
		apiStatus = "Failed"
	case "deleted":
		apiStatus = "Deleted"
	}
	if len(status.Errors) > 0 {
		e := status.Errors[0]
		code = strconv.Itoa(e.Code)
		title = e.Title
		message = firstNonEmpty(e.Message, e.Details)
	}
	return
}

func safeMediaFilename(name, messageID string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == "/" {
		return messageID
	}
	return name
}
