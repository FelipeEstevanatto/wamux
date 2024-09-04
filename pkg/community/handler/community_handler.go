package community_handler

import (
	"net/http"

	community_service "github.com/Zapbox-API/evolution-go/pkg/community/service"
	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

type CommunityHandler interface {
	CreateCommunity(ctx *gin.Context)
	CommunityAdd(ctx *gin.Context)
	CommunityRemove(ctx *gin.Context)
}

type communityHandler struct {
	communityService community_service.CommunityService
}

func (c *communityHandler) CreateCommunity(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *community_service.CreateCommunityStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.CommunityName == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "community name is required"})
		return
	}

	community, err := c.communityService.CreateCommunity(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": community})
}

func (c *communityHandler) CommunityAdd(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *community_service.AddParticipantStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.CommunityJID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "community jid is required"})
		return
	}

	if len(data.GroupJID) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "group jid is required"})
		return
	}

	resp, err := c.communityService.CommunityAdd(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
}

func (c *communityHandler) CommunityRemove(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *community_service.AddParticipantStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.CommunityJID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "community jid is required"})
		return
	}

	if len(data.GroupJID) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "group jid is required"})
		return
	}

	resp, err := c.communityService.CommunityRemove(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
}

func NewCommunityHandler(
	communityService community_service.CommunityService,
) CommunityHandler {
	return &communityHandler{
		communityService: communityService,
	}
}
