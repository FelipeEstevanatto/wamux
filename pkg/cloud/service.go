package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/channel"
	cloud_model "github.com/felipeestevanatto/wamux/pkg/cloud/model"
	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	localmedia "github.com/felipeestevanatto/wamux/pkg/media"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
	"github.com/felipeestevanatto/wamux/pkg/ssrf"
	storage_interfaces "github.com/felipeestevanatto/wamux/pkg/storage/interfaces"
	tokencrypt "github.com/felipeestevanatto/wamux/pkg/tokencrypt"
)

// EventSink is implemented by the whatsmeow service. Cloud events use the same
// webhook/RabbitMQ/NATS/WebSocket producers as Baileys.
type EventSink interface {
	CallWebhook(instance *instance_model.Instance, queueName string, jsonData []byte)
	SendToGlobalQueues(event string, jsonData []byte, userId string)
}

// MediaResolver resolves the storage used for inbound media.
type MediaResolver interface {
	MediaStorageFor(instance *instance_model.Instance) (storage_interfaces.MediaStorage, string)
}

// TypebotSink dispatches inbound text to the configured chatbot.
type TypebotSink interface {
	ProcessMessage(instance *instance_model.Instance, remoteJid, pushName, content string, fromMe bool)
}

// InstanceLookup hydrates a full instance row for event routing.
type InstanceLookup interface {
	GetInstanceByID(instanceID string) (*instance_model.Instance, error)
	Update(instance *instance_model.Instance) error
}

// SetInstanceLookup wires the instance repository after construction.
func (s *Service) SetInstanceLookup(lookup InstanceLookup) { s.instances = lookup }

// SetTypebotSink wires the chatbot after the send router exists.
func (s *Service) SetTypebotSink(sink TypebotSink) { s.typebot = sink }

// Service owns Cloud credentials, sending, webhooks, templates and media jobs.
type Service struct {
	repo      *Repository
	client    *Client
	codec     *tokencrypt.Codec
	cfg       *config.Config
	logger    *logger_wrapper.LoggerManager
	messages  message_repository.MessageRepository
	events    EventSink
	media     MediaResolver
	typebot   TypebotSink
	instances InstanceLookup

	// mediaHTTP is guarded against SSRF; Graph media URLs are still validated.
	mediaHTTP *http.Client

	lastWebhookMu sync.RWMutex
	lastWebhook   map[string]time.Time
}

// NewService builds the Cloud service. A nil codec (no encryption key) makes
// credential writes fail explicitly rather than storing plaintext.
func NewService(
	repo *Repository,
	client *Client,
	cfg *config.Config,
	logger *logger_wrapper.LoggerManager,
	messages message_repository.MessageRepository,
	events EventSink,
	media MediaResolver,
	typebot TypebotSink,
) *Service {
	var codec *tokencrypt.Codec
	if cfg != nil && len(cfg.DataEncryptionKey) > 0 {
		if c, err := tokencrypt.New(string(cfg.DataEncryptionKey)); err == nil {
			codec = c
		}
	}
	return &Service{
		repo:        repo,
		client:      client,
		codec:       codec,
		cfg:         cfg,
		logger:      logger,
		messages:    messages,
		events:      events,
		media:       media,
		typebot:     typebot,
		mediaHTTP:   ssrf.Default().Client(120 * time.Second),
		lastWebhook: map[string]time.Time{},
	}
}

func (s *Service) Repository() *Repository { return s.repo }
func (s *Service) Client() *Client         { return s.client }

func (s *Service) requireCodec() error {
	if s.codec == nil {
		return errors.New("cloud: DATA_ENCRYPTION_KEY is required for Cloud API credentials")
	}
	return nil
}

// SaveApp stores/updates a Meta app. The secret and verify token are write-only.
func (s *Service) SaveApp(appID, appSecret, verifyToken, graphVersion string) error {
	if err := s.requireCodec(); err != nil {
		return err
	}
	appID = strings.TrimSpace(appID)
	if appID == "" || strings.TrimSpace(appSecret) == "" || strings.TrimSpace(verifyToken) == "" {
		return errors.New("cloud: appId, appSecret and verifyToken are required")
	}
	enc, err := s.codec.Encrypt(appSecret)
	if err != nil {
		return err
	}
	app := &cloud_model.App{
		AppID:           appID,
		AppSecretEnc:    enc,
		VerifyTokenHash: s.codec.Hash(verifyToken),
		GraphVersion:    strings.TrimSpace(graphVersion),
	}
	if existing, err := s.repo.GetApp(appID); err == nil {
		app.CreatedAt = existing.CreatedAt
	}
	if app.GraphVersion == "" {
		app.GraphVersion = s.graphVersion()
	}
	return s.repo.SaveApp(app)
}

func (s *Service) SeedConfigApp() error {
	if s.cfg == nil || strings.TrimSpace(s.cfg.CloudAPIAppID) == "" ||
		strings.TrimSpace(s.cfg.CloudAPIAppSecret) == "" || strings.TrimSpace(s.cfg.CloudAPIVerifyToken) == "" {
		return nil
	}
	return s.SaveApp(s.cfg.CloudAPIAppID, s.cfg.CloudAPIAppSecret, s.cfg.CloudAPIVerifyToken, s.graphVersion())
}

