package send_handler

import (
	"net/http"

	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	send_service "github.com/Zapbox-API/evolution-go/pkg/sendMessage/service"
	"github.com/gin-gonic/gin"
)

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
	sendMessageService send_service.SendService
}

func (s *sendHandler) SendText(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.TextStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Text == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "message body is required"})
		return
	}

	msgId, ts, err := s.sendMessageService.SendText(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responseData := gin.H{
		"messageId": msgId,
		"timestamp": ts,
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

func (s *sendHandler) SendLink(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.LinkStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Text == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "message body is required"})
		return
	}

	msgId, ts, err := s.sendMessageService.SendLink(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responseData := gin.H{
		"messageId": msgId,
		"timestamp": ts,
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

func (s *sendHandler) SendMedia(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.MediaStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Url == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "URL is required"})
		return
	}

	if data.Type == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "media type is required"})
		return
	}

	msgId, ts, err := s.sendMessageService.SendMediaUrl(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responseData := gin.H{
		"messageId": msgId,
		"timestamp": ts,
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

// SendContact implements SendHandler.
func (s *sendHandler) SendContact(ctx *gin.Context) {
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

// SendPoll implements SendHandler.
func (s *sendHandler) SendPoll(ctx *gin.Context) {
	panic("unimplemented")
}

// SendSticker implements SendHandler.
func (s *sendHandler) SendSticker(ctx *gin.Context) {
	panic("unimplemented")
}

func NewSendHandler(
	sendMessageService send_service.SendService,
) SendHandler {
	return &sendHandler{
		sendMessageService: sendMessageService,
	}
}
