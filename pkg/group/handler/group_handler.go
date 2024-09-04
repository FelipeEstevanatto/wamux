package group_handler

import (
	"net/http"

	group_service "github.com/Zapbox-API/evolution-go/pkg/group/service"
	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

type GroupHandler interface {
	ListGroups(ctx *gin.Context)
	GetGroupInfo(ctx *gin.Context)
	GetGroupInviteLink(ctx *gin.Context)
	SetGroupPhoto(ctx *gin.Context)
	SetGroupName(ctx *gin.Context)
	CreateGroup(ctx *gin.Context)
	UpdateParticipant(ctx *gin.Context)
	GetMyGroups(ctx *gin.Context)
	JoinGroupLink(ctx *gin.Context)
}

type groupHandler struct {
	groupService group_service.GroupService
}

func (g *groupHandler) ListGroups(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	resp, err := g.groupService.ListGroups(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
}

func (g *groupHandler) GetGroupInfo(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *group_service.GetGroupInfoStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.GroupJID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "groupJID is required"})
		return
	}

	resp, err := g.groupService.GetGroupInfo(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
}

func (g *groupHandler) GetGroupInviteLink(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *group_service.GetGroupInviteLinkStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.GroupJID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "groupJID is required"})
		return
	}

	resp, err := g.groupService.GetGroupInviteLink(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
}

func (g *groupHandler) SetGroupPhoto(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *group_service.SetGroupPhotoStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.GroupJID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "groupJID is required"})
		return
	}

	if data.Image == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "image is required"})
		return
	}

	resp, err := g.groupService.SetGroupPhoto(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
}

func (g *groupHandler) SetGroupName(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *group_service.SetGroupNameStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.GroupJID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "groupJID is required"})
		return
	}

	if data.Name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	err = g.groupService.SetGroupName(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (g *groupHandler) CreateGroup(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *group_service.CreateGroupStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.GroupName == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "groupName is required"})
		return
	}

	if len(data.Participants) <= 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "participants are required"})
		return
	}

	group, err := g.groupService.CreateGroup(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": group})
}

func (g *groupHandler) UpdateParticipant(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *group_service.AddParticipantStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.GroupJID.String() == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "groupJid is required"})
		return
	}

	if data.Action == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "action is required"})
		return
	}

	if len(data.Participants) <= 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "participants are required"})
		return
	}

	err = g.groupService.UpdateParticipant(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (g *groupHandler) GetMyGroups(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	groups, err := g.groupService.GetMyGroups(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": groups})
}

func (g *groupHandler) JoinGroupLink(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *group_service.JoinGroupStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Code == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	err = g.groupService.JoinGroupLink(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

func NewGroupHandler(
	groupService group_service.GroupService,
) GroupHandler {
	return &groupHandler{
		groupService: groupService,
	}
}
