package group_handler

import "github.com/gin-gonic/gin"

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
}

// CreateGroup implements GroupHandler.
func (g *groupHandler) CreateGroup(ctx *gin.Context) {
	panic("unimplemented")
}

// GetGroupInfo implements GroupHandler.
func (g *groupHandler) GetGroupInfo(ctx *gin.Context) {
	panic("unimplemented")
}

// GetGroupInviteLink implements GroupHandler.
func (g *groupHandler) GetGroupInviteLink(ctx *gin.Context) {
	panic("unimplemented")
}

// GetMyGroups implements GroupHandler.
func (g *groupHandler) GetMyGroups(ctx *gin.Context) {
	panic("unimplemented")
}

// JoinGroupLink implements GroupHandler.
func (g *groupHandler) JoinGroupLink(ctx *gin.Context) {
	panic("unimplemented")
}

// ListGroups implements GroupHandler.
func (g *groupHandler) ListGroups(ctx *gin.Context) {
	panic("unimplemented")
}

// SetGroupName implements GroupHandler.
func (g *groupHandler) SetGroupName(ctx *gin.Context) {
	panic("unimplemented")
}

// SetGroupPhoto implements GroupHandler.
func (g *groupHandler) SetGroupPhoto(ctx *gin.Context) {
	panic("unimplemented")
}

// UpdateParticipant implements GroupHandler.
func (g *groupHandler) UpdateParticipant(ctx *gin.Context) {
	panic("unimplemented")
}

func NewGroupHandler() GroupHandler {
	return &groupHandler{}
}