func (s *Service) graphVersion() string {
	if s.cfg != nil && strings.TrimSpace(s.cfg.CloudAPIGraphVersion) != "" {
		return strings.TrimSpace(s.cfg.CloudAPIGraphVersion)
	}
	return "v27.0"
}

// VerifyTokenForApp verifies a GET challenge against one app row.
func (s *Service) VerifyTokenForApp(appID, token string) bool {
	if s.codec == nil || strings.TrimSpace(token) == "" {
		return false
	}
	app, err := s.repo.GetApp(appID)
	return err == nil && app.VerifyTokenHash == s.codec.Hash(token)
}

func (s *Service) DeleteApp(appID string) error {
	creds, err := s.repo.ListCredentials()
	if err != nil {
		return err
	}
	for _, c := range creds {
		if c.AppID == appID {
			return errors.New("cloud: app is still used by an instance")
		}
	}
	return s.repo.DeleteApp(appID)
}

func (s *Service) AppSecretSet(appID string) bool {
	app, err := s.repo.GetApp(appID)
	return err == nil && app.AppSecretEnc != ""
}

func (s *Service) appSecret(appID string) (string, error) {
	app, err := s.repo.GetApp(appID)
	if err != nil {
		return "", fmt.Errorf("cloud: app %s not found", appID)
	}
	if s.codec == nil {
		return "", errors.New("cloud: no encryption key for app secret")
	}
	return s.codec.Decrypt(app.AppSecretEnc)
}

func (s *Service) verifyTokenMatches(token string) bool {
	if s.codec == nil || strings.TrimSpace(token) == "" {
		return false
	}
	_, err := s.repo.FindAppByVerifyHash(s.codec.Hash(token))
	return err == nil
}

// SaveCredentials encrypts and stores the per-instance Meta credentials.
func (s *Service) SaveCredentials(instanceID string, in CredentialsInput) error {
	if err := s.requireCodec(); err != nil {
		return err
	}
	old, _ := s.repo.GetCredentials(instanceID)
	appID := strings.TrimSpace(in.AppID)
	if appID == "" && old != nil {
		appID = old.AppID
	}
	if appID == "" {
		apps, err := s.repo.ListApps()
		if err != nil || len(apps) == 0 {
			return errors.New("cloud: no Meta app configured")
		}
		appID = apps[0].AppID
	}
	if _, err := s.repo.GetApp(appID); err != nil {
		return fmt.Errorf("cloud: app %s is not configured", appID)
	}
	enc := ""
	if strings.TrimSpace(in.AccessToken) != "" {
		var err error
		enc, err = s.codec.Encrypt(in.AccessToken)
		if err != nil {
			return err
		}
	} else if old != nil {
		enc = old.AccessTokenEnc
	} else {
		return errors.New("cloud: accessToken is required")
	}
	if strings.TrimSpace(in.WABAID) == "" && old != nil {
		in.WABAID = old.WABAID
	}
	if strings.TrimSpace(in.PhoneNumberID) == "" && old != nil {
		in.PhoneNumberID = old.PhoneNumberID
	}
	if strings.TrimSpace(in.BusinessID) == "" && old != nil {
		in.BusinessID = old.BusinessID
	}
	if in.TokenExpiresAt == nil && old != nil {
		in.TokenExpiresAt = old.TokenExpiresAt
	}
	enforce := true
	if in.EnforceWindow != nil {
		enforce = *in.EnforceWindow
	} else if old != nil {
		enforce = old.EnforceWindow
	}
	c := &cloud_model.Credentials{
		InstanceID:         instanceID,
		AppID:              appID,
		WABAID:             strings.TrimSpace(in.WABAID),
		PhoneNumberID:      strings.TrimSpace(in.PhoneNumberID),
		BusinessID:         strings.TrimSpace(in.BusinessID),
		DisplayPhoneNumber: strings.TrimSpace(in.DisplayPhoneNumber),
		AccessTokenEnc:     enc,
		TokenExpiresAt:     in.TokenExpiresAt,
		EnforceWindow:      enforce,
	}
	if old != nil {
		c.CreatedAt = old.CreatedAt
		c.DisplayPhoneNumber = firstNonEmpty(c.DisplayPhoneNumber, old.DisplayPhoneNumber)
		c.VerifiedName = old.VerifiedName
		c.LastVerifiedAt = old.LastVerifiedAt
		c.LastVerifyError = old.LastVerifyError
		c.Registered = old.Registered
		c.RegisteredAt = old.RegisteredAt
		c.RegistrationStatus = old.RegistrationStatus
		c.LastRegistrationErr = old.LastRegistrationErr
		c.QualityRating = old.QualityRating
		c.MessagingLimit = old.MessagingLimit
	}
	if c.WABAID == "" || c.PhoneNumberID == "" || strings.TrimSpace(in.AccessToken) == "" {
		return errors.New("cloud: wabaId, phoneNumberId and accessToken are required")
	}
	return s.repo.SaveCredentials(c)
}

