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

type SendTextMessageRequest struct {
	Phone string `json:"phone" example:"557499879409"`
	Text  string `json:"text" example:"Teste"`
}

type SendMediaMessageRequest struct {
	Phone string `json:"phone" example:"557499879409"`
	Url   string `json:"url" example:"https://example.com/image.jpg"`
	Type  string `json:"type" example:"image"`
}

type SendPollMessageRequest struct {
	Phone     string   `json:"phone" example:"557499879409"`
	Question  string   `json:"question" example:"What is your favorite color?"`
	MaxAnswer int      `json:"maxAnswer" example:"1"`
	Options   []string `json:"options" example:"[\"Red\", \"Blue\"]"`
}

type SendStickerMessageRequest struct {
	Phone   string `json:"phone" example:"557499879409"`
	Sticker string `json:"sticker" example:"https://example.com/sticker.webp"`
}

type SendLocationMessageRequest struct {
	Phone     string  `json:"phone" example:"557499879409"`
	Name      string  `json:"name" example:"São Paulo"`
	Latitude  float64 `json:"latitude" example:"-23.5505199"`
	Longitude float64 `json:"longitude" example:"-46.6333094"`
}

type SendContactMessageRequest struct {
	Phone string `json:"phone" example:"557499879409"`
	Vcard struct {
		FullName string `json:"fullName" example:"John Doe"`
		Phone    string `json:"phone" example:"557499879409"`
	} `json:"vcard"`
}

type SendListMessageRequest struct {
	Phone      string   `json:"phone" example:"557499879409"`
	ButtonText string   `json:"buttonText" example:"View"`
	Desc       string   `json:"desc" example:"This is a list"`
	Items      []string `json:"items" example:"[\"Item 1\", \"Item 2\"]"`
}

// Send a text message
// @Summary Send a text message
// @Description Send a text message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body SendTextMessageRequest true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/text [post]
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

// Send a link message
// @Summary Send a link message
// @Description Send a link message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body SendTextMessageRequest true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/link [post]
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

// Send a media message
// @Summary Send a media message
// @Description Send a media message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body SendMediaMessageRequest true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/media [post]
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

// Send a poll message
// @Summary Send a poll message
// @Description Send a poll message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body SendPollMessageRequest true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/poll [post]
func (s *sendHandler) SendPoll(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.PollStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Question == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "question is required"})
		return
	}

	if len(data.Options) < 2 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "minimum 2 options are required"})
		return
	}

	msgId, ts, err := s.sendMessageService.SendPoll(data, instance)
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

// Send a sticker message
// @Summary Send a sticker message
// @Description Send a sticker message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body SendStickerMessageRequest true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/sticker [post]
func (s *sendHandler) SendSticker(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.StickerStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Sticker == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "sticker is required"})
		return
	}

	msgId, ts, err := s.sendMessageService.SendSticker(data, instance)
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

// Send a location message
// @Summary Send a location message
// @Description Send a location message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body SendLocationMessageRequest true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/location [post]
func (s *sendHandler) SendLocation(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.LocationStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Latitude == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "latitude is required"})
		return
	}

	if data.Longitude == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "longitude is required"})
		return
	}

	msgId, ts, err := s.sendMessageService.SendLocation(data, instance)
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

// Send a contact message
// @Summary Send a contact message
// @Description Send a contact message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body SendContactMessageRequest true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/contact [post]
func (s *sendHandler) SendContact(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.ContactStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Vcard.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "contact phone number is required"})
		return
	}

	if data.Vcard.FullName == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "contact full name is required"})
		return
	}

	msgId, ts, err := s.sendMessageService.SendContact(data, instance)
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

// Send a list message
// @Summary Send a list message
// @Description Send a list message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body SendListMessageRequest true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/list [post]
func (s *sendHandler) SendList(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.ListStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.ButtonText == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "button text is required"})
		return
	}

	if data.Desc == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "description is required"})
		return
	}

	msgId, ts, err := s.sendMessageService.SendList(data, instance)
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

func NewSendHandler(
	sendMessageService send_service.SendService,
) SendHandler {
	return &sendHandler{
		sendMessageService: sendMessageService,
	}
}
