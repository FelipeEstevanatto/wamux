package send_handler

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/felipeestevanatto/wamux/pkg/channel"
	cloud_service "github.com/felipeestevanatto/wamux/pkg/cloud"
	docmodels "github.com/felipeestevanatto/wamux/pkg/docmodels"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	send_service "github.com/felipeestevanatto/wamux/pkg/sendMessage/service"
	"github.com/gin-gonic/gin"
)

// Keep docmodels referenced so the import is not dropped
// (swag reads Go source, not pre-processed, and needs the alias to be in scope).
var _ = docmodels.Envelope{}

type SendHandler interface {
	SendText(ctx *gin.Context)
	SendLink(ctx *gin.Context)
	SendMedia(ctx *gin.Context)
	SendAlbum(ctx *gin.Context)
	SendPoll(ctx *gin.Context)
	SendSticker(ctx *gin.Context)
	SendLocation(ctx *gin.Context)
	SendContact(ctx *gin.Context)
	SendButton(ctx *gin.Context)
	SendList(ctx *gin.Context)
	SendCarousel(ctx *gin.Context)
	SendEvent(ctx *gin.Context)
	SendProduct(ctx *gin.Context)
	SendStatusText(ctx *gin.Context)
	SendStatusMedia(ctx *gin.Context)
	SendTemplate(ctx *gin.Context)
	SendCatalog(ctx *gin.Context)
	SendProductList(ctx *gin.Context)
	SendContactRequest(ctx *gin.Context)
}

// cloudExtras is implemented by the send router. Keeping it optional avoids
// forcing every Baileys-only fake to grow Cloud methods.
type cloudExtras interface {
	SendCatalog(*send_service.CatalogStruct, *instance_model.Instance) (*send_service.MessageSendStruct, error)
	SendProductList(*send_service.ProductListStruct, *instance_model.Instance) (*send_service.MessageSendStruct, error)
	SendContactRequest(*send_service.ContactRequestStruct, *instance_model.Instance) (*send_service.MessageSendStruct, error)
}

type sendHandler struct {
	sendMessageService send_service.SendService
}

// readUpload reads a multipart file that has to be in memory.
func readUpload(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, send_service.MaxInMemoryMediaBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > send_service.MaxInMemoryMediaBytes {
		return nil, send_service.ErrLargeMediaType
	}
	return data, nil
}

// writeMediaError answers with the real reason a file was refused.
func writeMediaError(ctx *gin.Context, err error) {
	var apiErr *cloud_service.APIError
	if errors.As(err, &apiErr) {
		writeSendError(ctx, err)
		return
	}
	status := http.StatusInternalServerError
	if errors.Is(err, send_service.ErrMediaTooLarge) || errors.Is(err, send_service.ErrLargeMediaType) {
		status = http.StatusRequestEntityTooLarge
	}
	ctx.JSON(status, gin.H{"error": err.Error()})
}

