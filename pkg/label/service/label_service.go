package label_service

import (
	"errors"

	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	"github.com/Zapbox-API/evolution-go/pkg/utils"
	whatsmeow_service "github.com/Zapbox-API/evolution-go/pkg/whatsmeow/service"
	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow/appstate"
)

type LabelService interface {
	ChatLabel(data *ChatLabelStruct, instance *instance_model.Instance) error
	MessageLabel(data *MessageLabelStruct, instance *instance_model.Instance) error
	EditLabel(data *EditLabelStruct, instance *instance_model.Instance) error
	ChatUnlabel(data *ChatLabelStruct, instance *instance_model.Instance) error
	MessageUnlabel(data *MessageLabelStruct, instance *instance_model.Instance) error
}

type labelService struct {
	clientPointer map[string]whatsmeow_service.ClientInfo
}

type ChatLabelStruct struct {
	JID     string `json:"jid"`
	LabelID string `json:"labelId"`
}

type MessageLabelStruct struct {
	JID       string `json:"jid"`
	LabelID   string `json:"labelId"`
	MessageID string `json:"messageId"`
}

type EditLabelStruct struct {
	LabelID string `json:"labelId"`
	Name    string `json:"name"`
	Color   int    `json:"color"`
	Action  bool   `json:"action"`
}

func (l *labelService) ChatLabel(data *ChatLabelStruct, instance *instance_model.Instance) error {
	if l.clientPointer[instance.Id].WAClient == nil {
		return errors.New("no session found")
	}

	jid, ok := utils.ParseJID(data.JID)
	if !ok {
		logger.LogError("error parse community jid")
		return errors.New("error parse community jid")
	}

	err := l.clientPointer[instance.Id].WAClient.SendAppState(appstate.BuildLabelChat(
		jid,
		data.LabelID,
		true,
	))
	if err != nil {
		logger.LogError("error label chat: %v", err)
		return err
	}

	return nil
}

func (l *labelService) MessageLabel(data *MessageLabelStruct, instance *instance_model.Instance) error {
	if l.clientPointer[instance.Id].WAClient == nil {
		return errors.New("no session found")
	}

	jid, ok := utils.ParseJID(data.JID)
	if !ok {
		logger.LogError("error parse community jid")
		return errors.New("error parse community jid")
	}

	err := l.clientPointer[instance.Id].WAClient.SendAppState(appstate.BuildLabelMessage(
		jid,
		data.LabelID,
		data.MessageID,
		true,
	))
	if err != nil {
		logger.LogError("error label message: %v", err)
		return err
	}

	return nil
}

func (l *labelService) EditLabel(data *EditLabelStruct, instance *instance_model.Instance) error {
	if l.clientPointer[instance.Id].WAClient == nil {
		return errors.New("no session found")
	}

	err := l.clientPointer[instance.Id].WAClient.SendAppState(appstate.BuildLabelEdit(
		data.LabelID,
		data.Name,
		int32(data.Color),
		true,
	))
	if err != nil {
		logger.LogError("error label message: %v", err)
		return err
	}

	return nil
}

func (l *labelService) ChatUnlabel(data *ChatLabelStruct, instance *instance_model.Instance) error {
	if l.clientPointer[instance.Id].WAClient == nil {
		return errors.New("no session found")
	}

	jid, ok := utils.ParseJID(data.JID)
	if !ok {
		logger.LogError("error parse community jid")
		return errors.New("error parse community jid")
	}

	err := l.clientPointer[instance.Id].WAClient.SendAppState(appstate.BuildLabelChat(
		jid,
		data.LabelID,
		false,
	))
	if err != nil {
		logger.LogError("error label chat: %v", err)
		return err
	}

	return nil
}

func (l *labelService) MessageUnlabel(data *MessageLabelStruct, instance *instance_model.Instance) error {
	if l.clientPointer[instance.Id].WAClient == nil {
		return errors.New("no session found")
	}

	jid, ok := utils.ParseJID(data.JID)
	if !ok {
		logger.LogError("error parse community jid")
		return errors.New("error parse community jid")
	}

	err := l.clientPointer[instance.Id].WAClient.SendAppState(appstate.BuildLabelMessage(
		jid,
		data.LabelID,
		data.MessageID,
		false,
	))
	if err != nil {
		logger.LogError("error label message: %v", err)
		return err
	}

	return nil
}

func NewLabelService(
	clientPointer map[string]whatsmeow_service.ClientInfo,
) LabelService {
	return &labelService{
		clientPointer: clientPointer,
	}
}
