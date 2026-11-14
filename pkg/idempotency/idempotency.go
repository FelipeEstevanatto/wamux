// Package idempotency implements the Idempotency-Key contract for send
// requests. A retry of the same key and body replays the first response
// instead of sending a second WhatsApp message.
package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	docmodels "github.com/felipeestevanatto/wamux/pkg/docmodels"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	message_model "github.com/felipeestevanatto/wamux/pkg/message/model"
	"github.com/gin-gonic/gin"
)

// Keep docmodels referenced for swag's source parser.
var _ = docmodels.Envelope{}

const (
	KeyHeader       = "Idempotency-Key"
	OperationHeader = "X-Wamux-Operation-Id"

	maxKeyLength      = 255
	maxRequestBody    = 64 << 20 // 64 MiB: enough for normal JSON media
	maxStoredResponse = 1 << 20  // 1 MiB
)

// Store is the persistence slice the middleware needs. messageRepository
// implements it; tests provide a fake.
type Store interface {
	BeginSendOperation(ctx context.Context, instanceId, key, payloadHash string) (*message_model.SendOperation, bool, error)
	CompleteSendOperation(ctx context.Context, id, state string, statusCode int, response []byte, messageID, errText string) error
	GetSendOperation(ctx context.Context, instanceId, id string) (*message_model.SendOperation, error)
	ListSendOperations(ctx context.Context, instanceId, state string, limit int) ([]message_model.SendOperation, error)
}

// Handler is the middleware and the operation-status endpoint.
type Handler struct {
	store  Store
	logger *logger_wrapper.LoggerManager
}

func New(store Store, logger *logger_wrapper.LoggerManager) *Handler {
	return &Handler{store: store, logger: logger}
}

// Middleware wraps a JSON send handler. Without an Idempotency-Key it is a
// no-op, so existing clients are unaffected. Multipart uploads are rejected
// when a key is present because the key would have to hash the whole file.
func (h *Handler) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(c.GetHeader(KeyHeader))
		if key == "" {
			c.Next()
			return
		}
		if h.store == nil {
			c.Next()
			return
		}
		if len(key) > maxKeyLength {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Idempotency-Key is too long"})
			return
		}
		if !strings.Contains(c.ContentType(), "application/json") {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"error": "Idempotency-Key is only supported for JSON requests",
			})
			return
		}

		body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxRequestBody+1))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "could not read request body"})
			return
		}
		if len(body) > maxRequestBody {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": "request body is too large for Idempotency-Key",
			})
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		hash := payloadHash(c.Request.Method, c.FullPath(), canonicalJSON(body))
		instance, _ := c.MustGet("instance").(*instance_model.Instance)
		if instance == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
			return
		}

		op, created, err := h.store.BeginSendOperation(c.Request.Context(), instance.Id, key, hash)
		if err != nil {
			h.log(c, "begin failed: %v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "could not record idempotent operation"})
			return
		}
		if !created {
			if op.PayloadHash != hash {
				c.AbortWithStatusJSON(http.StatusConflict, gin.H{
					"error":        "Idempotency-Key was already used with a different request",
					"operation_id": op.Id,
				})
				return
			}
			if op.State == "pending" {
				c.Header(OperationHeader, op.Id)
				c.AbortWithStatusJSON(http.StatusAccepted, gin.H{
					"status":       "pending",
					"operation_id": op.Id,
				})
				return
			}
			c.Header(OperationHeader, op.Id)
			h.replay(c, op)
			c.Abort()
			return
		}

		c.Header(OperationHeader, op.Id)
		writer := &captureWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		c.Next()
		c.Writer = writer.ResponseWriter

		status := writer.Status()
		state := "sent"
		if status >= 400 {
			state = "failed"
		}
		response := writer.body.Bytes()
		if len(response) > maxStoredResponse {
			response = nil
		}
		errText := ""
		if state == "failed" {
			errText = truncate(string(response), 1024)
		}
		if err := h.store.CompleteSendOperation(c.Request.Context(), op.Id, state, status, response, messageIDOf(response), errText); err != nil {
			h.log(c, "complete failed: %v", err)
		}
	}
}

