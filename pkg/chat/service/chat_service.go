package chat_service

import (
	"context"
	"errors"
	"time"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"github.com/EvolutionAPI/evolution-go/pkg/utils"
	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

type ChatService interface {
	ChatPin(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatUnpin(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatArchive(data *BodyStruct, instance *instance_model.Instance) (string, error)
	ChatMute(data *BodyStruct, instance *instance_model.Instance) (string, error)
	HistorySyncRequest(data *HistorySyncRequestStruct, instance *instance_model.Instance) (string, error)
}

type chatService struct {
	clientPointer map[string]*whatsmeow.Client
}

type BodyStruct struct {
	Chat string `json:"chat"`
}

type HistorySyncRequestStruct struct {
	MessageInfo *types.MessageInfo `json:"messageInfo"`
	Count       int                `json:"count"`
}

func (c *chatService) ChatPin(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	if c.clientPointer[instance.Id] == nil {
		return "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", errors.New("invalid phone number")
	}

	err := c.clientPointer[instance.Id].SendAppState(appstate.BuildPin(recipient, true))
	if err != nil {
		logger.LogError("error pin chat: %v", err)
		return "", err
	}

	return ts.String(), nil
}

func (c *chatService) ChatUnpin(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	if c.clientPointer[instance.Id] == nil {
		return "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", errors.New("invalid phone number")
	}

	err := c.clientPointer[instance.Id].SendAppState(appstate.BuildPin(recipient, false))
	if err != nil {
		logger.LogError("error unpin chat: %v", err)
		return "", err
	}

	return ts.String(), nil
}

func (c *chatService) ChatArchive(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	if c.clientPointer[instance.Id] == nil {
		return "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", errors.New("invalid phone number")
	}

	err := c.clientPointer[instance.Id].SendAppState(appstate.BuildArchive(recipient, true, time.Time{}, nil))
	if err != nil {
		logger.LogError("error archive chat: %v", err)
		return "", err
	}

	return ts.String(), nil
}

func (c *chatService) ChatMute(data *BodyStruct, instance *instance_model.Instance) (string, error) {
	if c.clientPointer[instance.Id] == nil {
		return "", errors.New("no session found")
	}

	var ts time.Time

	recipient, ok := utils.ParseJID(data.Chat)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", errors.New("invalid phone number")
	}

	err := c.clientPointer[instance.Id].SendAppState(appstate.BuildMute(recipient, true, 1*time.Hour))
	if err != nil {
		logger.LogError("error mute chat: %v", err)
		return "", err
	}

	return ts.String(), nil
}

func (c *chatService) HistorySyncRequest(data *HistorySyncRequestStruct, instance *instance_model.Instance) (string, error) {
	if c.clientPointer[instance.Id] == nil {
		return "", errors.New("no session found")
	}

	messageInfo := types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     data.MessageInfo.Chat,
			IsFromMe: data.MessageInfo.IsFromMe,
		},
		ID:        data.MessageInfo.ID,
		Timestamp: data.MessageInfo.Timestamp,
	}

	histRequest := c.clientPointer[instance.Id].BuildHistorySyncRequest(&messageInfo, data.Count)

	res, err := c.clientPointer[instance.Id].SendMessage(context.Background(), messageInfo.Chat, histRequest, whatsmeow.SendRequestExtra{Peer: true})
	if err != nil {
		logger.LogError("error history sync request: %v", err)
		return "", err
	}

	return res.ID, nil
}

func NewChatService(
	clientPointer map[string]*whatsmeow.Client,
) ChatService {
	return &chatService{
		clientPointer: clientPointer,
	}
}
