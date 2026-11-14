package chat_service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/safemap"

	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	message_repository "github.com/felipeestevanatto/wamux/pkg/message/repository"
	"github.com/felipeestevanatto/wamux/pkg/utils"
	whatsmeow_service "github.com/felipeestevanatto/wamux/pkg/whatsmeow/service"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type ChatService interface {
	ChatPin(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatUnpin(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatArchive(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatUnarchive(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatMarkUnread(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatMute(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatUnmute(data *BodyStruct, instance *instance_model.Instance) (string, error)
	SetEphemeralExpiration(data *EphemeralStruct, instance *instance_model.Instance) error
	HistorySyncRequest(ctx context.Context, data *HistorySyncRequestStruct, instance *instance_model.Instance) (*whatsmeow.SendResponse, error)
}

type chatService struct {
	clientPointer     *safemap.Map[*whatsmeow.Client]
	whatsmeowService  whatsmeow_service.WhatsmeowService
	messageRepository message_repository.MessageRepository
	loggerWrapper     *logger_wrapper.LoggerManager
}

type BodyStruct struct {
	Chat string `json:"chat" example:"5511999999999@s.whatsapp.net"`
}

// EphemeralStruct is the body of POST /chat/ephemeral.
type EphemeralStruct struct {
	Chat string `json:"chat" example:"5511999999999@s.whatsapp.net"`
	// Expiration is the disappearing-messages timer in SECONDS (0 disables it).
	// Official clients use 0, 86400 (24h), 604800 (7d) or 7776000 (90d).
	Expiration int64 `json:"expiration" example:"86400"`
}

// SetEphemeralExpiration sets the disappearing-messages timer for a chat and
// remembers it, so subsequent outgoing messages carry ContextInfo.Expiration
// and the recipient does not warn that the message will not disappear.
func (c *chatService) SetEphemeralExpiration(data *EphemeralStruct, instance *instance_model.Instance) error {
	if data.Expiration < 0 {
		return errors.New("expiration must be >= 0 seconds")
	}

	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}

	chat, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Invalid chat for ephemeral setting: %s", instance.Id, data.Chat)
		return errors.New("invalid chat")
	}
	chat = utils.CanonicalJID(chat)

	if err := client.SetDisappearingTimer(context.Background(), chat, time.Duration(data.Expiration)*time.Second, time.Now()); err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error setting disappearing timer: %v", instance.Id, err)
		return err
	}

	// Remember it locally too: the timer-change notification is not echoed back
	// to the sending device, so the cache would otherwise stay empty.
	whatsmeow_service.SetCachedChatEphemeral(instance.Id, chat, uint32(data.Expiration))
	c.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Disappearing timer set to %ds for %s", instance.Id, data.Expiration, chat.String())
	return nil
}

// HistorySyncRequestStruct asks the phone to resend history around a message.
// `messageInfo` is the original shape. `chat` + `timestamp` (Unix milliseconds)
// is the simpler timestamp-only form for a group or direct chat.
type HistorySyncRequestStruct struct {
	MessageInfo *types.MessageInfo `json:"messageInfo,omitempty"`
	Chat        string             `json:"chat,omitempty" example:"123456789@g.us"`
	Timestamp   int64              `json:"timestamp,omitempty" example:"1763000000000"`
	Count       int                `json:"count" example:"50"`
}

const (
	defaultHistorySyncCount = 50
	maxHistorySyncCount     = 100
)

var (
	// ErrInvalidHistorySync is a malformed request (missing chat or count).
	ErrInvalidHistorySync = errors.New("invalid history sync request")
	// ErrHistorySyncNotMember means the instance cannot ask for that group's
	// history because its account is not a participant.
	ErrHistorySyncNotMember = errors.New("the authenticated instance is not a participant of this group")
)

func historySyncCount(count int) int {
	if count <= 0 {
		return defaultHistorySyncCount
	}
	if count > maxHistorySyncCount {
		return maxHistorySyncCount
	}
	return count
}

func (c *chatService) ensureClientConnected(instanceId string) (*whatsmeow.Client, error) {
	client := c.clientPointer.Get(instanceId)
	c.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Checking client connection status - Client exists: %v", instanceId, client != nil)

	if client == nil {
		c.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] No client found, attempting to start new instance", instanceId)
		err := c.whatsmeowService.StartInstance(instanceId)
		if err != nil {
			c.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to start instance: %v", instanceId, err)
			return nil, errors.New("no active session found")
		}

		c.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Instance started, waiting 2 seconds...", instanceId)
		time.Sleep(2 * time.Second)

		client = c.clientPointer.Get(instanceId)
		c.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Checking new client - Exists: %v, Connected: %v",
			instanceId,
			client != nil,
			client != nil && client.IsConnected())

		if client == nil || !client.IsConnected() {
			c.loggerWrapper.GetLogger(instanceId).LogError("[%s] New client validation failed - Exists: %v, Connected: %v",
				instanceId,
				client != nil,
				client != nil && client.IsConnected())
			return nil, errors.New("no active session found")
		}
	} else if !client.IsConnected() {
		c.loggerWrapper.GetLogger(instanceId).LogError("[%s] Existing client is disconnected - Connected status: %v",
			instanceId,
			client.IsConnected())
		return nil, errors.New("client disconnected")
	}

	c.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Client successfully validated - Connected: %v", instanceId, client.IsConnected())
	return client, nil
}

func (c *chatService) ChatPin(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	err = client.SendAppState(context.Background(), appstate.BuildPin(recipient, true))
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error pin chat: %v", instance.Id, err)
		return "", err
	}
	c.setChatState(instance.Id, recipient, message_model.ChatStatePatch{Pinned: proto.Bool(true)})

	return ts.String(), nil
}

