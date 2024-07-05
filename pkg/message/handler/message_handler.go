package message_handler

import "github.com/gin-gonic/gin"

type MessageHandler interface {
	React(ctx *gin.Context)
	ChatPresence(ctx *gin.Context)
	MarkRead(ctx *gin.Context)
	DownloadImage(ctx *gin.Context)
	GetMessageStatus(ctx *gin.Context)
	DeleteMessageEveryone(ctx *gin.Context)
	EditMessage(ctx *gin.Context)
}

type messageHandler struct {
}

// ChatPresence implements MessageHandler.
func (m *messageHandler) ChatPresence(ctx *gin.Context) {
	panic("unimplemented")
}

// DeleteMessageEveryone implements MessageHandler.
func (m *messageHandler) DeleteMessageEveryone(ctx *gin.Context) {
	panic("unimplemented")
}

// DownloadImage implements MessageHandler.
func (m *messageHandler) DownloadImage(ctx *gin.Context) {
	panic("unimplemented")
}

// EditMessage implements MessageHandler.
func (m *messageHandler) EditMessage(ctx *gin.Context) {
	panic("unimplemented")
}

// GetMessageStatus implements MessageHandler.
func (m *messageHandler) GetMessageStatus(ctx *gin.Context) {
	panic("unimplemented")
}

// MarkRead implements MessageHandler.
func (m *messageHandler) MarkRead(ctx *gin.Context) {
	panic("unimplemented")
}

// React implements MessageHandler.
func (m *messageHandler) React(ctx *gin.Context) {
	panic("unimplemented")
}

func NewMessageHandler() MessageHandler {
	return &messageHandler{}
}