func (s *Service) credentialsToken(instanceID string) (*cloud_model.Credentials, string, error) {
	c, err := s.repo.GetCredentials(instanceID)
	if err != nil {
		return nil, "", errors.New("cloud: credentials are missing")
	}
	if s.codec == nil {
		return nil, "", errors.New("cloud: no encryption key for credentials")
	}
	token, err := s.codec.Decrypt(c.AccessTokenEnc)
	if err != nil {
		return nil, "", errors.New("cloud: could not decrypt the Meta access token")
	}
	return c, token, nil
}

// Connect verifies the token/phone and subscribes the app. It returns the
// non-secret status; the instance service persists Connected.
func (s *Service) Connect(instance *instance_model.Instance) (*CloudStatus, error) {
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return nil, err
	}
	info, err := s.client.VerifyPhone(context.Background(), token, c.PhoneNumberID)
	if err != nil {
		c.LastVerifyError = publicError(err)
		_ = s.repo.SaveCredentials(c)
		return nil, MapGraphError(err)
	}
	if err := s.client.SubscribeApp(context.Background(), token, c.WABAID); err != nil {
		c.LastVerifyError = publicError(err)
		_ = s.repo.SaveCredentials(c)
		return nil, MapGraphError(err)
	}
	now := time.Now()
	c.DisplayPhoneNumber = info.DisplayPhoneNumber
	c.VerifiedName = info.VerifiedName
	c.QualityRating = info.QualityRating
	c.MessagingLimit = info.Status
	c.LastVerifiedAt = &now
	c.LastVerifyError = ""
	_ = s.repo.SaveCredentials(c)
	instance.Connected = true
	instance.AutoReconnect = true
	if s.instances != nil {
		_ = s.instances.Update(instance)
	}
	if s.events != nil {
		s.emit(instance, "connected", map[string]any{
			"event": "Connected", "instanceId": instance.Id, "instanceName": instance.Name,
			"channel": "cloud", "data": map[string]any{"connected": true, "registered": c.Registered},
		})
	}
	return s.statusFrom(c), nil
}

// Register registers the phone number with its six-digit PIN. The PIN is never
// persisted.
func (s *Service) Register(instance *instance_model.Instance, pin string) (*CloudStatus, error) {
	if len(pin) != 6 || !regexp.MustCompile(`^[0-9]{6}$`).MatchString(pin) {
		return nil, &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "pin must be exactly six digits"}
	}
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return nil, err
	}
	if err := s.client.RegisterPhone(context.Background(), token, c.PhoneNumberID, pin); err != nil {
		apiErr := MapGraphError(err)
		c.LastRegistrationErr = apiErr.Message
		_ = s.repo.SaveCredentials(c)
		return nil, apiErr
	}
	now := time.Now()
	c.Registered = true
	c.RegisteredAt = &now
	c.RegistrationStatus = "registered"
	c.LastRegistrationErr = ""
	_ = s.repo.SaveCredentials(c)
	return s.statusFrom(c), nil
}

// CredentialSummary is the safe write-only credential view.
func (s *Service) CredentialSummary(instance *instance_model.Instance) map[string]any {
	c, err := s.repo.GetCredentials(instance.Id)
	if err != nil {
		return map[string]any{"configured": false}
	}
	return map[string]any{
		"configured": true, "appId": c.AppID, "wabaId": c.WABAID,
		"phoneNumberId": c.PhoneNumberID, "businessId": c.BusinessID,
		"displayPhoneNumber": c.DisplayPhoneNumber, "verifiedName": c.VerifiedName,
		"accessTokenSet": c.AccessTokenEnc != "", "enforceWindow": c.EnforceWindow,
		"registered": c.Registered, "registrationStatus": c.RegistrationStatus,
		"lastVerifiedAt": c.LastVerifiedAt, "qualityRating": c.QualityRating,
		"messagingLimit": c.MessagingLimit,
	}
}

func (s *Service) Status(instance *instance_model.Instance) (*CloudStatus, error) {
	c, err := s.repo.GetCredentials(instance.Id)
	if err != nil {
		return nil, err
	}
	return s.statusFrom(c), nil
}

func (s *Service) statusFrom(c *cloud_model.Credentials) *CloudStatus {
	hours := 24
	if s.cfg != nil && s.cfg.CloudAPIWindowHours > 0 {
		hours = s.cfg.CloudAPIWindowHours
	}
	caps := channel.CapabilitiesFor(channel.KindCloud)
	notes := map[string]string{}
	for _, name := range []string{"groups", "calls", "flows", "history"} {
		if note := channel.CapabilityNote(channel.KindCloud, name); note != "" {
			notes[name] = note
		}
	}
	var lastWebhook *time.Time
	s.lastWebhookMu.RLock()
	if t, ok := s.lastWebhook[c.InstanceID]; ok {
		lastWebhook = &t
	}
	s.lastWebhookMu.RUnlock()
	return &CloudStatus{
		DisplayPhoneNumber: c.DisplayPhoneNumber,
		VerifiedName:       c.VerifiedName,
		LastVerifiedAt:     c.LastVerifiedAt,
		LastWebhookAt:      lastWebhook,
		Subscribed:         c.LastVerifiedAt != nil,
		Registered:         c.Registered,
		RegistrationStatus: c.RegistrationStatus,
		QualityRating:      c.QualityRating,
		MessagingLimit:     c.MessagingLimit,
		WindowHours:        hours,
		Capabilities:       caps,
		CapabilityNotes:    notes,
	}
}

