package send_handler

import "github.com/gin-gonic/gin"

type SendHandler interface {
	SendText(ctx *gin.Context)
	SendLink(ctx *gin.Context)
	SendMedia(ctx *gin.Context)
	SendPoll(ctx *gin.Context)
	SendSticker(ctx *gin.Context)
	SendLocation(ctx *gin.Context)
	SendContact(ctx *gin.Context)
	SendList(ctx *gin.Context)
}

type sendHandler struct {
}

// SendContact implements SendHandler.
func (s *sendHandler) SendContact(ctx *gin.Context) {
	panic("unimplemented")
}

// SendLink implements SendHandler.
func (s *sendHandler) SendLink(ctx *gin.Context) {
	panic("unimplemented")
}

// SendList implements SendHandler.
func (s *sendHandler) SendList(ctx *gin.Context) {
	panic("unimplemented")
}

// SendLocation implements SendHandler.
func (s *sendHandler) SendLocation(ctx *gin.Context) {
	panic("unimplemented")
}

// SendMedia implements SendHandler.
func (s *sendHandler) SendMedia(ctx *gin.Context) {
	panic("unimplemented")
}

// SendPoll implements SendHandler.
func (s *sendHandler) SendPoll(ctx *gin.Context) {
	panic("unimplemented")
}

// SendSticker implements SendHandler.
func (s *sendHandler) SendSticker(ctx *gin.Context) {
	panic("unimplemented")
}

// SendText implements SendHandler.
func (s *sendHandler) SendText(ctx *gin.Context) {
	panic("unimplemented")
}

func NewSendHandler() SendHandler {
	return &sendHandler{}
}
