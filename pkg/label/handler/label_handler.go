package label_handler

import "github.com/gin-gonic/gin"

type LabelHandler interface {
	ChatLabel(ctx *gin.Context)
	MessageLabel(ctx *gin.Context)
	EditLabel(ctx *gin.Context)
	ChatUnlabel(ctx *gin.Context)
	MessageUnlabel(ctx *gin.Context)
}

type labelHandler struct {
}

// ChatLabel implements LabelHandler.
func (l *labelHandler) ChatLabel(ctx *gin.Context) {
	panic("unimplemented")
}

// ChatUnlabel implements LabelHandler.
func (l *labelHandler) ChatUnlabel(ctx *gin.Context) {
	panic("unimplemented")
}

// EditLabel implements LabelHandler.
func (l *labelHandler) EditLabel(ctx *gin.Context) {
	panic("unimplemented")
}

// MessageLabel implements LabelHandler.
func (l *labelHandler) MessageLabel(ctx *gin.Context) {
	panic("unimplemented")
}

// MessageUnlabel implements LabelHandler.
func (l *labelHandler) MessageUnlabel(ctx *gin.Context) {
	panic("unimplemented")
}

func NewLabelHandler() LabelHandler {
	return &labelHandler{}
}