// Diagnostics checks the current token/registration/subscription and returns
// non-secret health data. It is explicitly allowed to make one Graph call.
func (s *Service) Diagnostics(instance *instance_model.Instance) (*Diagnostics, error) {
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return &Diagnostics{TokenValid: false, LastGraphError: err.Error()}, nil
	}
	out := &Diagnostics{
		PhoneRegistered: c.Registered,
		Subscribed:      c.LastVerifiedAt != nil,
		DisplayPhone:    c.DisplayPhoneNumber,
		VerifiedName:    c.VerifiedName,
		QualityRating:   c.QualityRating,
		MessagingLimit:  c.MessagingLimit,
		EffectiveRPS:    s.client.EffectiveRPS(),
		LastWebhookAt:   s.LastWebhookAt(instance.Id),
	}
	templates, _ := s.repo.ListTemplates(instance.Id)
	for _, t := range templates {
		switch strings.ToUpper(t.Status) {
		case "APPROVED", "ACTIVE":
			out.TemplateApproved++
		case "PENDING":
			out.TemplatePending++
		case "REJECTED", "DISABLED", "PAUSED":
			out.TemplateRejected++
		}
	}
	info, err := s.client.VerifyPhone(context.Background(), token, c.PhoneNumberID)
	if err != nil {
		out.TokenValid = false
		out.LastGraphError = publicError(err)
		return out, nil
	}
	out.TokenValid = true
	out.DisplayPhone = info.DisplayPhoneNumber
	out.VerifiedName = info.VerifiedName
	out.QualityRating = info.QualityRating
	if info.Status != "" {
		out.MessagingLimit = info.Status
	}
	if app, aerr := s.repo.GetApp(c.AppID); aerr == nil {
		out.AppSecretSet = app.AppSecretEnc != ""
	}
	return out, nil
}

func (s *Service) Delete(instanceID string) error {
	return s.repo.DeleteInstanceData(instanceID)
}

// --- sending ---------------------------------------------------------------

// SendText sends a Cloud text message.
func (s *Service) SendText(ctx context.Context, instance *instance_model.Instance, msg channel.TextMessage) (channel.Result, error) {
	if err := validateRecipientType(msg.RecipientType); err != nil {
		return channel.Result{}, err
	}
	if len([]rune(msg.Body)) > 4096 {
		return channel.Result{}, &APIError{Status: http.StatusUnprocessableEntity, Code: "invalid_request", Message: "Cloud text messages are limited to 4096 characters"}
	}
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return channel.Result{}, err
	}
	to, recipient, err := target(msg.To, msg.Recipient)
	if err != nil {
		return channel.Result{}, err
	}
	if err := s.ensureWindow(instance, c, msg.Recipient, msg.To, false); err != nil {
		return channel.Result{}, err
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    recipientType(msg.RecipientType),
		"type":              "text",
		"text":              map[string]any{"body": msg.Body, "preview_url": msg.PreviewURL},
	}
	addTarget(payload, to, recipient)
	addReply(payload, msg.ReplyTo)
	addOperation(payload, msg.OperationID)
	resp, err := s.client.SendMessage(ctx, token, c.PhoneNumberID, payload)
	if err != nil {
		return channel.Result{}, s.sendError(err)
	}
	return s.recordOutbound(instance, c, resp, to, recipient, "text", msg.Body, "", "", "", msg.OperationID)
}

