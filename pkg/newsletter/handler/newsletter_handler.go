package newsletter_handler

import (
	"net/http"

	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	newsletter_service "github.com/Zapbox-API/evolution-go/pkg/newsletter/service"
	"github.com/gin-gonic/gin"
)

type NewsletterHandler interface {
	CreateNewsletter(ctx *gin.Context)
	ListNewsletter(ctx *gin.Context)
	GetNewsletter(ctx *gin.Context)
	GetNewsletterInvite(ctx *gin.Context)
	SubscribeNewsletter(ctx *gin.Context)
	GetNewsletterMessages(ctx *gin.Context)
}

type newsletterHandler struct {
	newsletterService newsletter_service.NewsletterService
}

func (n *newsletterHandler) CreateNewsletter(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *newsletter_service.CreateNewsletterStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	newsletter, err := n.newsletterService.CreateNewsletter(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": newsletter})
}

func (n *newsletterHandler) ListNewsletter(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	newsletters, err := n.newsletterService.ListNewsletter(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": newsletters})
}

func (n *newsletterHandler) GetNewsletter(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *newsletter_service.GetNewsletterStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.JID.String() == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "jid is required"})
		return
	}

	newsletter, err := n.newsletterService.GetNewsletter(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": newsletter})
}

func (n *newsletterHandler) GetNewsletterInvite(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *newsletter_service.GetNewsletterInviteStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Key == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
		return
	}

	newsletter, err := n.newsletterService.GetNewsletterInvite(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": newsletter})
}

func (n *newsletterHandler) SubscribeNewsletter(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *newsletter_service.GetNewsletterStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.JID.String() == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "jid is required"})
		return
	}

	err = n.newsletterService.SubscribeNewsletter(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (n *newsletterHandler) GetNewsletterMessages(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *newsletter_service.GetNewsletterMessagesStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.JID.String() == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "jid is required"})
		return
	}

	messages, err := n.newsletterService.GetNewsletterMessages(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": messages})
}

func NewNewsletterHandler(
	newsletterService newsletter_service.NewsletterService,
) NewsletterHandler {
	return &newsletterHandler{
		newsletterService: newsletterService,
	}
}
