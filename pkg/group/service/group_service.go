package group_service

import (
	"errors"
	"strings"

	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	"github.com/Zapbox-API/evolution-go/pkg/utils"
	whatsmeow_service "github.com/Zapbox-API/evolution-go/pkg/whatsmeow/service"
	"github.com/gin-gonic/gin"
	"github.com/gomessguii/logger"
	"github.com/vincent-petithory/dataurl"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type GroupService interface {
	ListGroups(instance *instance_model.Instance) (*GroupCollection, error)
	GetGroupInfo(data *GetGroupInfoStruct, instance *instance_model.Instance) (*types.GroupInfo, error)
	GetGroupInviteLink(data *GetGroupInviteLinkStruct, instance *instance_model.Instance) (string, error)
	SetGroupPhoto(data *SetGroupPhotoStruct, instance *instance_model.Instance) (string, error)
	SetGroupName(data *SetGroupNameStruct, instance *instance_model.Instance) error
	CreateGroup(data *CreateGroupStruct, instance *instance_model.Instance) (gin.H, error)
	UpdateParticipant(data *AddParticipantStruct, instance *instance_model.Instance) error
	GetMyGroups(instance *instance_model.Instance) ([]types.GroupInfo, error)
	JoinGroupLink(data *JoinGroupStruct, instance *instance_model.Instance) error
}

type groupService struct {
	clientPointer map[string]whatsmeow_service.ClientInfo
}

type SimpleGroupInfo struct {
	JID       types.JID `json:"jid"`
	GroupName string    `json:"groupName"`
}

type GroupCollection struct {
	Groups []SimpleGroupInfo
}

type GetGroupInfoStruct struct {
	GroupJID string `json:"groupJid"`
}

type GetGroupInviteLinkStruct struct {
	GroupJID string `json:"groupJid"`
	Reset    bool   `json:"reset"`
}

type SetGroupPhotoStruct struct {
	GroupJID string `json:"groupJid"`
	Image    string `json:"image"`
}

type SetGroupNameStruct struct {
	GroupJID string `json:"groupJid"`
	Name     string `json:"name"`
}

type CreateGroupStruct struct {
	GroupName    string   `json:"groupName"`
	Participants []string `json:"participants"`
}

type AddParticipantStruct struct {
	GroupJID     types.JID                   `json:"groupJid"`
	Participants []string                    `json:"participants"`
	Action       whatsmeow.ParticipantChange `json:"action"`
}

type JoinGroupStruct struct {
	Code string `json:"code"`
}

func (g *groupService) ListGroups(instance *instance_model.Instance) (*GroupCollection, error) {
	if g.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	resp, err := g.clientPointer[instance.Id].WAClient.GetJoinedGroups()
	if err != nil {
		logger.LogError("error mute chat: %v", err)
		return nil, err
	}

	gc := new(GroupCollection)
	for _, info := range resp {
		simpleGroup := SimpleGroupInfo{
			JID:       info.JID,
			GroupName: info.GroupName.Name,
		}
		gc.Groups = append(gc.Groups, simpleGroup)
	}

	return gc, nil
}

func (g *groupService) GetGroupInfo(data *GetGroupInfoStruct, instance *instance_model.Instance) (*types.GroupInfo, error) {
	if g.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	recipient, ok := utils.ParseJID(data.GroupJID)
	if !ok {
		logger.LogError("Error validating message fields")
		return nil, errors.New("invalid group jid")
	}

	resp, err := g.clientPointer[instance.Id].WAClient.GetGroupInfo(recipient)
	if err != nil {
		logger.LogError("error mute chat: %v", err)
		return nil, err
	}

	return resp, nil
}

func (g *groupService) GetGroupInviteLink(data *GetGroupInviteLinkStruct, instance *instance_model.Instance) (string, error) {
	if g.clientPointer[instance.Id].WAClient == nil {
		return "", errors.New("no session found")
	}

	recipient, ok := utils.ParseJID(data.GroupJID)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", errors.New("invalid group jid")
	}

	resp, err := g.clientPointer[instance.Id].WAClient.GetGroupInviteLink(recipient, data.Reset)
	if err != nil {
		logger.LogError("error mute chat: %v", err)
		return "", err
	}

	return resp, nil
}