// SendMedia sends a Cloud media message, uploading bytes first when needed.
func (s *Service) SendMedia(ctx context.Context, instance *instance_model.Instance, msg channel.MediaMessage) (channel.Result, error) {
	if err := validateRecipientType(msg.RecipientType); err != nil {
		return channel.Result{}, err
	}
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return channel.Result{}, err
	}
	to, recipient, err := target(msg.To, msg.Recipient)
	if err != nil {
		return channel.Result{}, err
	}
	if err := s.ensureWindow(instance, c, msg.Recipient, msg.To, false); err != nil {
		return channel.Result{}, err
	}
	kind := strings.ToLower(msg.Kind)
	if kind == "gif" || kind == "ptv" {
		kind = "video"
	}
	if !validMediaKind(kind) {
		return channel.Result{}, &APIError{Status: http.StatusUnprocessableEntity, Code: "unsupported_on_channel", Message: "media type is not supported by Cloud API"}
	}
	data := msg.Data
	mime := msg.MIME
	filename := msg.Filename
	if len(data) == 0 {
		data, mime, err = s.downloadForUpload(ctx, msg.URL)
		if err != nil {
			return channel.Result{}, err
		}
	}
	if len(data) == 0 {
		return channel.Result{}, &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "media data or url is required"}
	}
	if s.cfg != nil && s.cfg.CloudAPIMaxMediaBytes > 0 && int64(len(data)) > s.cfg.CloudAPIMaxMediaBytes {
		return channel.Result{}, &APIError{Status: http.StatusRequestEntityTooLarge, Code: "media_too_large", Message: "media exceeds the Cloud API limit"}
	}
	// Meta's per-type limits are smaller than the global inbound cap.
	limit := int64(100 << 20)
	switch kind {
	case "image":
		limit = 5 << 20
	case "video", "audio":
		limit = 16 << 20
	case "sticker":
		limit = 500 << 10
	}
	if int64(len(data)) > limit {
		return channel.Result{}, &APIError{Status: http.StatusRequestEntityTooLarge, Code: "media_too_large", Message: "media exceeds the Cloud API limit for " + kind}
	}
	if filename == "" {
		filename = "media"
	}
	if strings.TrimSpace(mime) == "" {
		mime = http.DetectContentType(data)
	}
	mediaID, err := s.client.UploadMedia(ctx, token, c.PhoneNumberID, data, mime, filename)
	if err != nil {
		return channel.Result{}, s.sendError(err)
	}
	body := map[string]any{"id": mediaID}
	if msg.Caption != "" && kind != "audio" {
		body["caption"] = msg.Caption
	}
	if kind == "document" && msg.Filename != "" {
		body["filename"] = msg.Filename
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    recipientType(msg.RecipientType),
		"type":              kind,
		kind:                body,
	}
	addTarget(payload, to, recipient)
	addReply(payload, msg.ReplyTo)
	addOperation(payload, msg.OperationID)
	resp, err := s.client.SendMessage(ctx, token, c.PhoneNumberID, payload)
	if err != nil {
		return channel.Result{}, s.sendError(err)
	}
	mediaURL := ""
	if s.cfg != nil && s.cfg.MediaLocalStore && len(data) > 0 && resp.MessageID() != "" {
		if err := localmedia.Save(instance.Id, resp.MessageID(), data); err == nil {
			mediaURL = localmedia.URLPath(resp.MessageID())
		}
	}
	return s.recordOutbound(instance, c, resp, to, recipient, kind, msg.Caption, mediaURL, mime, filename, msg.OperationID)
}

// SendTemplate sends an approved template.
func (s *Service) SendTemplate(ctx context.Context, instance *instance_model.Instance, msg channel.TemplateMessage) (channel.Result, error) {
	if err := validateRecipientType(msg.RecipientType); err != nil {
		return channel.Result{}, err
	}
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return channel.Result{}, err
	}
	to, recipient, err := target(msg.To, msg.Recipient)
	if err != nil {
		return channel.Result{}, err
	}
	tpl := map[string]any{
		"name":     msg.Name,
		"language": map[string]any{"code": msg.Language},
	}
	if msg.Components != nil {
		tpl["components"] = msg.Components
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    recipientType(msg.RecipientType),
		"type":              "template",
		"template":          tpl,
	}
	addTarget(payload, to, recipient)
	addOperation(payload, msg.OperationID)
	resp, err := s.client.SendMessage(ctx, token, c.PhoneNumberID, payload)
	if err != nil {
		return channel.Result{}, s.sendError(err)
	}
	return s.recordOutbound(instance, c, resp, to, recipient, "template", msg.Name, "", "", "", msg.OperationID)
}

// SendLocation sends a Cloud location message.
func (s *Service) SendLocation(ctx context.Context, instance *instance_model.Instance, msg channel.LocationMessage) (channel.Result, error) {
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return channel.Result{}, err
	}
	to, recipient, err := target(msg.To, msg.Recipient)
	if err != nil {
		return channel.Result{}, err
	}
	if err := s.ensureWindow(instance, c, msg.Recipient, msg.To, false); err != nil {
		return channel.Result{}, err
	}
	body := map[string]any{"latitude": msg.Latitude, "longitude": msg.Longitude}
	if msg.Name != "" {
		body["name"] = msg.Name
	}
	if msg.Address != "" {
		body["address"] = msg.Address
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"type":              "location",
		"location":          body,
	}
	addTarget(payload, to, recipient)
	addReply(payload, msg.ReplyTo)
	resp, err := s.client.SendMessage(ctx, token, c.PhoneNumberID, payload)
	if err != nil {
		return channel.Result{}, s.sendError(err)
	}
	return s.recordOutbound(instance, c, resp, to, recipient, "location", msg.Name, "", "", "", "")
}

