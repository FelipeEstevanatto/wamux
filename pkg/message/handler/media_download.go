package message_handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	_ "github.com/felipeestevanatto/wamux/pkg/docmodels" // Swagger types.
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
)

const (
	// downloadLinkTTL only has to cover the click: a download already running is not
	// cut off when its link expires.
	downloadLinkTTL = 5 * time.Minute

	// downloadWindow replaces the server's write timeout, which a large file on a
	// slow connection would run into.
	downloadWindow = 2 * time.Hour
)

func signDownload(secret, instanceID, messageID string, expires int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "wamux-download|%s|%s|%d", instanceID, messageID, expires)
	return hex.EncodeToString(mac.Sum(nil))
}

// downloadLink builds a link to one file of one instance.
func downloadLink(secret, instanceID, messageID string, now time.Time) string {
	expires := now.Add(downloadLinkTTL).Unix()
	query := url.Values{
		"instance": {instanceID},
		"expires":  {strconv.FormatInt(expires, 10)},
		"sig":      {signDownload(secret, instanceID, messageID, expires)},
	}
	return "/download/" + url.PathEscape(messageID) + "?" + query.Encode()
}

func validDownload(secret, instanceID, messageID, expiry, signature string, now time.Time) bool {
	if secret == "" || instanceID == "" {
		return false
	}
	expires, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil || now.Unix() >= expires {
		return false
	}
	return hmac.Equal([]byte(signature), []byte(signDownload(secret, instanceID, messageID, expires)))
}

// MediaDownloadLink creates a short-lived link to a stored file
// @Summary Create a download link for stored media
// @Description Returns a link valid for 5 minutes that downloads one stored file without an API key, so a browser can save it straight to disk.
// @Tags Message
// @Produce json
// @Param messageId path string true "Message id"
// @Success 200 {object} docmodels.Envelope "The link"
// @Failure 404 {object} docmodels.ErrorResponse "No stored file"
// @Router /chat/media/{messageId}/link [post]
func (m *messageHandler) MediaDownloadLink(ctx *gin.Context) {
	instance, ok := ctx.MustGet("instance").(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	messageID := ctx.Param("messageId")

	// Checked now so a missing file is an error here, not a broken download.
	file, _, _, err := m.messageService.GetStoredMedia(messageID, instance)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "media not found"})
		return
	}
	file.Close()

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{
		"url": downloadLink(m.downloadSecret, instance.Id, messageID, time.Now()),
	}})
}

// DownloadStoredMedia serves a stored file through a signed link
// @Summary Download stored media through a signed link
// @Description Streams one stored file as an attachment, with Range support. The link comes from POST /chat/media/{messageId}/link.
// @Tags Message
// @Produce octet-stream
// @Param messageId path string true "Message id"
// @Param instance query string true "Instance id"
// @Param expires query int true "Expiry, in Unix seconds"
// @Param sig query string true "Signature"
// @Success 200 {file} binary "The file"
// @Failure 403 {object} docmodels.ErrorResponse "Invalid or expired link"
// @Failure 404 {object} docmodels.ErrorResponse "No stored file"
// @Router /download/{messageId} [get]
func (m *messageHandler) DownloadStoredMedia(ctx *gin.Context) {
	messageID := ctx.Param("messageId")
	instanceID := ctx.Query("instance")
	if !validDownload(m.downloadSecret, instanceID, messageID, ctx.Query("expires"), ctx.Query("sig"), time.Now()) {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "invalid or expired link"})
		return
	}

	// Not every writer supports deadlines. The server's default then applies.
	_ = http.NewResponseController(ctx.Writer).SetWriteDeadline(time.Now().Add(downloadWindow))

	instance := &instance_model.Instance{Id: instanceID}
	ctx.Header("Content-Disposition", attachmentHeader(m.messageService.StoredMediaName(messageID, instance), messageID))
	// The file came from a stranger: it must never run as a page on this origin.
	ctx.Header("X-Content-Type-Options", "nosniff")
	m.serveStoredMedia(ctx, messageID, instance)
}

// attachmentHeader keeps only the last path part: the name came from the sender.
func attachmentHeader(name, fallback string) string {
	name = strings.TrimSpace(name[strings.LastIndexAny(name, `/\`)+1:])
	if name == "" {
		name = fallback
	}
	return mime.FormatMediaType("attachment", map[string]string{"filename": name})
}
