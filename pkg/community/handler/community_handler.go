package community_handler

import "github.com/gin-gonic/gin"

type CommunityHandler interface {
	CreateCommunity(ctx *gin.Context)
	CommunityAdd(ctx *gin.Context)
	CommunityRemove(ctx *gin.Context)
}

type communityHandler struct {
}

// CommunityAdd implements CommunityHandler.
func (c *communityHandler) CommunityAdd(ctx *gin.Context) {
	panic("unimplemented")
}

// CommunityRemove implements CommunityHandler.
func (c *communityHandler) CommunityRemove(ctx *gin.Context) {
	panic("unimplemented")
}

// CreateCommunity implements CommunityHandler.
func (c *communityHandler) CreateCommunity(ctx *gin.Context) {
	panic("unimplemented")
}

func NewCommunityHandler() CommunityHandler {
	return &communityHandler{}
}
