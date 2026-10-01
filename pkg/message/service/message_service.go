package message_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/felipeestevanatto/wamux/pkg/safemap"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	localmedia "github.com/felipeestevanatto/wamux/pkg/media"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
	"github.com/felipeestevanatto/wamux/pkg/utils"
	whatsmeow_service "github.com/felipeestevanatto/wamux/pkg/whatsmeow/service"
	"github.com/vincent-petithory/dataurl"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type MessageService interface {
	React(data *ReactStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	ChatPresence(data *ChatPresenceStruct, instance *instance_model.Instance) (string, error)
	SubscribePresence(data *SubscribePresenceStruct, instance *instance_model.Instance) error
	MarkRead(data *MarkReadStruct, instance *instance_model.Instance) (string, error)
	MarkPlayed(data *MarkPlayedStruct, instance *instance_model.Instance) (string, error)
	DownloadMedia(data *DownloadMediaStruct, instance *instance_model.Instance, request *http.Request) (*dataurl.DataURL, string, error)
	GetMessageStatus(data *MessageStatusStruct, instance *instance_model.Instance) (*message_model.Message, string, error)
	DeleteMessageEveryone(data *MessageStruct, instance *instance_model.Instance) (string, string, error)
	EditMessage(data *EditMessageStruct, instance *instance_model.Instance) (string, string, error)

	// History readback.
	GetHistory(data *HistoryQuery, instance *instance_model.Instance) ([]message_model.Message, error)
	ListChats(instance *instance_model.Instance, limit int) ([]message_repository.ChatSummary, error)

	// GetStoredMedia opens a locally stored attachment for a message, scoped to
	// the given instance. It also returns the stored content type.
	GetStoredMedia(messageID string, instance *instance_model.Instance) (*os.File, fs.FileInfo, string, error)
}

type messageService struct {
	clientPointer     *safemap.Map[*whatsmeow.Client]
	messageRepository message_repository.MessageRepository
	whatsmeowService  whatsmeow_service.WhatsmeowService
	loggerWrapper     *logger_wrapper.LoggerManager
}

type ReactStruct struct {
	Number      string `json:"number" example:"5511999999999"`
	Reaction    string `json:"reaction" example:"👍"`
	Id          string `json:"id" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	FromMe      bool   `json:"fromMe" example:"false"`
	Participant string `json:"participant,omitempty" example:"5511999999999@s.whatsapp.net"`
}

type ChatPresenceStruct struct {
	Number  string `json:"number" example:"5511999999999"`
	State   string `json:"state" example:"composing"`
	IsAudio bool   `json:"isAudio" example:"false"`
	// Delay, in milliseconds, keeps the "composing"/"recording" indicator alive
	// for the given duration (re-sending it periodically) and then sends "paused".
	// Only applies when State is "composing". 0 = single fire (legacy behaviour).
	Delay int `json:"delay" example:"5000"`
}

type SubscribePresenceStruct struct {
	Number string `json:"number" example:"5511999999999"`
}

type MarkReadStruct struct {
	Id     []string `json:"id" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	Number string   `json:"number" example:"5511999999999"`
}

type MarkPlayedStruct struct {
	Id     []string `json:"id" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	Number string   `json:"number" example:"5511999999999"`
}

type DownloadMediaStruct struct {
	Message *waE2E.Message `json:"message"`
	// Optional message context. When the media is gone (403/404/410) and this is
	// provided, the server asks the sender's phone to re-upload it (media retry)
	// and the next request with the same `id` returns the refreshed bytes.
	Id          string `json:"id,omitempty" example:"3EB0A1B2C3D4E5F6A7B8C9"`
	Chat        string `json:"chat,omitempty" example:"5511999999999@s.whatsapp.net"`
	FromMe      bool   `json:"fromMe,omitempty" example:"true"`
	IsGroup     bool   `json:"isGroup,omitempty" example:"false"`
	Participant string `json:"participant,omitempty" example:"5511999999999@s.whatsapp.net"`
}

type MessageStatusStruct struct {
	Id string `json:"id" example:"3EB0A1B2C3D4E5F6A7B8C9"`
}

type MessageStruct struct {
	Chat      string `json:"chat" example:"5511999999999@s.whatsapp.net"`
	MessageID string `json:"messageId" example:"3EB0A1B2C3D4E5F6A7B8C9"`
}

type EditMessageStruct struct {
	Chat      string `json:"chat" example:"5511999999999@s.whatsapp.net"`
	Message   string `json:"message" example:"Mensagem editada"`
	MessageID string `json:"messageId" example:"3EB0A1B2C3D4E5F6A7B8C9"`
}

type MessageSendStruct struct {
	Info               types.MessageInfo
	Message            *waE2E.Message
	MessageContextInfo *waE2E.ContextInfo
}

// HistoryQuery is the input for GET /chat/history. Chat accepts a phone number
// or a full JID (including a group JID); Before pages backwards and is the
// timestamp of the oldest message already seen.
type HistoryQuery struct {
	Chat   string `form:"chat" json:"chat" example:"5511999999999"`
	Limit  int    `form:"limit" json:"limit" example:"50"`
	Before string `form:"before" json:"before" example:"2026-05-09 10:00:00"`
}

func (m *messageService) ensureClientConnected(instanceId string) (*whatsmeow.Client, error) {
	client := m.clientPointer.Get(instanceId)
	m.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Checking client connection status - Client exists: %v", instanceId, client != nil)

	if client == nil {
		m.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] No client found, attempting to start new instance", instanceId)
		err := m.whatsmeowService.StartInstance(instanceId)
		if err != nil {
			m.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to start instance: %v", instanceId, err)
			return nil, errors.New("no active session found")
		}

		m.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Instance started, waiting 2 seconds...", instanceId)
		time.Sleep(2 * time.Second)

		client = m.clientPointer.Get(instanceId)
		m.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Checking new client - Exists: %v, Connected: %v",
			instanceId,
			client != nil,
			client != nil && client.IsConnected())

		if client == nil || !client.IsConnected() {
			m.loggerWrapper.GetLogger(instanceId).LogError("[%s] New client validation failed - Exists: %v, Connected: %v",
				instanceId,
				client != nil,
				client != nil && client.IsConnected())
			return nil, errors.New("no active session found")
		}
	} else if !client.IsConnected() {
		m.loggerWrapper.GetLogger(instanceId).LogError("[%s] Existing client is disconnected - Connected status: %v",
			instanceId,
			client.IsConnected())
		return nil, errors.New("client disconnected")
	}

	m.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Client successfully validated - Connected: %v", instanceId, client.IsConnected())
	return client, nil
}

// reactionAuthor derives the author JID that whatsmeow's BuildMessageKey needs
// for a reaction: empty for our own messages (so FromMe=true), the participant
// for a group message from someone else, the chat for a 1:1 from someone else.
// authorKnown is false for a group message with no participant, where the caller
// keeps the explicit FromMe=false it was given.
func reactionAuthor(fromMe, isGroup bool, participant string, chat types.JID) (types.JID, bool) {
	switch {
	case fromMe:
		return types.EmptyJID, true
	case isGroup && participant != "":
		if participantJID, ok := utils.ParseJID(participant); ok {
			return utils.CanonicalJID(participantJID), true
		}
		return types.EmptyJID, false
	case !isGroup:
		return chat, true
	default:
		return types.EmptyJID, false
	}
}

func (m *messageService) React(data *ReactStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	client, err := m.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	msgId := ""

	recipient, ok := utils.ParseJID(data.Number)
	if !ok {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return nil, errors.New("invalid phone number")
	}

	// Strip the "+" that ParseJID/CreateJID adds. The recipient is used both as
	// the SendMessage target (usync/device resolution) AND as the MessageKey
	// RemoteJID that references the reacted message's chat. A malformed "+JID"
	// breaks device resolution (usync) and prevents the reaction from matching
	// the original message's chat. See utils.CanonicalJID.
	recipient = utils.CanonicalJID(recipient)

	if data.Id == "" {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Missing Id in Payload", instance.Id)
		return nil, errors.New("missing id in payload")
	} else {
		msgId = data.Id
	}

	fromMe := data.FromMe
	reaction := data.Reaction
	if reaction == "remove" {
		reaction = ""
	}

	isGroup := strings.Contains(data.Number, "@g.us")

	// Build the reaction with whatsmeow's BuildReaction/BuildMessageKey: it
	// derives the key's FromMe from the author (comparing against both our phone
	// number and our LID) and sets the group participant only when the author is
	// someone else — instead of trusting the API's fromMe flag and participant
	// verbatim. msgId is the ID of the message being reacted to, not the reaction
	// envelope (so it must not be reused as the envelope ID).
	author, authorKnown := reactionAuthor(fromMe, isGroup, data.Participant, recipient)
	if !authorKnown {
		m.loggerWrapper.GetLogger(instance.Id).LogWarn(
			"[%s] Reaction to a group message without `participant`; the reaction may be dropped", instance.Id)
	}

	msg := client.BuildReaction(recipient, author, msgId, reaction)

	// A group message with no participant leaves the author unknown, so
	// BuildMessageKey would mark the key FromMe; keep the explicit FromMe=false
	// the API asked for (and no participant), as before.
	if !fromMe {
		if key := msg.GetReactionMessage().GetKey(); key != nil && key.GetFromMe() {
			key.FromMe = proto.Bool(false)
		}
	}

	// Do NOT pass ID: msgId in SendRequestExtra. Doing so would reuse the
	// original message ID as the reaction envelope ID; WhatsApp silently
	// deduplicates it and drops the reaction. Let whatsmeow generate a
	// fresh, unique ID for the envelope.
	response, err := client.SendMessage(context.Background(), recipient, msg)
	if err != nil {
		return nil, err
	}

	messageType := "ReactionMessage"

	messageInfo := types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     recipient,
			Sender:   *client.Store.ID,
			IsFromMe: true,
			IsGroup:  isGroup,
		},
		ID:        response.ID,
		Timestamp: time.Now(),
		ServerID:  response.ServerID,
		Type:      messageType,
	}

	messageSent := &MessageSendStruct{
		Info:    messageInfo,
		Message: msg,
	}

	return messageSent, nil
}

// presenceAfterTransientOnline is the presence an instance must return to after
// an operation that briefly marked it available (typing indicators, presence
// subscription). WhatsApp suppresses push notifications on the operator's phone
// while a linked device is "available", so an instance with alwaysOnline=false
// must not be left available afterwards. Issues #70 / #54 / #55.
func presenceAfterTransientOnline(alwaysOnline bool) types.Presence {
	if alwaysOnline {
		return types.PresenceAvailable
	}
	return types.PresenceUnavailable
}

func (m *messageService) ChatPresence(data *ChatPresenceStruct, instance *instance_model.Instance) (string, error) {
	client, err := m.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Number)
	if !ok {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	// chatstate (typing) is a RAW node sent without usync normalization, so it
	// needs a canonical digits-only JID or WhatsApp silently drops it. See
	// utils.CanonicalJID for the full rationale.
	recipient = utils.CanonicalJID(recipient)

	media := ""

	if data.IsAudio {
		media = "audio"
	}

	// WhatsApp only forwards chatstate (typing / recording) events to the
	// recipient while the sender is marked online. SendChatPresence merely
	// sends the chatstate node — it does NOT mark us available. Background
	// presence handling (events.AppStateSyncComplete) may have set us to
	// Unavailable, in which case the server silently drops the typing
	// indicator. Mark ourselves available first to guarantee delivery.
	if presErr := client.SendPresence(context.Background(), types.PresenceAvailable); presErr != nil {
		m.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] SendPresence(available) before chatstate failed (non-fatal): %v", instance.Id, presErr)
	}
	// Return to the instance's configured presence once the chatstate is done.
	// With alwaysOnline=false this sends Unavailable so the linked device stops
	// looking "online" and the operator's phone keeps receiving notifications
	// (issues #70/#54/#55). Runs after the optional keep-alive loop below.
	defer func() {
		state := presenceAfterTransientOnline(instance.AlwaysOnline)
		if rerr := client.SendPresence(context.Background(), state); rerr != nil {
			m.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Failed to restore presence %s after chatstate (non-fatal): %v", instance.Id, state, rerr)
		} else {
			m.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Restored presence to %s after chatstate", instance.Id, state)
		}
	}()

	state := types.ChatPresence(data.State)
	mediaType := types.ChatPresenceMedia(media)

	err = client.SendChatPresence(context.Background(), recipient, state, mediaType)
	if err != nil {
		return "", err
	}

	// A single "composing" indicator is ephemeral: WhatsApp expires it after a
	// few seconds unless refreshed. When a Delay is provided (and we're typing),
	// keep the indicator alive for the requested duration by re-sending it, then
	// send "paused" so the indicator clears cleanly instead of timing out.
	if data.Delay > 0 && state == types.ChatPresenceComposing {
		const keepAliveInterval = 5 * time.Second
		const maxDelay = 60 * time.Second

		remaining := time.Duration(data.Delay) * time.Millisecond
		if remaining > maxDelay {
			remaining = maxDelay
		}

		for remaining > 0 {
			sleep := keepAliveInterval
			if remaining < sleep {
				sleep = remaining
			}
			time.Sleep(sleep)
			remaining -= sleep

			if remaining > 0 {
				// Refresh the indicator so it doesn't expire mid-delay.
				if refreshErr := client.SendChatPresence(context.Background(), recipient, state, mediaType); refreshErr != nil {
					m.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Refresh chatstate failed (non-fatal): %v", instance.Id, refreshErr)
				}
			}
		}

		if pausedErr := client.SendChatPresence(context.Background(), recipient, types.ChatPresencePaused, mediaType); pausedErr != nil {
			m.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] SendChatPresence(paused) failed (non-fatal): %v", instance.Id, pausedErr)
		}
	}

	m.loggerWrapper.GetLogger(instance.Id).LogInfo("Presence (%s) sent to %s", data.State, data.Number)

	return ts.String(), nil
}

// SubscribePresence subscribes to a contact's presence (online / last-seen).
// WhatsApp only delivers events.Presence for JIDs we've explicitly subscribed to,
// and only while we're marked available — so we send available first (idempotent;
// ChatPresence and the background presence loop already do this). Subscriptions are
// ephemeral (reset on reconnect), so the caller re-subscribes when a chat is opened.
func (m *messageService) SubscribePresence(data *SubscribePresenceStruct, instance *instance_model.Instance) error {
	client, err := m.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}

	recipient, ok := utils.ParseJID(data.Number)
	if !ok {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] SubscribePresence: invalid number %s", instance.Id, data.Number)
		return errors.New("invalid phone number")
	}
	recipient = utils.CanonicalJID(recipient)

	// Must be available to receive others' presence updates (non-fatal if it fails).
	if presErr := client.SendPresence(context.Background(), types.PresenceAvailable); presErr != nil {
		m.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] SendPresence(available) before subscribe failed (non-fatal): %v", instance.Id, presErr)
	}
	// Presence updates only flow while the device is available, but leaving it
	// available silences the operator's phone. With alwaysOnline=false we honor
	// the notification setting: restore Unavailable after subscribing, and warn
	// that live Presence events will therefore be limited. Issues #70/#54/#55.
	if !instance.AlwaysOnline {
		m.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] alwaysOnline=false: restoring unavailable after subscribe; live Presence updates require alwaysOnline=true", instance.Id)
	}
	defer func() {
		state := presenceAfterTransientOnline(instance.AlwaysOnline)
		if rerr := client.SendPresence(context.Background(), state); rerr != nil {
			m.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Failed to restore presence %s after subscribe (non-fatal): %v", instance.Id, state, rerr)
		} else {
			m.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Restored presence to %s after subscribe", instance.Id, state)
		}
	}()

	if err := client.SubscribePresence(context.Background(), recipient); err != nil {
		return err
	}

	m.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Subscribed to presence of %s", instance.Id, data.Number)
	return nil
}

func (m *messageService) MarkRead(data *MarkReadStruct, instance *instance_model.Instance) (string, error) {
	client, err := m.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	jid, ok := utils.ParseJID(data.Number)
	if !ok {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	// Read receipts are RAW nodes (no usync) — strip the "+" so the receipt
	// reaches the recipient. Same root cause as the typing fix above.
	jid = utils.CanonicalJID(jid)

	err = client.MarkRead(context.Background(), data.Id, time.Now(), jid, jid)
	if err != nil {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error marking message as read: %v", instance.Id, err)
		return "", errors.New("error marking message as read")
	}

	return ts.String(), nil
}

func (m *messageService) MarkPlayed(data *MarkPlayedStruct, instance *instance_model.Instance) (string, error) {
	client, err := m.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	jid, ok := utils.ParseJID(data.Number)
	if !ok {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	// Played receipts are RAW nodes (no usync) — strip the "+" so the receipt
	// reaches the recipient. Same root cause as the MarkRead fix.
	jid = utils.CanonicalJID(jid)

	err = client.MarkRead(context.Background(), data.Id, time.Now(), jid, jid, types.ReceiptTypePlayed)
	if err != nil {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error marking message as played: %v", instance.Id, err)
		return "", errors.New("error marking message as played")
	}

	return ts.String(), nil
}

func (m *messageService) DownloadMedia(data *DownloadMediaStruct, instance *instance_model.Instance, request *http.Request) (*dataurl.DataURL, string, error) {
	client, err := m.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, "", err
	}

	var ts time.Time

	msg := data.Message

	mimetype := ""
	var mediaData []byte

	img := msg.GetImageMessage()
	audio := msg.GetAudioMessage()
	document := msg.GetDocumentMessage()
	video := msg.GetVideoMessage()
	sticker := msg.GetStickerMessage()

	if img == nil && audio == nil && document == nil && video == nil && sticker == nil {
		return nil, "", errors.New("invalid media type")
	}

	userDirectory := fmt.Sprintf(`files/user_%s`, instance.Id)
	_, err = os.Stat(userDirectory)
	if os.IsNotExist(err) {
		errDir := os.MkdirAll(userDirectory, 0751)
		if errDir != nil {
			m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Could not create user directory (%s)", instance.Id, userDirectory)
			return nil, "", errDir
		}
	}

	// Resolve which media this message carries.
	type mediaJob struct {
		label string
		media whatsmeow.DownloadableMessage
		mime  string
	}
	var job *mediaJob
	switch {
	case img != nil:
		job = &mediaJob{"image", img, img.GetMimetype()}
	case audio != nil:
		job = &mediaJob{"audio", audio, audio.GetMimetype()}
	case document != nil:
		job = &mediaJob{"document", document, document.GetMimetype()}
	case video != nil:
		job = &mediaJob{"video", video, video.GetMimetype()}
	case sticker != nil:
		job = &mediaJob{"sticker", sticker, sticker.GetMimetype()}
	}

	// A previously requested media retry may already have refreshed the bytes.
	if data.Id != "" {
		if refreshed, ok := m.whatsmeowService.GetRetriedMedia(instance.Id, data.Id); ok {
			m.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Serving %s from the media-retry cache", instance.Id, job.label)
			return dataurl.New(refreshed, job.mime), ts.String(), nil
		}
	}

	mediaData, err = client.Download(context.Background(), job.media)
	if err != nil {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to download %s", instance.Id, job.label)
		// An expired direct path (403/404/410) can be refreshed by asking the
		// sender's phone to re-upload the media. This needs the message context
		// (id/chat/fromMe), which is optional in the request; without it we can
		// only report the failure.
		if data.Id != "" && isMediaGoneError(err) {
			if info := data.messageInfo(); info != nil {
				if rerr := m.whatsmeowService.RequestMediaRetry(instance.Id, info, mediaKeyOf(msg), job.media); rerr == nil {
					return nil, "", fmt.Errorf("%s is no longer available; a media retry was requested, try again in a few seconds", job.label)
				}
			}
		}
		return nil, "", fmt.Errorf("Failed to download %s %v", job.label, err)
	}
	mimetype = job.mime

	dataURL := dataurl.New(mediaData, mimetype)

	return dataURL, ts.String(), nil
}

// isMediaGoneError reports whether a download failed because the media expired.
func isMediaGoneError(err error) bool {
	return errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith403) ||
		errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith404) ||
		errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith410)
}

// mediaKeyOf returns the media key of whichever media the message carries.
func mediaKeyOf(msg *waE2E.Message) []byte {
	switch {
	case msg.GetImageMessage() != nil:
		return msg.GetImageMessage().GetMediaKey()
	case msg.GetAudioMessage() != nil:
		return msg.GetAudioMessage().GetMediaKey()
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage().GetMediaKey()
	case msg.GetVideoMessage() != nil:
		return msg.GetVideoMessage().GetMediaKey()
	case msg.GetStickerMessage() != nil:
		return msg.GetStickerMessage().GetMediaKey()
	}
	return nil
}

// messageInfo rebuilds the message source a media retry needs from the optional
// request context. Returns nil when the caller did not provide it.
func (d *DownloadMediaStruct) messageInfo() *types.MessageInfo {
	if d == nil || d.Id == "" || d.Chat == "" {
		return nil
	}
	chat, ok := utils.ParseJID(d.Chat)
	if !ok {
		return nil
	}
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     utils.CanonicalJID(chat),
			IsFromMe: d.FromMe,
			IsGroup:  d.IsGroup,
		},
		ID: d.Id,
	}
	if d.Participant != "" {
		if p, ok := utils.ParseJID(d.Participant); ok {
			info.Sender = utils.CanonicalJID(p)
		}
	}
	return info
}

func (m *messageService) GetMessageStatus(data *MessageStatusStruct, instance *instance_model.Instance) (*message_model.Message, string, error) {
	_, err := m.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, "", err
	}

	var ts time.Time

	// Tenant-scoped: an instance may only read its own messages, even though it
	// knows the (globally unique) message id.
	result, err := m.messageRepository.GetMessageByIDForInstance(instance.Id, data.Id)
	if err != nil {
		return nil, "", err
	}
	if result == nil {
		return nil, "", errors.New("message not found")
	}

	return result, ts.String(), nil
}

func (m *messageService) DeleteMessageEveryone(data *MessageStruct, instance *instance_model.Instance) (string, string, error) {
	client, err := m.ensureClientConnected(instance.Id)
	if err != nil {
		return "", "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", "", errors.New("invalid phone number")
	}
	// The chat JID lands inside the revoke's protocolMessage Key: with the "+"
	// prefix CreateJID adds, receiving devices look up a chat that doesn't
	// exist and silently ignore the revoke. See utils.CanonicalJID.
	recipient = utils.CanonicalJID(recipient)

	m.loggerWrapper.GetLogger(instance.Id).LogInfo("Revoking message %s from %s", data.MessageID, recipient)

	resp, err := client.SendMessage(
		context.Background(),
		recipient,
		client.BuildRevoke(recipient, types.EmptyJID, data.MessageID))
	if err != nil {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error revoking message: %v", instance.Id, err)
		return "", "", err
	}

	response := resp.ID

	return response, ts.String(), nil
}

func (m *messageService) EditMessage(data *EditMessageStruct, instance *instance_model.Instance) (string, string, error) {
	client, err := m.ensureClientConnected(instance.Id)
	if err != nil {
		return "", "", err
	}

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", "", errors.New("invalid phone number")
	}
	// Same as DeleteMessageEveryone: the JID lands inside the edit's
	// protocolMessage Key, so the "+" prefix makes recipients ignore it.
	recipient = utils.CanonicalJID(recipient)

	resp, err := client.SendMessage(
		context.Background(),
		recipient,
		client.BuildEdit(
			recipient,
			data.MessageID,
			&waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text: &data.Message,
				},
			}))
	if err != nil {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error revoking message: %v", instance.Id, err)
		return "", "", err
	}

	return resp.ID, resp.Timestamp.String(), nil
}

// canonicalChat turns a phone number or JID into the canonical JID stored in
// messages.chat_jid. When the chat is LID-addressed and the running instance
// knows the LID->phone mapping, the phone-number JID is returned, so a 1:1
// conversation is not split across a LID row and a PN row.
func (m *messageService) canonicalChat(instanceId, raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return "", false
	}
	jid, ok := utils.ParseJID(raw)
	if !ok {
		return "", false
	}
	jid = utils.CanonicalJID(jid)
	if m.clientPointer != nil {
		if client := m.clientPointer.Get(instanceId); client != nil {
			jid = whatsmeow_service.CanonicalChatJID(context.Background(), client, jid)
		}
	}
	return jid.ToNonAD().String(), true
}

// chatJidVariants returns the canonical JID plus, when known, its counterpart
// (the LID for a phone number, or vice versa), so a conversation already split
// across both forms is read back together.
func (m *messageService) chatJidVariants(instanceId, canonical string) []string {
	variants := []string{canonical}
	if m.clientPointer == nil {
		return variants
	}
	if client := m.clientPointer.Get(instanceId); client != nil {
		if jid, ok := utils.ParseJID(canonical); ok {
			if alt, ok := whatsmeow_service.AlternateChatJID(context.Background(), client, jid.ToNonAD()); ok {
				if altJid := alt.ToNonAD().String(); altJid != canonical {
					variants = append(variants, altJid)
				}
			}
		}
	}
	return variants
}

// historyLimit clamps a requested page size (same bounds as the repository).
func historyLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 500 {
		return 500
	}
	return limit
}

// mergeMessagesByID de-duplicates by message id (last write wins) and sorts
// newest-first. It is used when a conversation's rows come from more than one
// JID variant.
func mergeMessagesByID(messages []message_model.Message) []message_model.Message {
	byID := make(map[string]message_model.Message, len(messages))
	for _, msg := range messages {
		byID[msg.MessageID] = msg
	}
	out := make([]message_model.Message, 0, len(byID))
	for _, msg := range byID {
		out = append(out, msg)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Timestamp != out[j].Timestamp {
			return out[i].Timestamp > out[j].Timestamp
		}
		return out[i].Id > out[j].Id
	})
	return out
}

// GetHistory returns one conversation's stored messages, newest first,
// including rows stored under the other JID form (LID vs phone number).
func (m *messageService) GetHistory(data *HistoryQuery, instance *instance_model.Instance) ([]message_model.Message, error) {
	if data == nil || instance == nil {
		return nil, errors.New("invalid request")
	}
	chatJid, ok := m.canonicalChat(instance.Id, data.Chat)
	if !ok {
		return nil, errors.New("invalid chat")
	}

	limit := historyLimit(data.Limit)
	before := strings.TrimSpace(data.Before)

	var all []message_model.Message
	for _, variant := range m.chatJidVariants(instance.Id, chatJid) {
		msgs, err := m.messageRepository.ListMessages(instance.Id, variant, before, limit)
		if err != nil {
			m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to read message history for %s: %v", instance.Id, variant, err)
			return nil, err
		}
		all = append(all, msgs...)
	}

	all = mergeMessagesByID(all)
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// ListChats returns each conversation's newest message plus its message count,
// merging rows that belong to the same 1:1 conversation (LID + phone number).
func (m *messageService) ListChats(instance *instance_model.Instance, limit int) ([]message_repository.ChatSummary, error) {
	if instance == nil {
		return nil, errors.New("invalid instance")
	}

	effective := historyLimit(limit)
	// Overfetch so merging LID+PN rows still fills the requested page.
	chats, err := m.messageRepository.ListChats(instance.Id, effective*4)
	if err != nil {
		m.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to list chats: %v", instance.Id, err)
		return nil, err
	}

	merged := m.mergeChatSummaries(instance.Id, chats)
	if len(merged) > effective {
		merged = merged[:effective]
	}
	return merged, nil
}

// GetStoredMedia opens a message's locally stored attachment. The message must
// belong to the given instance so one tenant cannot read another's media.
func (m *messageService) GetStoredMedia(messageID string, instance *instance_model.Instance) (*os.File, fs.FileInfo, string, error) {
	if instance == nil || messageID == "" {
		return nil, nil, "", errors.New("invalid request")
	}
	if m.messageRepository == nil {
		return nil, nil, "", errors.New("media not found")
	}
	// Tenant-scoped lookup: the repository enforces the instance filter, so a
	// message id from another tenant can never be opened here.
	msg, err := m.messageRepository.GetMessageByIDForInstance(instance.Id, messageID)
	if err != nil {
		return nil, nil, "", err
	}
	if msg == nil {
		return nil, nil, "", errors.New("media not found")
	}
	file, info, err := localmedia.Open(instance.Id, messageID)
	if err != nil {
		return nil, nil, "", err
	}
	return file, info, msg.MediaMimetype, nil
}

// mergeChatSummaries canonicalizes each conversation's JID and merges entries
// that collapse to the same one, summing counts and keeping the newest message.
func (m *messageService) mergeChatSummaries(instanceId string, chats []message_repository.ChatSummary) []message_repository.ChatSummary {
	var client *whatsmeow.Client
	if m.clientPointer != nil {
		client = m.clientPointer.Get(instanceId)
	}

	byJid := make(map[string]message_repository.ChatSummary, len(chats))
	order := make([]string, 0, len(chats))
	for _, c := range chats {
		if jid, ok := utils.ParseJID(c.ChatJid); ok {
			c.ChatJid = whatsmeow_service.CanonicalChatJID(context.Background(), client, jid.ToNonAD()).String()
		}
		if existing, ok := byJid[c.ChatJid]; ok {
			total := existing.MessageCount + c.MessageCount
			if c.Timestamp > existing.Timestamp {
				c.MessageCount = total
				byJid[c.ChatJid] = c
			} else {
				existing.MessageCount = total
				byJid[c.ChatJid] = existing
			}
		} else {
			byJid[c.ChatJid] = c
			order = append(order, c.ChatJid)
		}
	}

	out := make([]message_repository.ChatSummary, 0, len(order))
	for _, jid := range order {
		out = append(out, byJid[jid])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	return out
}

func NewMessageService(
	clientPointer *safemap.Map[*whatsmeow.Client],
	messageRepository message_repository.MessageRepository,
	whatsmeowService whatsmeow_service.WhatsmeowService,
	loggerWrapper *logger_wrapper.LoggerManager,
) MessageService {
	return &messageService{
		clientPointer:     clientPointer,
		messageRepository: messageRepository,
		whatsmeowService:  whatsmeowService,
		loggerWrapper:     loggerWrapper,
	}
}