func (h *Handler) replay(c *gin.Context, op *message_model.SendOperation) {
	status := op.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	if len(op.Response) == 0 {
		c.JSON(status, gin.H{"status": op.State, "operation_id": op.Id})
		return
	}
	c.Data(status, "application/json; charset=utf-8", op.Response)
}

// Operation serves GET /send/operations/:operationId, scoped to the
// authenticated instance.
// @Summary Get an idempotent send operation
// @Description Returns the state and result of a send made with an Idempotency-Key.
// @Tags Send Message
// @Produce json
// @Param operationId path string true "Operation id"
// @Success 200 {object} docmodels.Envelope "operation"
// @Failure 404 {object} docmodels.ErrorResponse "Unknown operation"
// @Router /send/operations/{operationId} [get]
func (h *Handler) Operation(c *gin.Context) {
	instance, _ := c.MustGet("instance").(*instance_model.Instance)
	if instance == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}
	op, err := h.store.GetSendOperation(c.Request.Context(), instance.Id, c.Param("operationId"))
	if err != nil {
		h.log(c, "lookup failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not read operation"})
		return
	}
	if op == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "IDEMPOTENCY_OPERATION_NOT_FOUND"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"id":              op.Id,
			"state":           op.State,
			"status_code":     op.StatusCode,
			"message_id":      op.MessageID,
			"error":           op.Error,
			"created_at":      op.CreatedAt,
			"updated_at":      op.UpdatedAt,
			"idempotency_key": op.IdempotencyKey,
		},
	})
}

// Operations lists this instance's idempotent sends, oldest first. Filter with
// ?state=pending after a restart to find requests whose outcome is unknown.
// @Summary List idempotent send operations
// @Description Lists this instance's Idempotency-Key records, oldest first.
// @Tags Send Message
// @Produce json
// @Param state query string false "pending, sent or failed"
// @Param limit query int false "Max records (default 50, max 500)"
// @Success 200 {object} docmodels.Envelope "operations"
// @Router /send/operations [get]
func (h *Handler) Operations(c *gin.Context) {
	instance, _ := c.MustGet("instance").(*instance_model.Instance)
	if instance == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}
	state := strings.TrimSpace(c.Query("state"))
	switch state {
	case "", "pending", "sent", "failed":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "state must be pending, sent or failed"})
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	ops, err := h.store.ListSendOperations(c.Request.Context(), instance.Id, state, limit)
	if err != nil {
		h.log(c, "list failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list operations"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": ops})
}

// canonicalJSON normalizes object key order and whitespace so a retry that is
// semantically identical hashes the same. Numbers keep their literal form via
// UseNumber; invalid or non-single-value JSON falls back to the raw bytes.
func canonicalJSON(body []byte) []byte {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return body
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return body
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return canonical
}

func payloadHash(method, path string, body []byte) string {
	sum := sha256.New()
	sum.Write([]byte(method))
	sum.Write([]byte(" "))
	sum.Write([]byte(path))
	sum.Write([]byte("\n"))
	sum.Write(body)
	return hex.EncodeToString(sum.Sum(nil))
}

func messageIDOf(response []byte) string {
	if len(response) == 0 {
		return ""
	}
	var envelope struct {
		Data struct {
			Info struct {
				ID string `json:"ID"`
			} `json:"Info"`
		} `json:"data"`
	}
	if json.Unmarshal(response, &envelope) != nil {
		return ""
	}
	return envelope.Data.Info.ID
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func (h *Handler) log(c *gin.Context, format string, args ...any) {
	if h.logger == nil {
		return
	}
	instance, _ := c.MustGet("instance").(*instance_model.Instance)
	id := ""
	if instance != nil {
		id = instance.Id
	}
	h.logger.GetLogger(id).LogError("[%s] idempotency: "+format, append([]any{id}, args...)...)
}

// captureWriter records the response while forwarding it to the caller.
type captureWriter struct {
	gin.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *captureWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *captureWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.body.Len() < maxStoredResponse {
		room := maxStoredResponse - w.body.Len()
		if room > len(p) {
			room = len(p)
		}
		w.body.Write(p[:room])
	}
	return w.ResponseWriter.Write(p)
}

func (w *captureWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

func (w *captureWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}
