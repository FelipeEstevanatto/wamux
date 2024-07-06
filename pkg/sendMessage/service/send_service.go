package send_service

import (
	"context"
	"errors"
	"time"

	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	"github.com/Zapbox-API/evolution-go/pkg/utils"
	whatsmeow_service "github.com/Zapbox-API/evolution-go/pkg/whatsmeow/service"
	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

type SendService interface {
	SendText(data *TextStruct, instance *instance_model.Instance) (string, string, error)
}

type sendService struct {
	clientPointer    map[int]whatsmeow_service.ClientInfo
	whatsmeowService whatsmeow_service.WhatsmeowService
}

type TextStruct struct {
	Phone       string
	Body        string
	Id          string
	ContextInfo waE2E.ContextInfo
}

func validateMessageFields(phone string, stanzaid *string, participant *string) (types.JID, error) {

	recipient, ok := utils.ParseJID(phone)
	if !ok {
		return types.NewJID("", types.DefaultUserServer), errors.New("Could not parse Phone")
	}

	if stanzaid != nil {
		if participant == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("Missing Participant in ContextInfo")
		}
	}

	if participant != nil {
		if stanzaid == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("Missing StanzaId in ContextInfo")
		}
	}

	return recipient, nil
}

func (s *sendService) SendText(data *TextStruct, instance *instance_model.Instance) (string, string, error) {
	if s.clientPointer[instance.Id].WAClient == nil {
		return "", "", errors.New("no session found")
	}

	var ts time.Time

	msgId := s.clientPointer[instance.Id].WAClient.GenerateMessageID()

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &data.Body,
		},
	}

	if data.ContextInfo.StanzaID != nil {
		msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{
			StanzaID:      proto.String(*data.ContextInfo.StanzaID),
			Participant:   proto.String(*data.ContextInfo.Participant),
			QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
		}
	}

	recipient, err := validateMessageFields(data.Phone, data.ContextInfo.StanzaID, data.ContextInfo.Participant)
	if err != nil {
		logger.LogError("Error validating message fields: %v", err)
		return "", "", err
	}

	_, err = s.clientPointer[instance.Id].WAClient.SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{
		ID: msgId,
	})
	if err != nil {
		return "", "", err
	}

	logger.LogInfo("Message sent to %s", data.Phone)

	return msgId, ts.String(), nil
}

func NewSendService(
	clientPointer map[int]whatsmeow_service.ClientInfo,
	whatsmeowService whatsmeow_service.WhatsmeowService,
) SendService {
	return &sendService{
		clientPointer:    clientPointer,
		whatsmeowService: whatsmeowService,
	}
}