func (c *chatService) ChatUnpin(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	err = client.SendAppState(context.Background(), appstate.BuildPin(recipient, false))
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error unpin chat: %v", instance.Id, err)
		return "", err
	}
	c.setChatState(instance.Id, recipient, message_model.ChatStatePatch{Pinned: proto.Bool(false)})

	return ts.String(), nil
}

func (c *chatService) ChatArchive(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	err = client.SendAppState(context.Background(), appstate.BuildArchive(recipient, true, time.Time{}, nil))
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error archive chat: %v", instance.Id, err)
		return "", err
	}
	c.setChatState(instance.Id, recipient, message_model.ChatStatePatch{Archived: proto.Bool(true)})

	return ts.String(), nil
}

func (c *chatService) ChatUnarchive(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	err = client.SendAppState(context.Background(), appstate.BuildArchive(recipient, false, time.Time{}, nil))
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error unarchive chat: %v", instance.Id, err)
		return "", err
	}
	c.setChatState(instance.Id, recipient, message_model.ChatStatePatch{Archived: proto.Bool(false)})

	return ts.String(), nil
}

// ChatMarkUnread shows the chat as unread on the account's devices. WhatsApp applies
// the change up to a message, so the newest stored one is sent when there is one.
func (c *chatService) ChatMarkUnread(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}
	// The JID is the app-state index, which has no "+". See utils.CanonicalJID.
	recipient = utils.CanonicalJID(recipient)

	lastTimestamp, lastKey := c.lastMessageOf(instance.Id, recipient)
	err = client.SendAppState(context.Background(), appstate.BuildMarkChatAsRead(recipient, false, lastTimestamp, lastKey))
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error marking chat unread: %v", instance.Id, err)
		return "", err
	}
	c.setChatState(instance.Id, recipient, message_model.ChatStatePatch{MarkedUnread: proto.Bool(true)})

	return time.Now().String(), nil
}

// lastMessageOf is the newest stored message of a chat, in app-state form. Without
// stored history it is empty, which WhatsApp accepts.
func (c *chatService) lastMessageOf(instanceId string, chat types.JID) (time.Time, *waCommon.MessageKey) {
	if c.messageRepository == nil {
		return time.Time{}, nil
	}
	rows, err := c.messageRepository.ListMessages(instanceId, chat.String(), "", 1)
	if err != nil || len(rows) == 0 {
		return time.Time{}, nil
	}
	at, err := time.Parse("2006-01-02 15:04:05", rows[0].Timestamp)
	if err != nil {
		return time.Time{}, nil
	}
	key := &waCommon.MessageKey{
		RemoteJID: proto.String(chat.String()),
		FromMe:    proto.Bool(rows[0].IsFromMe),
		ID:        proto.String(rows[0].MessageID),
	}
	if chat.Server == types.GroupServer && !rows[0].IsFromMe {
		key.Participant = proto.String(rows[0].SenderJid)
	}
	return at, key
}