// SendContact sends one or more Cloud contacts.
func (s *Service) SendContact(ctx context.Context, instance *instance_model.Instance, msg channel.ContactMessage) (channel.Result, error) {
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return channel.Result{}, err
	}
	to, recipient, err := target(msg.To, msg.Recipient)
	if err != nil {
		return channel.Result{}, err
	}
	if err := s.ensureWindow(instance, c, msg.Recipient, msg.To, false); err != nil {
		return channel.Result{}, err
	}
	contacts := make([]map[string]any, 0, len(msg.Contacts))
	for _, in := range msg.Contacts {
		name := map[string]any{"formatted_name": in.Name}
		if in.Name != "" {
			name["first_name"] = in.Name
		}
		phones := make([]map[string]any, 0, len(in.Phones))
		for _, p := range in.Phones {
			phones = append(phones, map[string]any{"phone": p})
		}
		emails := make([]map[string]any, 0, len(in.Emails))
		for _, e := range in.Emails {
			emails = append(emails, map[string]any{"email": e})
		}
		contacts = append(contacts, map[string]any{"name": name, "phones": phones, "emails": emails})
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"type":              "contacts",
		"contacts":          contacts,
	}
	addTarget(payload, to, recipient)
	addReply(payload, msg.ReplyTo)
	resp, err := s.client.SendMessage(ctx, token, c.PhoneNumberID, payload)
	if err != nil {
		return channel.Result{}, s.sendError(err)
	}
	return s.recordOutbound(instance, c, resp, to, recipient, "contact", "", "", "", "", "")
}

// SendInteractive sends reply buttons or a list.
func (s *Service) SendInteractive(ctx context.Context, instance *instance_model.Instance, msg channel.InteractiveMessage) (channel.Result, error) {
	if err := validateRecipientType(msg.RecipientType); err != nil {
		return channel.Result{}, err
	}
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return channel.Result{}, err
	}
	to, recipient, err := target(msg.To, msg.Recipient)
	if err != nil {
		return channel.Result{}, err
	}
	if err := s.ensureWindow(instance, c, msg.Recipient, msg.To, false); err != nil {
		return channel.Result{}, err
	}
	var interactive map[string]any
	if len(msg.Buttons) > 0 {
		if len(msg.Buttons) > 3 {
			return channel.Result{}, &APIError{Status: http.StatusUnprocessableEntity, Code: "invalid_request", Message: "Cloud interactive messages support at most 3 reply buttons"}
		}
		buttons := make([]map[string]any, 0, len(msg.Buttons))
		for _, b := range msg.Buttons {
			switch strings.ToLower(b.Type) {
			case "", "reply":
				buttons = append(buttons, map[string]any{"type": "reply", "reply": map[string]any{"id": b.ID, "title": b.Text}})
			case "url":
				buttons = append(buttons, map[string]any{"type": "cta_url", "cta_url": map[string]any{"url": b.URL, "display_text": b.Text}})
			default:
				return channel.Result{}, &APIError{Status: http.StatusNotImplemented, Code: "not_implemented_on_channel", Message: "this interactive button type is not implemented for Cloud API"}
			}
		}
		interactive = map[string]any{
			"type":   "button",
			"body":   map[string]any{"text": msg.Body},
			"action": map[string]any{"buttons": buttons},
		}
		if msg.Header != "" {
			interactive["header"] = map[string]any{"type": "text", "text": msg.Header}
		}
		if msg.Footer != "" {
			interactive["footer"] = map[string]any{"text": msg.Footer}
		}
	} else {
		rows := 0
		sections := make([]map[string]any, 0, len(msg.Sections))
		for _, sec := range msg.Sections {
			out := map[string]any{"title": sec.Title, "rows": []map[string]any{}}
			for _, row := range sec.Rows {
				rows++
				r := map[string]any{"id": row.ID, "title": row.Title}
				if row.Description != "" {
					r["description"] = row.Description
				}
				out["rows"] = append(out["rows"].([]map[string]any), r)
			}
			sections = append(sections, out)
		}
		if rows == 0 || rows > 10 {
			return channel.Result{}, &APIError{Status: http.StatusUnprocessableEntity, Code: "invalid_request", Message: "Cloud list messages require 1 to 10 rows"}
		}
		interactive = map[string]any{
			"type": "list",
			"body": map[string]any{"text": msg.Body},
			"action": map[string]any{
				"button": firstNonEmpty(msg.ButtonText, "Choose"), "sections": sections,
			},
		}
		if msg.Header != "" {
			interactive["header"] = map[string]any{"type": "text", "text": msg.Header}
		}
		if msg.Footer != "" {
			interactive["footer"] = map[string]any{"text": msg.Footer}
		}
	}
	payload := map[string]any{
		"messaging_product": "whatsapp", "recipient_type": recipientType(msg.RecipientType),
		"type": "interactive", "interactive": interactive,
	}
	addTarget(payload, to, recipient)
	addReply(payload, msg.ReplyTo)
	addOperation(payload, msg.OperationID)
	resp, err := s.client.SendMessage(ctx, token, c.PhoneNumberID, payload)
	if err != nil {
		return channel.Result{}, s.sendError(err)
	}
	return s.recordOutbound(instance, c, resp, to, recipient, "interactive", msg.Body, "", "", "", msg.OperationID)
}