// Send a text message
// @Summary Send a text message
// @Description Send a text message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.TextStruct true "Message data"
// @Success 200 {object} docmodels.Envelope{data=docmodels.MessageSend} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /send/text [post]
func (s *sendHandler) SendText(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	data := &send_service.TextStruct{}
	err := ctx.ShouldBindBodyWithJSON(data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" && data.Recipient == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number or recipient is required"})
		return
	}

	if data.Text == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "message body is required"})
		return
	}

	message, err := s.sendMessageService.SendText(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a link message
// @Summary Send a link message
// @Description Send a link message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.LinkStruct true "Message data"
// @Success 200 {object} docmodels.Envelope{data=docmodels.MessageSend} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /send/link [post]
func (s *sendHandler) SendLink(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	data := &send_service.LinkStruct{}
	err := ctx.ShouldBindBodyWithJSON(data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" && data.Recipient == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number or recipient is required"})
		return
	}

	if data.Text == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "message body is required"})
		return
	}

	message, err := s.sendMessageService.SendLink(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a media message
// @Summary Send a media message
// @Description Send a media message. `type` is one of image, video, gif, audio, document, ptv.
// @Description For `type: "gif"` (or a GIF URL with `gifPlayback: true`) the animated GIF is
// @Description transcoded to a silent MP4 and sent as a looping animation without controls.
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.MediaStruct true "Message data"
// @Success 200 {object} docmodels.Envelope{data=docmodels.MessageSend} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /send/media [post]
func (s *sendHandler) SendMedia(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	contentType := ctx.ContentType()

	data := &send_service.MediaStruct{}

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle form-data
		number := ctx.PostForm("number")
		recipient := ctx.PostForm("recipient")
		if number == "" && recipient == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number or recipient is required"})
			return
		}

		mediaType := ctx.PostForm("type")
		if mediaType == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "media type is required"})
			return
		}

		caption := ctx.PostForm("caption")
		filename := ctx.PostForm("filename")
		id := ctx.PostForm("id")
		delayStr := ctx.PostForm("delay")
		delay := int32(0)
		if delayStr != "" {
			delay64, err := strconv.ParseInt(delayStr, 10, 32)
			if err != nil {
				ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid delay"})
				return
			}
			delay = int32(delay64)
		}

		mentionAll := ctx.PostForm("mentionAll") == "true"

		var mentionedJID []string
		// Accept multiple values (mentionedJid=x&mentionedJid=y) or a single
		// comma-separated string (mentionedJid=x,y).
		for _, raw := range ctx.PostFormArray("mentionedJid") {
			for _, v := range strings.Split(raw, ",") {
				if trimmed := strings.TrimSpace(v); trimmed != "" {
					mentionedJID = append(mentionedJID, trimmed)
				}
			}
		}

		var quoted send_service.QuotedStruct
		quoted.MessageID = ctx.PostForm("quoted.messageId")
		quoted.Participant = ctx.PostForm("quoted.participant")

		// Get file
		file, err := ctx.FormFile("file")
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
			return
		}

		// Open file
		fileData, err := file.Open()
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "cannot open file"})
			return
		}
		defer fileData.Close()

		// Create MediaStruct
		data = &send_service.MediaStruct{
			Number:       number,
			Recipient:    recipient,
			Type:         mediaType,
			Caption:      caption,
			Filename:     filename,
			Id:           id,
			Delay:        delay,
			MentionAll:   mentionAll,
			MentionedJID: mentionedJID,
			Quoted:       quoted,
		}

		// A file this large is already on disk, where the multipart parser put it.
		if onDisk, ok := fileData.(*os.File); ok && file.Size > send_service.MaxInMemoryMediaBytes {
			message, err := s.sendMessageService.SendMediaStream(data, onDisk, instance)
			if err != nil {
				writeMediaError(ctx, err)
				return
			}
			ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
			return
		}

		fileBytes, err := readUpload(fileData)
		if err != nil {
			writeMediaError(ctx, err)
			return
		}

		// Pass fileBytes to the send service
		message, err := s.sendMessageService.SendMediaFile(data, fileBytes, instance)
		if err != nil {
			writeMediaError(ctx, err)
			return
		}

		ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})

	} else {

		err := ctx.ShouldBindBodyWithJSON(data)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if data.Number == "" && data.Recipient == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number or recipient is required"})
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

		var message *send_service.MessageSendStruct

		if !strings.HasPrefix(data.Url, "http://") && !strings.HasPrefix(data.Url, "https://") {
			// Treat as base64-encoded media
			fileBytes, err := base64.StdEncoding.DecodeString(data.Url)
			if err != nil {
				ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64 encoding"})
				return
			}
			message, err = s.sendMessageService.SendMediaFile(data, fileBytes, instance)
			if err != nil {
				writeSendError(ctx, err)
				return
			}
		} else {
			message, err = s.sendMessageService.SendMediaUrl(data, instance)
			if err != nil {
				writeMediaError(ctx, err)
				return
			}
		}

		ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
	}
}

// Send an album
// @Summary Send an album
// @Description Sends 2-30 images/videos as one WhatsApp album. Each item may have its own caption.
// @Description `media` accepts a URL, a data URI or raw base64. The reply/mentions are attached to the first item.
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.AlbumStruct true "Album data"
// @Success 200 {object} docmodels.Envelope{data=send_service.AlbumSendResult} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /send/album [post]
func (s *sendHandler) SendAlbum(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	if ctx.GetHeader("Idempotency-Key") != "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Idempotency-Key is not supported for albums; each item has its own WhatsApp message",
		})
		return
	}

	data := &send_service.AlbumStruct{}
	if err := ctx.ShouldBindBodyWithJSON(data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}
	if len(data.Items) < 2 || len(data.Items) > 30 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "an album needs between 2 and 30 items"})
		return
	}
	for i, item := range data.Items {
		if item.Media == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "album item " + strconv.Itoa(i) + " has no media"})
			return
		}
		if item.Mediatype != "image" && item.Mediatype != "video" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "album item " + strconv.Itoa(i) + " must be image or video"})
			return
		}
	}

	result, err := s.sendMessageService.SendAlbum(data, instance)
	if err != nil {
		writeMediaError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": result})
}

