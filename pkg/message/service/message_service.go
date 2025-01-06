package message_service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	message_model "github.com/EvolutionAPI/evolution-go/pkg/message/model"
	message_repository "github.com/EvolutionAPI/evolution-go/pkg/message/repository"
	"github.com/EvolutionAPI/evolution-go/pkg/utils"
	"github.com/gomessguii/logger"
	"github.com/vincent-petithory/dataurl"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type MessageService interface {
	React(data *ReactStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	ChatPresence(data *ChatPresenceStruct, instance *instance_model.Instance) (string, error)
	MarkRead(data *MarkReadStruct, instance *instance_model.Instance) (string, error)
	DownloadMedia(data *DownloadMediaStruct, instance *instance_model.Instance, request *http.Request) (*dataurl.DataURL, string, error)
	GetMessageStatus(data *MessageStatusStruct, instance *instance_model.Instance) (*message_model.Message, string, error)
	DeleteMessageEveryone(data *MessageStruct, instance *instance_model.Instance) (string, string, error)
	EditMessage(data *EditMessageStruct, instance *instance_model.Instance) (string, string, error)
}

type messageService struct {
	clientPointer     map[string]*whatsmeow.Client
	messageRepository message_repository.MessageRepository
}

type ReactStruct struct {
	Number   string `json:"number"`
	Reaction string `json:"reaction"`
	Id       string `json:"id"`
}

type ChatPresenceStruct struct {
	Number  string `json:"number"`
	State   string `json:"state"`
	IsAudio bool   `json:"isAudio"`
}

type MarkReadStruct struct {
	Id     []string `json:"id"`
	Number string   `json:"number"`
}

type DownloadMediaStruct struct {
	Message *waE2E.Message `json:"message"`
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

func (m *messageService) React(data *ReactStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	if m.clientPointer[instance.Id] == nil {
		return nil, errors.New("no session found")
	}

	msgId := ""

	recipient, ok := utils.ParseJID(data.Number)
	if !ok {
		logger.LogError("[%s] Error validating message fields", instance.Id)
		return nil, errors.New("invalid phone number")
	}

	if data.Id == "" {
		logger.LogError("[%s] Missing Id in Payload", instance.Id)
		return nil, errors.New("missing id in payload")
	} else {
		msgId = data.Id
	}

	fromMe := false
	if strings.HasPrefix(msgId, "me:") {
		fromMe = true
		msgId = msgId[len("me:"):]
	}
	reaction := data.Reaction
	if reaction == "remove" {
		reaction = ""
	}

	msg := &waE2E.Message{
		ReactionMessage: &waE2E.ReactionMessage{
			Key: &waCommon.MessageKey{
				RemoteJID: proto.String(recipient.String()),
				FromMe:    proto.Bool(fromMe),
				ID:        proto.String(msgId),
			},
			Text:              proto.String(reaction),
			GroupingKey:       proto.String(reaction),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	}

	response, err := m.clientPointer[instance.Id].SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{
		ID: msgId,
	})
	if err != nil {
		return nil, err
	}

	isGroup := strings.Contains(data.Number, "@g.us")
	messageType := "ReactionMessage"

	messageInfo := types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     recipient,
			Sender:   *m.clientPointer[instance.Id].Store.ID,
			IsFromMe: true,
			IsGroup:  isGroup,
		},
		ID:        msgId,
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

func (m *messageService) ChatPresence(data *ChatPresenceStruct, instance *instance_model.Instance) (string, error) {
	if m.clientPointer[instance.Id] == nil {
		return "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Number)
	if !ok {
		logger.LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	media := ""

	if data.IsAudio {
		media = "audio"
	}

	err := m.clientPointer[instance.Id].SendChatPresence(recipient, types.ChatPresence(data.State), types.ChatPresenceMedia(media))
	if err != nil {
		return "", err
	}

	logger.LogInfo("Message sent to %s", data.Number)

	return ts.String(), nil
}

func (m *messageService) MarkRead(data *MarkReadStruct, instance *instance_model.Instance) (string, error) {
	if m.clientPointer[instance.Id] == nil {
		return "", errors.New("no session found")
	}

	var ts time.Time

	jid, ok := utils.ParseJID(data.Number)
	if !ok {
		logger.LogError("[%s] Error validating message fields", instance.Id)
		return "", errors.New("invalid phone number")
	}

	err := m.clientPointer[instance.Id].MarkRead(data.Id, time.Now(), jid, jid)
	if err != nil {
		logger.LogError("[%s] error marking message as read: %v", instance.Id, err)
		return "", errors.New("error marking message as read")
	}

	return ts.String(), nil
}

func (m *messageService) DownloadMedia(data *DownloadMediaStruct, instance *instance_model.Instance, request *http.Request) (*dataurl.DataURL, string, error) {
	if m.clientPointer[instance.Id] == nil {
		return nil, "", errors.New("no session found")
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
	_, err := os.Stat(userDirectory)
	if os.IsNotExist(err) {
		errDir := os.MkdirAll(userDirectory, 0751)
		if errDir != nil {
			logger.LogError("[%s] Could not create user directory (%s)", instance.Id, userDirectory)
			return nil, "", errDir
		}
	}

	if img != nil {
		mediaData, err = m.clientPointer[instance.Id].Download(img)
		if err != nil {
			logger.LogError("[%s] Failed to download image", instance.Id)
			msg := fmt.Sprintf("Failed to download image %v", err)
			return nil, "", errors.New(msg)
		}
		mimetype = img.GetMimetype()
	}

	if audio != nil {
		mediaData, err = m.clientPointer[instance.Id].Download(audio)
		if err != nil {
			logger.LogError("[%s] Failed to download audio", instance.Id)
			msg := fmt.Sprintf("Failed to download audio %v", err)
			return nil, "", errors.New(msg)
		}
		mimetype = audio.GetMimetype()
	}

	if document != nil {
		mediaData, err = m.clientPointer[instance.Id].Download(document)
		if err != nil {
			logger.LogError("[%s] Failed to download document", instance.Id)
			msg := fmt.Sprintf("Failed to download document %v", err)
			return nil, "", errors.New(msg)
		}
		mimetype = document.GetMimetype()
	}

	if video != nil {
		mediaData, err = m.clientPointer[instance.Id].Download(video)
		if err != nil {
			logger.LogError("[%s] Failed to download video", instance.Id)
			msg := fmt.Sprintf("Failed to download video %v", err)
			return nil, "", errors.New(msg)
		}
		mimetype = video.GetMimetype()
	}

	if sticker != nil {
		mediaData, err = m.clientPointer[instance.Id].Download(sticker)
		if err != nil {
			logger.LogError("[%s] Failed to download sticker", instance.Id)
			msg := fmt.Sprintf("Failed to download sticker %v", err)
			return nil, "", errors.New(msg)
		}
		mimetype = sticker.GetMimetype()
	}

	dataURL := dataurl.New(mediaData, mimetype)

	return dataURL, ts.String(), nil
}

func (m *messageService) GetMessageStatus(data *MessageStatusStruct, instance *instance_model.Instance) (*message_model.Message, string, error) {
	if m.clientPointer[instance.Id] == nil {
		return nil, "", errors.New("no session found")
	}

	var ts time.Time

	result, err := m.messageRepository.GetMessageByID(data.Id)
	if err != nil {
		return nil, "", err
	}

	return result, ts.String(), nil
}

func (m *messageService) DeleteMessageEveryone(data *MessageStruct, instance *instance_model.Instance) (string, string, error) {
	if m.clientPointer[instance.Id] == nil {
		return "", "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		logger.LogError("[%s] Error validating message fields", instance.Id)
		return "", "", errors.New("invalid phone number")
	}

	logger.LogInfo("Revoking message %s from %s", data.MessageID, recipient)

	resp, err := m.clientPointer[instance.Id].SendMessage(
		context.Background(),
		recipient, m.clientPointer[instance.Id].BuildRevoke(recipient, types.EmptyJID, data.MessageID))
	if err != nil {
		logger.LogError("[%s] error revoking message: %v", instance.Id, err)
		return "", "", err
	}

	response := resp.ID

	return response, ts.String(), nil
}

func (m *messageService) EditMessage(data *EditMessageStruct, instance *instance_model.Instance) (string, string, error) {
	if m.clientPointer[instance.Id] == nil {
		return "", "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		logger.LogError("[%s] Error validating message fields", instance.Id)
		return "", "", errors.New("invalid phone number")
	}

	resp, err := m.clientPointer[instance.Id].SendMessage(
		context.Background(),
		recipient,
		m.clientPointer[instance.Id].BuildEdit(
			recipient,
			data.MessageID,
			&waE2E.Message{
				Conversation: proto.String(data.Message),
			}))
	if err != nil {
		logger.LogError("[%s] error revoking message: %v", instance.Id, err)
		return "", "", err
	}

	response := resp.ID

	return response, ts.String(), nil
}

func NewMessageService(
	clientPointer map[string]*whatsmeow.Client,
	messageRepository message_repository.MessageRepository,
) MessageService {
	return &messageService{
		clientPointer:     clientPointer,
		messageRepository: messageRepository,
	}
}