// MarkRead marks a message as read, optionally with a typing indicator.
func (s *Service) MarkRead(ctx context.Context, instance *instance_model.Instance, messageID string, typing bool) error {
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"messaging_product": "whatsapp", "status": "read", "message_id": messageID,
	}
	if typing {
		payload["typing_indicator"] = map[string]any{"type": "text"}
	}
	_, err = s.client.SendMessage(ctx, token, c.PhoneNumberID, payload)
	if err != nil {
		return s.sendError(err)
	}
	return nil
}

// SendReaction sends a Cloud reaction.
func (s *Service) SendReaction(ctx context.Context, instance *instance_model.Instance, msg channel.ReactionMessage) (channel.Result, error) {
	c, token, err := s.credentialsToken(instance.Id)
	if err != nil {
		return channel.Result{}, err
	}
	to, recipient, err := target(msg.To, msg.Recipient)
	if err != nil {
		return channel.Result{}, err
	}
	if err := s.ensureWindow(instance, c, msg.Recipient, msg.To, false); err != nil {
		return channel.Result{}, err
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"type":              "reaction",
		"reaction":          map[string]any{"message_id": msg.MessageID, "emoji": msg.Emoji},
	}
	addTarget(payload, to, recipient)
	resp, err := s.client.SendMessage(ctx, token, c.PhoneNumberID, payload)
	if err != nil {
		return channel.Result{}, s.sendError(err)
	}
	return s.recordOutbound(instance, c, resp, to, recipient, "reaction", msg.Emoji, "", "", "", "")
}

func (s *Service) sendError(err error) error {
	var ge *GraphError
	if errors.As(err, &ge) {
		return MapGraphError(ge)
	}
	return &APIError{Status: http.StatusAccepted, Code: "cloud_send_outcome_unknown", Message: "the Cloud API outcome is unknown; do not retry blindly"}
}

func (s *Service) recordOutbound(instance *instance_model.Instance, c *cloud_model.Credentials, resp SendResponse, to, recipient, msgType, text, mediaURL, mime, filename, operationID string) (channel.Result, error) {
	id := resp.MessageID()
	now := time.Now()
	contactID := s.resolveContactID(instance.Id, to, recipient)
	if contactID == "" {
		contactID = "phone:" + digits(to)
	}
	chatJID := phoneJID(to)
	if chatJID == "" {
		chatJID = contactID
	}
	result := channel.Result{ID: id, Timestamp: now, Channel: channel.KindCloud, Recipient: recipient, WAID: to, OperationID: operationID}
	if id == "" {
		return result, nil
	}
	msg := message_model.Message{
		MessageID: id, InstanceId: instance.Id,
		Timestamp: now.Format("2006-01-02 15:04:05"), Status: "Sent", Source: digits(to),
		ChatJid: chatJID, SenderJid: chatJID, MessageType: msgType, TextContent: text,
		IsFromMe: true, Channel: "cloud", ContactID: contactID, MediaMimetype: mime,
		FileName: filename,
	}
	if mediaURL != "" {
		msg.MediaUrl = mediaURL
	}
	if s.messages != nil {
		go func() {
			if err := s.messages.InsertMessage(msg); err != nil && s.logger != nil {
				s.logger.GetLogger(instance.Id).LogWarn("[%s] failed to persist Cloud outbound %s: %v", instance.Id, id, err)
			}
		}()
	}
	if s.events != nil {
		post := map[string]any{
			"event": "SendMessage", "instanceId": instance.Id, "instanceName": instance.Name,
			"channel": "cloud",
			"data": map[string]any{
				"Info": map[string]any{
					"ID": id, "Chat": chatJID, "Sender": chatJID, "Timestamp": now,
				},
				"channel": "cloud", "contactId": contactID, "bsuid": recipient, "waId": to,
				"messageType": msgType, "text": text,
			},
		}
		if s.cfg == nil || s.cfg.WebhookIncludeToken {
			post["instanceToken"] = instance.Token
		}
		if data, err := json.Marshal(post); err == nil {
			go s.events.CallWebhook(instance, strings.ToLower(instance.Id+".sendmessage"), data)
			go s.events.SendToGlobalQueues("SendMessage", data, instance.Id)
		}
	}
	return result, nil
}

func (s *Service) ensureWindow(instance *instance_model.Instance, c *cloud_model.Credentials, recipient, to string, template bool) error {
	if template || c == nil || !c.EnforceWindow {
		return nil
	}
	contactID := s.resolveContactID(instance.Id, to, recipient)
	if contactID == "" {
		return nil
	}
	window, err := s.repo.GetWindow(instance.Id, contactID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return &WindowExpiredError{ContactID: contactID}
		}
		return nil
	}
	if time.Now().After(window.ExpiresAt) {
		return &WindowExpiredError{ContactID: contactID, ExpiresAt: window.ExpiresAt}
	}
	return nil
}

