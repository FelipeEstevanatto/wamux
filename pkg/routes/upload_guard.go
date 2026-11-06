package routes

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// uploadWindow replaces the server's read and write timeouts on upload routes.
// Sending a large file means receiving it and then uploading it to WhatsApp, inside
// one request. Auth runs first, so only a known caller can hold a connection this long.
const uploadWindow = time.Hour

const multipartMemory = 32 << 20

// uploadGuard refuses a file over maxBytes and gives the request time to finish.
func uploadGuard(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		tooLarge := func() {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": fmt.Sprintf("file exceeds the limit of %d bytes (MAX_UPLOAD_BYTES)", maxBytes),
			})
		}
		if c.Request.ContentLength > maxBytes {
			tooLarge()
			return
		}

		// Not every writer supports deadlines. The server's defaults then apply.
		controller := http.NewResponseController(c.Writer)
		deadline := time.Now().Add(uploadWindow)
		_ = controller.SetReadDeadline(deadline)
		_ = controller.SetWriteDeadline(deadline)

		if !strings.HasPrefix(c.ContentType(), "multipart/form-data") {
			c.Next()
			return
		}

		// Parsed here because later steps ignore this error: a chunked body over the
		// limit would read as a request with no fields.
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		if err := c.Request.ParseMultipartForm(multipartMemory); err != nil {
			var limit *http.MaxBytesError
			if errors.As(err, &limit) {
				tooLarge()
				return
			}
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid multipart form"})
			return
		}
		c.Next()
	}
}
