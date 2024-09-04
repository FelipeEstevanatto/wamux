package chat_service

import (
	"errors"
	"time"

	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	"github.com/Zapbox-API/evolution-go/pkg/utils"
	whatsmeow_service "github.com/Zapbox-API/evolution-go/pkg/whatsmeow/service"
	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow/appstate"
)

type ChatService interface {
	ChatPin(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatUnpin(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatArchive(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatMute(data *BodyStruct, instance *instance_model.Instance) (string, error)
}

type chatService struct {
	clientPointer map[string]whatsmeow_service.ClientInfo
}

type BodyStruct struct {
	Chat string `json:"chat"`
}

func (c *chatService) ChatPin(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	if c.clientPointer[instance.Id].WAClient == nil {
		return "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", errors.New("invalid phone number")
	}

	err := c.clientPointer[instance.Id].WAClient.SendAppState(appstate.BuildPin(recipient, true))
	if err != nil {
		logger.LogError("error pin chat: %v", err)
		return "", err
	}

	return ts.String(), nil
}

func (c *chatService) ChatUnpin(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	if c.clientPointer[instance.Id].WAClient == nil {
		return "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", errors.New("invalid phone number")
	}

	err := c.clientPointer[instance.Id].WAClient.SendAppState(appstate.BuildPin(recipient, false))
	if err != nil {
		logger.LogError("error unpin chat: %v", err)
		return "", err
	}

	return ts.String(), nil
}

func (c *chatService) ChatArchive(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	if c.clientPointer[instance.Id].WAClient == nil {
		return "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", errors.New("invalid phone number")
	}

	err := c.clientPointer[instance.Id].WAClient.SendAppState(appstate.BuildArchive(recipient, true, time.Time{}, nil))
	if err != nil {
		logger.LogError("error archive chat: %v", err)
		return "", err
	}

	return ts.String(), nil
}

func (c *chatService) ChatMute(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	if c.clientPointer[instance.Id].WAClient == nil {
		return "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", errors.New("invalid phone number")
	}

	err := c.clientPointer[instance.Id].WAClient.SendAppState(appstate.BuildMute(recipient, true, 1*time.Hour))
	if err != nil {
		logger.LogError("error mute chat: %v", err)
		return "", err
	}

	return ts.String(), nil
}

func NewChatService(
	clientPointer map[string]whatsmeow_service.ClientInfo,
) ChatService {
	return &chatService{
		clientPointer: clientPointer,
	}
}
