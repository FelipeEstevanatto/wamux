package label_service

import (
	"errors"
	"time"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	label_model "github.com/EvolutionAPI/evolution-go/pkg/label/model"
	label_repository "github.com/EvolutionAPI/evolution-go/pkg/label/repository"
	"github.com/EvolutionAPI/evolution-go/pkg/utils"
	whatsmeow_service "github.com/EvolutionAPI/evolution-go/pkg/whatsmeow/service"
	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
)

type LabelService interface {
	ChatLabel(data *ChatLabelStruct, instance *instance_model.Instance) error
	MessageLabel(data *MessageLabelStruct, instance *instance_model.Instance) error
	EditLabel(data *EditLabelStruct, instance *instance_model.Instance) error
	ChatUnlabel(data *ChatLabelStruct, instance *instance_model.Instance) error
	MessageUnlabel(data *MessageLabelStruct, instance *instance_model.Instance) error
	GetLabels(instance *instance_model.Instance) ([]label_model.Label, error)
}

type labelService struct {
	clientPointer    map[string]*whatsmeow.Client
	whatsmeowService whatsmeow_service.WhatsmeowService
	labelRepository  label_repository.LabelRepository
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
	Deleted bool   `json:"deleted"`
}

func (l *labelService) ensureClientConnected(instanceId string) (*whatsmeow.Client, error) {
	client := l.clientPointer[instanceId]
	logger.LogInfo("[%s] Checking client connection status - Client exists: %v", instanceId, client != nil)

	if client == nil {
		logger.LogInfo("[%s] No client found, attempting to start new instance", instanceId)
		err := l.whatsmeowService.StartInstance(instanceId)
		if err != nil {
			logger.LogError("[%s] Failed to start instance: %v", instanceId, err)
			return nil, errors.New("no active session found")
		}

		logger.LogInfo("[%s] Instance started, waiting 2 seconds...", instanceId)
		time.Sleep(2 * time.Second)

		client = l.clientPointer[instanceId]
		logger.LogInfo("[%s] Checking new client - Exists: %v, Connected: %v",
			instanceId,
			client != nil,
			client != nil && client.IsConnected())

		if client == nil || !client.IsConnected() {
			logger.LogError("[%s] New client validation failed - Exists: %v, Connected: %v",
				instanceId,
				client != nil,
				client != nil && client.IsConnected())
			return nil, errors.New("no active session found")
		}
	} else if !client.IsConnected() {
		logger.LogError("[%s] Existing client is disconnected - Connected status: %v",
			instanceId,
			client.IsConnected())
		return nil, errors.New("client disconnected")
	}

	logger.LogInfo("[%s] Client successfully validated - Connected: %v", instanceId, client.IsConnected())
	return client, nil
}

func (l *labelService) ChatLabel(data *ChatLabelStruct, instance *instance_model.Instance) error {
	client, err := l.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}

	jid, ok := utils.ParseJID(data.JID)
	if !ok {
		logger.LogError("[%s] error parse community jid", instance.Id)
		return errors.New("error parse community jid")
	}

	err = client.SendAppState(appstate.BuildLabelChat(
		jid,
		data.LabelID,
		true,
	))
	if err != nil {
		logger.LogError("[%s] error label chat: %v", instance.Id, err)
		return err
	}

	return nil
}

func (l *labelService) MessageLabel(data *MessageLabelStruct, instance *instance_model.Instance) error {
	client, err := l.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}

	jid, ok := utils.ParseJID(data.JID)
	if !ok {
		logger.LogError("[%s] error parse community jid", instance.Id)
		return errors.New("error parse community jid")
	}

	err = client.SendAppState(appstate.BuildLabelMessage(
		jid,
		data.LabelID,
		data.MessageID,
		true,
	))
	if err != nil {
		logger.LogError("[%s] error label message: %v", instance.Id, err)
		return err
	}

	return nil
}

func (l *labelService) EditLabel(data *EditLabelStruct, instance *instance_model.Instance) error {
	client, err := l.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}

	err = client.SendAppState(appstate.BuildLabelEdit(
		data.LabelID,
		data.Name,
		int32(data.Color),
		data.Deleted,
	))
	if err != nil {
		logger.LogError("[%s] error label message: %v", instance.Id, err)
		return err
	}

	return nil
}

func (l *labelService) ChatUnlabel(data *ChatLabelStruct, instance *instance_model.Instance) error {
	client, err := l.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}

	jid, ok := utils.ParseJID(data.JID)
	if !ok {
		logger.LogError("[%s] error parse community jid", instance.Id)
		return errors.New("error parse community jid")
	}

	err = client.SendAppState(appstate.BuildLabelChat(
		jid,
		data.LabelID,
		false,
	))
	if err != nil {
		logger.LogError("[%s] error label chat: %v", instance.Id, err)
		return err
	}

	return nil
}

func (l *labelService) MessageUnlabel(data *MessageLabelStruct, instance *instance_model.Instance) error {
	client, err := l.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}

	jid, ok := utils.ParseJID(data.JID)
	if !ok {
		logger.LogError("[%s] error parse community jid", instance.Id)
		return errors.New("error parse community jid")
	}

	err = client.SendAppState(appstate.BuildLabelMessage(
		jid,
		data.LabelID,
		data.MessageID,
		false,
	))
	if err != nil {
		logger.LogError("[%s] error label message: %v", instance.Id, err)
		return err
	}

	return nil
}

func (l *labelService) GetLabels(instance *instance_model.Instance) ([]label_model.Label, error) {
	_, err := l.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	labels, err := l.labelRepository.GetAllLabelsByInstanceID(instance.Id)
	if err != nil {
		logger.LogError("[%s] error fetching labels from database: %v", instance.Id, err)
		return nil, err
	}

	return labels, nil
}

func NewLabelService(
	clientPointer map[string]*whatsmeow.Client,
	whatsmeowService whatsmeow_service.WhatsmeowService,
	labelRepository label_repository.LabelRepository,
) LabelService {
	return &labelService{
		clientPointer:    clientPointer,
		whatsmeowService: whatsmeowService,
		labelRepository:  labelRepository,
	}
}