// Send a poll message
// @Summary Send a poll message
// @Description Send a poll message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.PollStruct true "Message data"
// @Success 200 {object} docmodels.Envelope{data=docmodels.MessageSend} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /send/poll [post]
func (s *sendHandler) SendPoll(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	data := &send_service.PollStruct{}
	err := ctx.ShouldBindBodyWithJSON(data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
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

	message, err := s.sendMessageService.SendPoll(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a sticker message
// @Summary Send a sticker message
// @Description Send a sticker message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.StickerStruct true "Message data"
// @Success 200 {object} docmodels.Envelope{data=docmodels.MessageSend} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /send/sticker [post]
func (s *sendHandler) SendSticker(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	data := &send_service.StickerStruct{}
	err := ctx.ShouldBindBodyWithJSON(data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Sticker == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "sticker is required"})
		return
	}

	message, err := s.sendMessageService.SendSticker(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a location message
// @Summary Send a location message
// @Description Send a location message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.LocationStruct true "Message data"
// @Success 200 {object} docmodels.Envelope{data=docmodels.MessageSend} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /send/location [post]
func (s *sendHandler) SendLocation(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	data := &send_service.LocationStruct{}
	err := ctx.ShouldBindBodyWithJSON(data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
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

	if data.Address == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "address is required"})
		return
	}

	if data.Name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	message, err := s.sendMessageService.SendLocation(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a contact message
// @Summary Send a contact message
// @Description Send a contact message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.ContactStruct true "Message data"
// @Success 200 {object} docmodels.Envelope{data=docmodels.MessageSend} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /send/contact [post]
func (s *sendHandler) SendContact(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	data := &send_service.ContactStruct{}
	err := ctx.ShouldBindBodyWithJSON(data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
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

	message, err := s.sendMessageService.SendContact(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a button message
// @Summary Send a button message
// @Description Send an interactive message with buttons. Each button has a `type`: `reply`, `copy`, `url`, `call` or `pix`.
// @Description
// @Description Combination rules enforced by the server:
// @Description   - Up to 3 `reply` buttons per message.
// @Description   - `reply` buttons cannot be mixed with any other type.
// @Description   - `pix` button must be sent ALONE (no other button in the same message).
// @Description
// @Description WhatsApp client rendering quirks (NOT enforced by the server, but verified in the field):
// @Description   - WhatsApp Web: only `reply`-only messages (up to 3) OR CTAs grouped together (`copy` + `url` + `call`) render correctly.
// @Description   - Do NOT mix `reply` with CTA buttons (`copy`/`url`/`call`) — the message will not appear on WhatsApp Web.
// @Description
// @Description Required body fields: `number`, `title`, `description`, `footer`, `buttons`.
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.ButtonStruct true "Message data"
// @Success 200 {object} docmodels.Envelope{data=docmodels.MessageSend} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /send/button [post]
func (s *sendHandler) SendButton(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	data := &send_service.ButtonStruct{}
	err := ctx.ShouldBindBodyWithJSON(data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" && data.Recipient == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number or recipient is required"})
		return
	}

	if data.Title == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}

	if data.Description == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "description is required"})
		return
	}

	if data.Footer == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "footer is required"})
		return
	}

	message, err := s.sendMessageService.SendButton(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a list message
// @Summary Send a list message
// @Description Send an interactive list message (single-select) rendered as a tappable menu.
// @Description
// @Description Required body fields: `number`, `title`, `description`, `footerText`, `buttonText`, `sections`.
// @Description Each section must contain one or more `rows`. When `rowId` is omitted, the server generates a fallback ID.
// @Description When `buttonText` is empty, the server falls back to "Ver Menu".
// @Description
// @Description Uses legacy `ListMessage` format (no ViewOnceMessage wrapper) so it renders on iOS, Android and WhatsApp Web.
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.ListStruct true "Message data"
// @Success 200 {object} docmodels.Envelope{data=docmodels.MessageSend} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /send/list [post]
func (s *sendHandler) SendList(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	data := &send_service.ListStruct{}
	err := ctx.ShouldBindBodyWithJSON(data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" && data.Recipient == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number or recipient is required"})
		return
	}

	if data.Title == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}

	if data.Description == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "description is required"})
		return
	}

	if data.FooterText == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "footer is required"})
		return
	}

	if data.ButtonText == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "button text is required"})
		return
	}

	message, err := s.sendMessageService.SendList(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a carousel message
// @Summary Send a carousel message
// @Description Send an interactive carousel (multiple swipeable cards). Each card carries its own image or video, body and optional buttons.
// @Description
// @Description Card button `type` accepted values (case-insensitive, uppercased internally): `REPLY` (default), `URL`, `CALL`, `COPY`.
// @Description The `PIX` button type is NOT supported in carousel cards — use `/send/button` for PIX.
// @Description
// @Description IMPORTANT — `CarouselButtonStruct` is different from the flat button used in `/send/button`:
// @Description   - URL button: put the link in the `id` field (NOT in a `url` field).
// @Description   - CALL button: put the phone number in the `id` field (NOT in a `phoneNumber` field).
// @Description   - COPY button: put the code to be copied in `copyCode`.
// @Description   - REPLY button: put the payload/callback ID in `id`.
// @Description
// @Description Per-card combination rules (NOT enforced by the server, but verified in the field):
// @Description   - Same WhatsApp Web quirk as `/send/button`: avoid mixing REPLY with CTA buttons (URL/CALL/COPY) in the same card — mixed sets do not render on Web.
// @Description   - Stick to either "only REPLY" or "only CTAs grouped together" per card.
// @Description
// @Description Required body fields: `number`, `cards` (at least one). Each card requires `header` + `body`.
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.CarouselStruct true "Message data"
// @Success 200 {object} docmodels.Envelope{data=docmodels.MessageSend} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /send/carousel [post]
func (s *sendHandler) SendCarousel(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	data := &send_service.CarouselStruct{}
	err := ctx.ShouldBindBodyWithJSON(data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if len(data.Cards) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "at least one card is required"})
		return
	}

	message, err := s.sendMessageService.SendCarousel(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a text status message
// @Summary Send a WhatsApp text status
// @Description Send a WhatsApp text status to status@broadcast
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.StatusTextStruct true "Status text data"
// @Success 200 {object} docmodels.Envelope{data=docmodels.MessageSend} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /send/status/text [post]
func (s *sendHandler) SendStatusText(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	data := new(send_service.StatusTextStruct)
	err := ctx.ShouldBindBodyWithJSON(data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Text == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "text is required"})
		return
	}

	message, err := s.sendMessageService.SendStatusText(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a media status message (image or video)
// @Summary Send a WhatsApp media status (image/video)
// @Description Send an image or video status to status@broadcast. Supports JSON (URL) or multipart/form-data (file upload)
// @Tags Send Message
// @Accept json, multipart/form-data
// @Produce json
// @Param type formData string true "Media type: image or video"
// @Param file formData file false "Media file (for multipart upload)"
// @Param url formData string false "Media URL (for JSON upload)"
// @Param caption formData string false "Caption for the media"
// @Param id formData string false "Custom message ID"
// @Success 200 {object} docmodels.Envelope{data=docmodels.MessageSend} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /send/status/media [post]
func (s *sendHandler) SendStatusMedia(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	contentType := ctx.ContentType()

	data := new(send_service.StatusMediaStruct)

	if strings.HasPrefix(contentType, "multipart/form-data") {
		mediaType := ctx.PostForm("type")
		if mediaType == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "media type is required"})
			return
		}

		if mediaType != "image" && mediaType != "video" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "type must be 'image' or 'video'"})
			return
		}

		caption := ctx.PostForm("caption")
		id := ctx.PostForm("id")

		file, err := ctx.FormFile("file")
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
			return
		}

		fileData, err := file.Open()
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "cannot open file"})
			return
		}
		defer fileData.Close()
		fileBytes, err := readUpload(fileData)
		if err != nil {
			writeMediaError(ctx, err)
			return
		}

		data = &send_service.StatusMediaStruct{
			Type:    mediaType,
			Caption: caption,
			Id:      id,
		}

		message, err := s.sendMessageService.SendStatusMediaFile(data, fileBytes, instance)
		if err != nil {
			writeSendError(ctx, err)
			return
		}

		ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
		return
	}

	err := ctx.ShouldBindBodyWithJSON(data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Url == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}

	if data.Type != "image" && data.Type != "video" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "type must be 'image' or 'video'"})
		return
	}

	message, err := s.sendMessageService.SendStatusMediaUrl(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

func NewSendHandler(
	sendMessageService send_service.SendService,
) SendHandler {
	return &sendHandler{
		sendMessageService: sendMessageService,
	}
}

// Send a Cloud API template message.
// @Summary Send a template message
// @Description Send an approved WhatsApp Cloud API template. This route is
// @Description unsupported on Baileys instances.
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.TemplateStruct true "Template message data"
// @Success 200 {object} docmodels.Envelope{data=docmodels.MessageSend} "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 422 {object} docmodels.ErrorResponse "Unsupported on this channel"
// @Router /send/template [post]
func (s *sendHandler) SendTemplate(ctx *gin.Context) {
	instance, ok := ctx.MustGet("instance").(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	data := &send_service.TemplateStruct{}
	if err := ctx.ShouldBindJSON(data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if data.Name == "" || data.Language == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "template name and language are required"})
		return
	}
	message, err := s.sendMessageService.SendTemplate(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// SendCatalog sends a Cloud catalog message.
func (s *sendHandler) SendCatalog(ctx *gin.Context) {
	instance, ok := ctx.MustGet("instance").(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	data := &send_service.CatalogStruct{}
	if err := ctx.ShouldBindJSON(data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if data.Number == "" && data.Recipient == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number or recipient is required"})
		return
	}
	extras, ok := s.sendMessageService.(cloudExtras)
	if !ok {
		ctx.JSON(http.StatusNotImplemented, gin.H{"error": "not_implemented_on_channel", "message": "catalog messages are not supported on this channel"})
		return
	}
	message, err := extras.SendCatalog(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// SendProductList sends a Cloud catalog-backed multi-product message.
func (s *sendHandler) SendProductList(ctx *gin.Context) {
	instance, ok := ctx.MustGet("instance").(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	data := &send_service.ProductListStruct{}
	if err := ctx.ShouldBindJSON(data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if data.Number == "" && data.Recipient == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number or recipient is required"})
		return
	}
	extras, ok := s.sendMessageService.(cloudExtras)
	if !ok {
		ctx.JSON(http.StatusNotImplemented, gin.H{"error": "not_implemented_on_channel", "message": "product lists are not supported on this channel"})
		return
	}
	message, err := extras.SendProductList(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// SendContactRequest sends a Cloud contact-request interactive message.
func (s *sendHandler) SendContactRequest(ctx *gin.Context) {
	instance, ok := ctx.MustGet("instance").(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	data := &send_service.ContactRequestStruct{}
	if err := ctx.ShouldBindJSON(data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if data.Number == "" && data.Recipient == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number or recipient is required"})
		return
	}
	extras, ok := s.sendMessageService.(cloudExtras)
	if !ok {
		ctx.JSON(http.StatusNotImplemented, gin.H{"error": "not_implemented_on_channel", "message": "contact requests are not supported on this channel"})
		return
	}
	message, err := extras.SendContactRequest(data, instance)
	if err != nil {
		writeSendError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

func writeSendError(ctx *gin.Context, err error) {
	var windowErr *cloud_service.WindowExpiredError
	if errors.As(err, &windowErr) {
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "cloud_window_expired", "message": windowErr.Error(),
			"details": gin.H{"contactId": windowErr.ContactID, "templateRequired": true,
				"expiresAt": windowErr.ExpiresAt},
		})
		return
	}
	var apiErr *cloud_service.APIError
	if errors.As(err, &apiErr) {
		status := apiErr.Status
		if status == 0 {
			status = http.StatusInternalServerError
		}
		body := gin.H{"error": apiErr.Code, "message": apiErr.Message}
		if len(apiErr.Details) > 0 {
			body["details"] = apiErr.Details
		}
		if apiErr.RetryAfter > 0 {
			ctx.Header("Retry-After", strconv.Itoa(apiErr.RetryAfter))
		}
		ctx.JSON(status, body)
		return
	}
	switch {
	case errors.Is(err, channel.ErrUnsupported):
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{"error": "unsupported_on_channel", "message": err.Error()})
	case errors.Is(err, channel.ErrNotImplemented):
		ctx.JSON(http.StatusNotImplemented, gin.H{"error": "not_implemented_on_channel", "message": err.Error()})
	default:
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