func (c *chatService) ChatMute(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	err = client.SendAppState(context.Background(), appstate.BuildMute(recipient, true, 1*time.Hour))
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error mute chat: %v", instance.Id, err)
		return "", err
	}
	c.setChatState(instance.Id, recipient, message_model.ChatStatePatch{MutedUntil: proto.Int64(time.Now().Add(time.Hour).Unix())})

	return ts.String(), nil
}

func (c *chatService) ChatUnmute(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return "", err
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	err = client.SendAppState(context.Background(), appstate.BuildMute(recipient, false, 0*time.Hour))
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error unmute chat: %v", instance.Id, err)
		return "", err
	}
	c.setChatState(instance.Id, recipient, message_model.ChatStatePatch{MutedUntil: proto.Int64(0)})

	return ts.String(), nil
}

func (c *chatService) HistorySyncRequest(ctx context.Context, data *HistorySyncRequestStruct, instance *instance_model.Instance) (*whatsmeow.SendResponse, error) {
	if data == nil {
		return nil, ErrInvalidHistorySync
	}

	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	var messageInfo types.MessageInfo
	if data.MessageInfo != nil {
		messageInfo = *data.MessageInfo
		if messageInfo.Chat.IsEmpty() {
			return nil, ErrInvalidHistorySync
		}
	} else {
		if strings.TrimSpace(data.Chat) == "" {
			return nil, ErrInvalidHistorySync
		}
		chat, ok := utils.ParseJID(data.Chat)
		if !ok {
			return nil, ErrInvalidHistorySync
		}
		messageInfo.Chat = chat
	}

	messageInfo.IsGroup = messageInfo.Chat.Server == types.GroupServer
	if messageInfo.Timestamp.IsZero() {
		if data.Timestamp > 0 {
			messageInfo.Timestamp = time.UnixMilli(data.Timestamp)
		} else {
			messageInfo.Timestamp = time.Now()
		}
	}

	// Asking for history from a group the account is not in gets an empty
	// response at best and abuses the phone's history endpoint at worst.
	if messageInfo.IsGroup {
		info, err := client.GetGroupInfo(ctx, messageInfo.Chat.ToNonAD())
		if err != nil {
			c.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] history sync group lookup failed: %v", instance.Id, err)
			return nil, err
		}
		if !historySyncMember(client, info) {
			return nil, ErrHistorySyncNotMember
		}
	}

	histRequest := client.BuildHistorySyncRequest(&messageInfo, historySyncCount(data.Count))

	// On-demand history-sync requests must be sent to our OWN JID as a peer message, not to the
	// contact. SendPeerMessage does exactly that (getOwnID().ToNonAD() + Peer:true). Sending it to
	// messageInfo.Chat (the contact) fails with "no signal session established" and never returns
	// history. whatsmeow's BuildHistorySyncRequest doc: "The built message can be sent using
	// Client.SendPeerMessage." The target chat/cursor is already encoded inside histRequest.
	// Uses the caller's context so the request can be cancelled / time-bounded.
	res, err := client.SendPeerMessage(ctx, histRequest)
	if err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error history sync request: %v", instance.Id, err)
		return nil, err
	}

	return &res, nil
}

// historySyncMember reports whether our account appears in the group roster.
// The roster may carry the phone JID, the LID, or both for the same person.
func historySyncMember(client *whatsmeow.Client, info *types.GroupInfo) bool {
	if client == nil || info == nil || client.Store.ID == nil {
		return false
	}
	own := client.Store.ID
	for _, participant := range info.Participants {
		for _, jid := range []types.JID{participant.JID, participant.PhoneNumber, participant.LID} {
			if !jid.IsEmpty() && jid.User == own.User {
				return true
			}
		}
	}
	return false
}

func (c *chatService) setChatState(instanceId string, chat types.JID, patch message_model.ChatStatePatch) {
	if c.messageRepository == nil || patch.Empty() {
		return
	}
	canonical := utils.CanonicalJID(chat).ToNonAD().String()
	if err := c.messageRepository.SetChatState(instanceId, canonical, patch); err != nil {
		c.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Failed to store chat state for %s: %v", instanceId, canonical, err)
	}
}

func NewChatService(
	clientPointer *safemap.Map[*whatsmeow.Client],
	whatsmeowService whatsmeow_service.WhatsmeowService,
	messageRepository message_repository.MessageRepository,
	loggerWrapper *logger_wrapper.LoggerManager,
) ChatService {
	return &chatService{
		clientPointer:     clientPointer,
		whatsmeowService:  whatsmeowService,
		messageRepository: messageRepository,
		loggerWrapper:     loggerWrapper,
	}
}