func (s *Service) downloadForUpload(ctx context.Context, rawURL string) ([]byte, string, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, "", errors.New("media URL is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := s.mediaHTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("failed to download media: HTTP %s", resp.Status)
	}
	max := int64(100 << 20)
	if s.cfg != nil && s.cfg.CloudAPIMaxMediaBytes > 0 {
		max = s.cfg.CloudAPIMaxMediaBytes
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > max {
		return nil, "", errors.New("media exceeds Cloud API limit")
	}
	mime := resp.Header.Get("Content-Type")
	return data, mime, nil
}

func (s *Service) downloadMedia(ctx context.Context, token, mediaID string) ([]byte, MediaInfo, error) {
	info, err := s.client.GetMedia(ctx, token, mediaID)
	if err != nil {
		return nil, info, err
	}
	max := int64(100 << 20)
	if s.cfg != nil && s.cfg.CloudAPIMaxMediaBytes > 0 {
		max = s.cfg.CloudAPIMaxMediaBytes
	}
	data, err := s.client.DownloadMedia(ctx, token, info.URL, max)
	if err != nil {
		return nil, info, err
	}
	if info.SHA256 != "" {
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), info.SHA256) {
			return nil, info, errors.New("cloud: media sha256 does not match")
		}
	}
	return data, info, nil
}

func (s *Service) storeInboundMedia(instance *instance_model.Instance, messageID string, data []byte, mime, filename string) (string, error) {
	if s.media != nil {
		if storage, delivery := s.media.MediaStorageFor(instance); storage != nil && delivery != "base64" {
			url, err := storage.Store(context.Background(), data, filename, mime)
			if err == nil {
				return url, nil
			}
		}
	}
	if s.cfg == nil || !s.cfg.MediaLocalStore {
		return "", nil
	}
	if err := localmedia.Save(instance.Id, messageID, data); err != nil {
		return "", err
	}
	return localmedia.URLPath(messageID), nil
}

// --- small helpers ---------------------------------------------------------

var digitsOnly = regexp.MustCompile(`\D`)

func digits(s string) string { return digitsOnly.ReplaceAllString(s, "") }

func phoneJID(phone string) string {
	d := digits(phone)
	if d == "" {
		return ""
	}
	return d + "@s.whatsapp.net"
}

func (s *Service) resolveContactID(instanceID, to, recipient string) string {
	if strings.TrimSpace(recipient) != "" {
		if c, err := s.repo.FindContactByBSUID(instanceID, strings.TrimSpace(recipient)); err == nil {
			return c.ContactID
		}
	}
	if d := digits(to); d != "" {
		if c, err := s.repo.FindContactByWAID(instanceID, d); err == nil {
			return c.ContactID
		}
	}
	return contactIDForRecipient(to, recipient)
}

func contactIDForRecipient(to, recipient string) string {
	if strings.TrimSpace(recipient) != "" {
		return "bsuid:" + strings.TrimSpace(recipient)
	}
	if d := digits(to); d != "" {
		return "phone:" + d
	}
	return ""
}

func target(to, recipient string) (string, string, error) {
	to = digits(to)
	recipient = strings.TrimSpace(recipient)
	if to == "" && recipient == "" {
		return "", "", &APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "number or recipient is required"}
	}
	return to, recipient, nil
}

func validateRecipientType(v string) error {
	if strings.EqualFold(strings.TrimSpace(v), "group") {
		return &channel.NotImplementedError{Channel: channel.KindCloud, Op: "group messaging"}
	}
	return nil
}

func recipientType(v string) string {
	if strings.EqualFold(v, "group") {
		return "group"
	}
	return "individual"
}

func addTarget(payload map[string]any, to, recipient string) {
	if to != "" {
		payload["to"] = to
	}
	if recipient != "" {
		payload["recipient"] = recipient
	}
}

func addReply(payload map[string]any, id string) {
	if strings.TrimSpace(id) != "" {
		payload["context"] = map[string]any{"message_id": strings.TrimSpace(id)}
	}
}

func addOperation(payload map[string]any, id string) {
	if strings.TrimSpace(id) != "" {
		payload["biz_opaque_callback_data"] = strings.TrimSpace(id)
	}
}

func validMediaKind(kind string) bool {
	switch kind {
	case "image", "video", "audio", "document", "sticker":
		return true
	}
	return false
}

func (s *Service) markWebhook(instanceID string) {
	if instanceID == "" {
		return
	}
	s.lastWebhookMu.Lock()
	s.lastWebhook[instanceID] = time.Now()
	s.lastWebhookMu.Unlock()
}

// LastWebhookAt returns the last successful webhook time for diagnostics.
func (s *Service) LastWebhookAt(instanceID string) *time.Time {
	s.lastWebhookMu.RLock()
	defer s.lastWebhookMu.RUnlock()
	if t, ok := s.lastWebhook[instanceID]; ok {
		return &t
	}
	return nil
}

func publicError(err error) string {
	var ge *GraphError
	if errors.As(err, &ge) {
		if ge.Details != "" {
			return ge.Details
		}
		if ge.UserMsg != "" {
			return ge.UserMsg
		}
		return ge.Message
	}
	return err.Error()
}

// RunCleanup periodically removes old webhook/status/account/media rows.
func (s *Service) RunCleanup(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if _, err := s.repo.Cleanup(now, 30, 90, 90, 30); err != nil && s.logger != nil {
				s.logger.GetLogger("cloud").LogWarn("[cloud] retention cleanup failed: %v", err)
			}
		}
	}
}