func (g *groupService) SetGroupPhoto(data *SetGroupPhotoStruct, instance *instance_model.Instance) (string, error) {
	if g.clientPointer[instance.Id].WAClient == nil {
		return "", errors.New("no session found")
	}

	recipient, ok := utils.ParseJID(data.GroupJID)
	if !ok {
		logger.LogError("Error validating message fields")
		return "", errors.New("invalid group jid")
	}

	var fileData []byte

	if data.Image[0:13] == "data:image/jp" {
		dataURL, err := dataurl.DecodeString(data.Image)
		if err != nil {
			logger.LogError("Could not decode base64 encoded data from payloads")
			return "", err
		} else {
			fileData = dataURL.Data
		}
	} else {
		logger.LogError("Image data should start with \"data:image/jpeg;base64,\"")
		return "", errors.New("image data should start with \"data:image/jpeg;base64,\"")
	}

	picture_id, err := g.clientPointer[instance.Id].WAClient.SetGroupPhoto(recipient, fileData)
	if err != nil {
		logger.LogError("error mute chat: %v", err)
		return "", err
	}

	return picture_id, nil
}

func (g *groupService) SetGroupName(data *SetGroupNameStruct, instance *instance_model.Instance) error {
	if g.clientPointer[instance.Id].WAClient == nil {
		return errors.New("no session found")
	}

	recipient, ok := utils.ParseJID(data.GroupJID)
	if !ok {
		logger.LogError("Error validating message fields")
		return errors.New("invalid group jid")
	}

	err := g.clientPointer[instance.Id].WAClient.SetGroupName(recipient, data.Name)
	if err != nil {
		logger.LogError("error mute chat: %v", err)
		return err
	}

	return nil
}

func (g *groupService) CreateGroup(data *CreateGroupStruct, instance *instance_model.Instance) (gin.H, error) {
	if g.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	var participants []types.JID
	for _, participant := range data.Participants {
		recipient, ok := utils.ParseJID(participant)
		participants = append(participants, recipient)
		if !ok {
			logger.LogError("Error validating message fields")
			return nil, errors.New("invalid phone number")
		}
	}

	resp, err := g.clientPointer[instance.Id].WAClient.CreateGroup(whatsmeow.ReqCreateGroup{
		Name:         data.GroupName,
		Participants: participants,
	})
	if err != nil {
		logger.LogError("error create group: %v", err)
		return nil, err
	}

	var failed []types.JID
	for _, participant := range resp.Participants {
		if participant.Error != 0 {
			failed = append(failed, participant.JID)
		}
	}

	var added []types.JID
	infoResp, err := g.clientPointer[instance.Id].WAClient.GetGroupInfo(resp.JID)
	if err != nil {
		logger.LogError("error get group info: %v", err)
		return nil, err
	}
	for _, add := range infoResp.Participants {
		added = append(added, add.JID)
	}

	response := gin.H{
		"jid":    resp.JID,
		"name":   resp.Name,
		"owner":  resp.OwnerJID,
		"added":  added,
		"failed": failed,
	}

	return response, nil
}

func (g *groupService) UpdateParticipant(data *AddParticipantStruct, instance *instance_model.Instance) error {
	if g.clientPointer[instance.Id].WAClient == nil {
		return errors.New("no session found")
	}

	var participants []types.JID
	for _, participant := range data.Participants {
		recipient, ok := utils.ParseJID(participant)
		participants = append(participants, recipient)
		if !ok {
			logger.LogError("Error validating message fields")
			return errors.New("invalid phone number")
		}
	}

	_, err := g.clientPointer[instance.Id].WAClient.UpdateGroupParticipants(data.GroupJID, participants, data.Action)
	if err != nil {
		logger.LogError("error create group: %v", err)
		return err
	}

	return nil
}

func (g *groupService) GetMyGroups(instance *instance_model.Instance) ([]types.GroupInfo, error) {
	if g.clientPointer[instance.Id].WAClient == nil {
		return nil, errors.New("no session found")
	}

	resp, err := g.clientPointer[instance.Id].WAClient.GetJoinedGroups()
	if err != nil {
		logger.LogError("error create group: %v", err)
		return nil, err
	}

	var jid string = g.clientPointer[instance.Id].WAClient.Store.ID.String()
	var jidClear = strings.Split(jid, ".")[0]
	jidOfAdmin, ok := utils.ParseJID(jidClear)
	if !ok {
		logger.LogError("Error validating message fields")
		return nil, errors.New("invalid phone number")
	}
	var adminGroups []types.GroupInfo
	for _, group := range resp {
		if group.OwnerJID == jidOfAdmin {
			adminGroups = append(adminGroups, *group)
			_ = adminGroups
		}
	}

	return adminGroups, nil
}

func (g *groupService) JoinGroupLink(data *JoinGroupStruct, instance *instance_model.Instance) error {
	if g.clientPointer[instance.Id].WAClient == nil {
		return errors.New("no session found")
	}

	_, err := g.clientPointer[instance.Id].WAClient.JoinGroupWithLink(data.Code)
	if err != nil {
		logger.LogError("error create group: %v", err)
		return err
	}

	return nil
}

func NewGroupService(
	clientPointer map[string]whatsmeow_service.ClientInfo,
) GroupService {
	return &groupService{
		clientPointer: clientPointer,
	}
}
