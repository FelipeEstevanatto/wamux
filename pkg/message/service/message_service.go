package message_service

import (
	"context"
	"errors"
	"fmt"
	"github.com/evolution-foundation/evolution-go/pkg/safemap"
	"net/http"
	"os"
	"strings"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
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
}

type messageService struct {
	clientPointer     *safemap.Map[*whatsmeow.Client]
	messageRepository message_repository.MessageRepository
	whatsmeowService  whatsmeow_service.WhatsmeowService
	loggerWrapper     *logger_wrapper.LoggerManager
}

type ReactStruct struct {
	Number      string `json:"number"`
	Reaction    string `json:"reaction"`
	Id          string `json:"id"`
	FromMe      bool   `json:"fromMe"`
	Participant string `json:"participant,omitempty"`
}

type ChatPresenceStruct struct {
	Number  string `json:"number"`
	State   string `json:"state"`
	IsAudio bool   `json:"isAudio"`
	// Delay, in milliseconds, keeps the "composing"/"recording" indicator alive
	// for the given duration (re-sending it periodically) and then sends "paused".
	// Only applies when State is "composing". 0 = single fire (legacy behaviour).
	Delay int `json:"delay"`
}

type SubscribePresenceStruct struct {
	Number string `json:"number"`
}

type MarkReadStruct struct {
	Id     []string `json:"id"`
	Number string   `json:"number"`
}

type MarkPlayedStruct struct {
	Id     []string `json:"id"`
	Number string   `json:"number"`
}

type DownloadMediaStruct struct {
	Message *waE2E.Message `json:"message"`
	// Optional message context. When the media is gone (403/404/410) and this is
	// provided, the server asks the sender's phone to re-upload it (media retry)
	// and the next request with the same `id` returns the refreshed bytes.
	Id          string `json:"id,omitempty"`
	Chat        string `json:"chat,omitempty"`
	FromMe      bool   `json:"fromMe,omitempty"`
	IsGroup     bool   `json:"isGroup,omitempty"`
	Participant string `json:"participant,omitempty"`
}

type MessageStatusStruct struct {
	Id string `json:"id"`
}

type MessageStruct struct {
	Chat      string `json:"chat"`
	MessageID string `json:"messageId"`
}

type EditMessageStruct struct {
	Chat      string `json:"chat"`
	Message   string `json:"message"`
	MessageID string `json:"messageId"`
}

type MessageSendStruct struct {
	Info               types.MessageInfo
	Message            *waE2E.Message
	MessageContextInfo *waE2E.ContextInfo
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

	result, err := m.messageRepository.GetMessageByID(data.Id)
	if err != nil {
		return nil, "", err
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
