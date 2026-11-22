package server_handler

import (
	"net/http"
	"strconv"

	"github.com/felipeestevanatto/wamux/pkg/audit"
	"github.com/gin-gonic/gin"
)

// Audit serves GET /server/audit: the newest local administrative actions.
// @Summary Administrative audit log
// @Description Returns the newest local administrative actions (instance create/delete, proxy/S3/HMAC changes, token reads). Admin key only.
// @Tags Server
// @Produce json
// @Param limit query int false "Entries to return (default 100, max 1000)" example(100)
// @Success 200 {object} docmodels.Envelope{data=[]audit.Entry} "audit entries"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /server/audit [get]
func (s *serverHandler) Audit(ctx *gin.Context) {
	limit, _ := strconv.Atoi(ctx.Query("limit"))
	entries, err := audit.List(limit)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if entries == nil {
		entries = []audit.Entry{}
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": entries})
}
