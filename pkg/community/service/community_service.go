package community_service

import (
	"errors"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"github.com/EvolutionAPI/evolution-go/pkg/utils"
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
	clientPointer map[string]*whatsmeow.Client
}

type CreateCommunityStruct struct {
	CommunityName string `json:"communityName"`
}

type AddParticipantStruct struct {
	CommunityJID string   `json:"communityJid"`
	GroupJID     []string `json:"groupJid"`
}

func (c *communityService) CreateCommunity(data *CreateCommunityStruct, instance *instance_model.Instance) (*types.GroupInfo, error) {
	if c.clientPointer[instance.Id] == nil {
		return nil, errors.New("no session found")
	}

	resp, err := c.clientPointer[instance.Id].CreateGroup(whatsmeow.ReqCreateGroup{
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
	if c.clientPointer[instance.Id] == nil {
		return nil, errors.New("no session found")
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

		err := c.clientPointer[instance.Id].LinkGroup(communityJID, groupJID)

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
	if c.clientPointer[instance.Id] == nil {
		return nil, errors.New("no session found")
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

		err := c.clientPointer[instance.Id].UnlinkGroup(communityJID, groupJID)

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
) CommunityService {
	return &communityService{
		clientPointer: clientPointer,
	}
}
