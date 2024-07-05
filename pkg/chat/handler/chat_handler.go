package chat_handler

import "github.com/gin-gonic/gin"

type ChatHandler interface {
	ChatPin(ctx *gin.Context)
	ChatUnpin(ctx *gin.Context)
	ChatArchive(ctx *gin.Context)
	ChatMute(ctx *gin.Context)
}

type chatHandler struct {
}

// ChatArchive implements ChatHandler.
func (c *chatHandler) ChatArchive(ctx *gin.Context) {
	panic("unimplemented")
}

// ChatMute implements ChatHandler.
func (c *chatHandler) ChatMute(ctx *gin.Context) {
	panic("unimplemented")
}

// ChatPin implements ChatHandler.
func (c *chatHandler) ChatPin(ctx *gin.Context) {
	panic("unimplemented")
}

// ChatUnpin implements ChatHandler.
func (c *chatHandler) ChatUnpin(ctx *gin.Context) {
	panic("unimplemented")
}

func NewChatHandler() ChatHandler {
	return &chatHandler{}
}
