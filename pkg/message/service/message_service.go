package message_service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	message_model "github.com/Zapbox-API/evolution-go/pkg/message/model"
	message_repository "github.com/Zapbox-API/evolution-go/pkg/message/repository"
	"github.com/Zapbox-API/evolution-go/pkg/utils"
	whatsmeow_service "github.com/Zapbox-API/evolution-go/pkg/whatsmeow/service"
	"github.com/gomessguii/logger"
	"github.com/vincent-petithory/dataurl"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type MessageService interface {
	React(data *ReactStruct, instance *instance_model.Instance) (string, string, error)
	ChatPresence(data *ChatPresenceStruct, instance *instance_model.Instance) (string, error)
	MarkRead(data *MarkReadStruct, instance *instance_model.Instance) (string, error)
	DownloadImage(data *DownloadImageStruct, instance *instance_model.Instance, request *http.Request) (*dataurl.DataURL, string, error)
	GetMessageStatus(data *MessageStatusStruct, instance *instance_model.Instance) (*message_model.Message, string, error)
	DeleteMessageEveryone(data *MessageStruct, instance *instance_model.Instance) (string, string, error)
	EditMessage(data *EditMessageStruct, instance *instance_model.Instance) (string, string, error)
}

type messageService struct {
	clientPointer     map[string]whatsmeow_service.ClientInfo
	messageRepository message_repository.MessageRepository
}

type ReactStruct struct {
	Phone string `json:"phone"`
	Body  string `json:"body"`
	Id    string `json:"id"`
}

type ChatPresenceStruct struct {
	Phone string `json:"phone"`
	State string `json:"state"`
	Media string `json:"media"`
}

type MarkReadStruct struct {
	Id     []string  `json:"id"`
	Chat   types.JID `json:"chat"`
	Sender types.JID `json:"sender"`
}

type DownloadImageStruct struct {
	Url           string `json:"url"`
	DirectPath    string `json:"directPath"`
	MediaKey      []byte `json:"mediaKey"`
	Mimetype      string `json:"mimetype"`
	FileEncSHA256 []byte `json:"fileEncSHA256"`
	FileSHA256    []byte `json:"fileSHA256"`
	FileLength    uint64 `json:"fileLength"`
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

func (m *messageService) React(data *ReactStruct, instance *instance_model.Instance) (string, string, error) {
	if m.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	msgId := ""
	var ts time.Time

	recipient, ok := utils.ParseJID(data.Phone)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", "", errors.New("invalid phone number")
	}

	if data.Id == "" {
		logger.LogError("Missing Id in Payload")
		return "", "", errors.New("missing id in payload")
	} else {
		msgId = data.Id
	}

	fromMe := false
	if strings.HasPrefix(msgId, "me:") {
		fromMe = true
		msgId = msgId[len("me:"):]
	}
	reaction := data.Body
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

	_, err := m.clientPointer[instance.Id].WAClient.SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{
		ID: msgId,
	})
	if err != nil {
		return "", "", err
	}

	return msgId, ts.String(), nil
}

func (m *messageService) ChatPresence(data *ChatPresenceStruct, instance *instance_model.Instance) (string, error) {
	if m.clientPointer[instance.Id].WAClient == nil {
		return "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Phone)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", errors.New("invalid phone number")
	}

	err := m.clientPointer[instance.Id].WAClient.SendChatPresence(recipient, types.ChatPresence(data.State), types.ChatPresenceMedia(data.Media))
	if err != nil {
		return "", err
	}

	logger.LogInfo("Message sent to %s", data.Phone)

	return ts.String(), nil
}

func (m *messageService) MarkRead(data *MarkReadStruct, instance *instance_model.Instance) (string, error) {
	if m.clientPointer[instance.Id].WAClient == nil {
		return "", errors.New("no session found")
	}

	var ts time.Time

	err := m.clientPointer[instance.Id].WAClient.MarkRead(data.Id, time.Now(), data.Chat, data.Sender)
	if err != nil {
		return "", err
	}

	return ts.String(), nil
}

func (m *messageService) DownloadImage(data *DownloadImageStruct, instance *instance_model.Instance, request *http.Request) (*dataurl.DataURL, string, error) {
	if m.clientPointer[instance.Id].WAClient == nil {
		return nil, "", errors.New("no session found")
	}

	var ts time.Time

	mimetype := ""
	var imgData []byte

	userDirectory := fmt.Sprintf(`files/user_%s`, instance.Id)
	_, err := os.Stat(userDirectory)
	if os.IsNotExist(err) {
		errDir := os.MkdirAll(userDirectory, 0751)
		if errDir != nil {
			logger.LogError("Could not create user directory (%s)", userDirectory)
			return nil, "", errDir
		}
	}

	decoder := json.NewDecoder(request.Body)
	var t DownloadImageStruct
	err = decoder.Decode(&t)
	if err != nil {
		logger.LogError("invalid payload")
		return nil, "", err
	}

	msg := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		URL:           proto.String(t.Url),
		DirectPath:    proto.String(t.DirectPath),
		MediaKey:      t.MediaKey,
		Mimetype:      proto.String(t.Mimetype),
		FileEncSHA256: t.FileEncSHA256,
		FileSHA256:    t.FileSHA256,
		FileLength:    &t.FileLength,
	}}

	img := msg.GetImageMessage()

	if img != nil {
		imgData, err = m.clientPointer[instance.Id].WAClient.Download(img)
		if err != nil {
			logger.LogError("Failed to download image")
			msg := fmt.Sprintf("Failed to download image %v", err)
			return nil, "", errors.New(msg)
		}
		mimetype = img.GetMimetype()
	}

	dataURL := dataurl.New(imgData, mimetype)

	return dataURL, ts.String(), nil
}

func (m *messageService) GetMessageStatus(data *MessageStatusStruct, instance *instance_model.Instance) (*message_model.Message, string, error) {
	if m.clientPointer[instance.Id].WAClient == nil {
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
	if m.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", "", errors.New("invalid phone number")
	}

	resp, err := m.clientPointer[instance.Id].WAClient.SendMessage(
		context.Background(),
		recipient, m.clientPointer[instance.Id].WAClient.BuildRevoke(recipient, types.EmptyJID, data.MessageID))
	if err != nil {
		logger.LogError("error revoking message: %v", err)
		return "", "", err
	}

	response := resp.ID

	return response, ts.String(), nil
}

func (m *messageService) EditMessage(data *EditMessageStruct, instance *instance_model.Instance) (string, string, error) {
	if m.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", "", errors.New("invalid phone number")
	}

	resp, err := m.clientPointer[instance.Id].WAClient.SendMessage(
		context.Background(),
		recipient,
		m.clientPointer[instance.Id].WAClient.BuildEdit(
			recipient,
			data.MessageID,
			&waE2E.Message{
				Conversation: proto.String(data.Message),
			}))
	if err != nil {
		logger.LogError("error revoking message: %v", err)
		return "", "", err
	}

	response := resp.ID

	return response, ts.String(), nil
}

func NewMessageService(
	clientPointer map[string]whatsmeow_service.ClientInfo,
	messageRepository message_repository.MessageRepository,
) MessageService {
	return &messageService{
		clientPointer:     clientPointer,
		messageRepository: messageRepository,
	}
}
