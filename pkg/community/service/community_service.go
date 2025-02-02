package community_service

import (
	"errors"
	"time"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"github.com/EvolutionAPI/evolution-go/pkg/utils"
	whatsmeow_service "github.com/EvolutionAPI/evolution-go/pkg/whatsmeow/service"
	"github.com/gin-gonic/gin"
	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type CommunityService interface {
	CreateCommunity(data *CreateCommunityStruct, instance *instance_model.Instance) (*types.GroupInfo, error)
	CommunityAdd(data *AddParticipantStruct, instance *instance_model.Instance) (gin.H, error)
	CommunityRemove(data *AddParticipantStruct, instance *instance_model.Instance) (gin.H, error)
}

type communityService struct {
	clientPointer    map[string]*whatsmeow.Client
	whatsmeowService whatsmeow_service.WhatsmeowService
}

type CreateCommunityStruct struct {
	CommunityName string `json:"communityName"`
}

type AddParticipantStruct struct {
	CommunityJID string   `json:"communityJid"`
	GroupJID     []string `json:"groupJid"`
}

func (c *communityService) ensureClientConnected(instanceId string) (*whatsmeow.Client, error) {
	client := c.clientPointer[instanceId]
	logger.LogInfo("[%s] Checking client connection status - Client exists: %v", instanceId, client != nil)

	if client == nil {
		logger.LogInfo("[%s] No client found, attempting to start new instance", instanceId)
		err := c.whatsmeowService.StartInstance(instanceId)
		if err != nil {
			logger.LogError("[%s] Failed to start instance: %v", instanceId, err)
			return nil, errors.New("no active session found")
		}

		logger.LogInfo("[%s] Instance started, waiting 2 seconds...", instanceId)
		time.Sleep(2 * time.Second)

		client = c.clientPointer[instanceId]
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

func (c *communityService) CreateCommunity(data *CreateCommunityStruct, instance *instance_model.Instance) (*types.GroupInfo, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	resp, err := client.CreateGroup(whatsmeow.ReqCreateGroup{
		Name: data.CommunityName,
		GroupParent: types.GroupParent{
			IsParent: true,
		},
	})
	if err != nil {
		logger.LogError("[%s] error create community: %v", instance.Id, err)
		return nil, err
	}

	return resp, nil
}

func (c *communityService) CommunityAdd(data *AddParticipantStruct, instance *instance_model.Instance) (gin.H, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	communityJID, ok := utils.ParseJID(data.CommunityJID)
	if !ok {
		logger.LogError("[%s] error parse community jid", instance.Id)
		return nil, errors.New("error parse community jid")
	}

	var successList []string
	var failedList []string

	for _, participant := range data.GroupJID {
		groupJID, _ := utils.ParseJID(participant)
		err := client.LinkGroup(communityJID, groupJID)
		if err != nil {
			logger.LogError("[%s] error link group: %v", instance.Id, err)
			failedList = append(failedList, groupJID.String())
		}
		successList = append(failedList, groupJID.String())
	}

	return gin.H{
		"success": successList,
		"failed":  failedList,
	}, nil
}

func (c *communityService) CommunityRemove(data *AddParticipantStruct, instance *instance_model.Instance) (gin.H, error) {
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	communityJID, ok := utils.ParseJID(data.CommunityJID)
	if !ok {
		logger.LogError("[%s] error parse community jid", instance.Id)
		return nil, errors.New("error parse community jid")
	}

	var successList []string
	var failedList []string

	for _, participant := range data.GroupJID {
		groupJID, _ := utils.ParseJID(participant)
		err := client.UnlinkGroup(communityJID, groupJID)
		if err != nil {
			logger.LogError("[%s] error link group: %v", instance.Id, err)
			failedList = append(failedList, groupJID.String())
		}
		successList = append(failedList, groupJID.String())
	}

	return gin.H{
		"success": successList,
		"failed":  failedList,
	}, nil
}

func NewCommunityService(
	clientPointer map[string]*whatsmeow.Client,
	whatsmeowService whatsmeow_service.WhatsmeowService,
) CommunityService {
	return &communityService{
		clientPointer:    clientPointer,
		whatsmeowService: whatsmeowService,
	}
}
