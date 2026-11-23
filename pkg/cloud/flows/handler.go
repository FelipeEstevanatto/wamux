package flows

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/felipeestevanatto/wamux/pkg/channel"
	cloud_service "github.com/felipeestevanatto/wamux/pkg/cloud"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

// Handler exposes the Flows HTTP surface. Routes are registered only when
// CLOUD_FLOWS is enabled.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("", h.List)
	r.POST("", h.Create)
	r.GET("/events", h.ListEvents)
	r.GET("/:id", h.Get)
	r.POST("/:id", h.Update)
	r.POST("/:id/send", h.Send)
	r.POST("/:id/publish", h.Publish)
	r.POST("/:id/deprecate", h.Deprecate)
	r.DELETE("/:id", h.Delete)
	r.GET("/:id/assets", h.Assets)
}

func (h *Handler) RegisterPublicRoutes(eng *gin.Engine) {
	eng.POST("/webhook/cloud/flows/:instanceId", h.Callback)
}

func (h *Handler) instance(c *gin.Context) (*instance_model.Instance, bool) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return nil, false
	}
	return inst, true
}

func (h *Handler) List(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	items, err := h.svc.List(c.Request.Context(), inst)
	if err != nil {
		writeFlowError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) Get(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	item, err := h.svc.Get(c.Request.Context(), inst, c.Param("id"))
	if err != nil {
		writeFlowError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) Create(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	var in map[string]any
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := h.svc.Create(c.Request.Context(), inst, in)
	if err != nil {
		writeFlowError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) Update(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	var in map[string]any
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := h.svc.Update(c.Request.Context(), inst, c.Param("id"), in)
	if err != nil {
		writeFlowError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) Publish(c *gin.Context) {
	h.action(c, h.svc.Publish)
}

func (h *Handler) Deprecate(c *gin.Context) {
	h.action(c, h.svc.Deprecate)
}

func (h *Handler) action(c *gin.Context, fn func(context.Context, *instance_model.Instance, string) (json.RawMessage, error)) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	raw, err := fn(c.Request.Context(), inst, c.Param("id"))
	if err != nil {
		writeFlowError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *Handler) Delete(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), inst, c.Param("id")); err != nil {
		writeFlowError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handler) Assets(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	raw, err := h.svc.Assets(c.Request.Context(), inst, c.Param("id"))
	if err != nil {
		writeFlowError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *Handler) ListEvents(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	items, err := h.svc.ListEvents(inst.Id, c.Query("flowId"), 100)
	if err != nil {
		writeFlowError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

// Callback verifies Meta's X-Hub-Signature-256 over the exact raw bytes. The
// endpoint URL carries the instance UUID; the signature is the credential.
func (h *Handler) Callback(c *gin.Context) {
	const maxBody = 1 << 20
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "flow callback too large"})
		return
	}
	instanceID := c.Param("instanceId")
	secret, err := h.svc.cloud.AppSecretForInstance(instanceID)
	if err != nil || secret == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown instance"})
		return
	}
	if !cloud_service.VerifyWebhookSignature(secret, body, c.GetHeader("X-Hub-Signature-256")) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}
	var meta struct {
		FlowID string `json:"flow_id"`
		Event  string `json:"event"`
		Type   string `json:"type"`
	}
	_ = json.Unmarshal(body, &meta)
	eventType := first(meta.Event, meta.Type, "endpoint")
	if err := h.svc.RecordEndpointEvent(instanceID, meta.FlowID, eventType, body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not store flow callback"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func writeFlowError(c *gin.Context, err error) {
	var apiErr *cloud_service.APIError
	if errors.As(err, &apiErr) {
		status := apiErr.Status
		if status == 0 {
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{"error": apiErr.Code, "message": apiErr.Message, "details": apiErr.Details})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func (h *Handler) Send(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	var in struct {
		Number             string `json:"number"`
		Recipient          string `json:"recipient"`
		RecipientType      string `json:"recipientType"`
		Header             string `json:"header"`
		Body               string `json:"body"`
		Footer             string `json:"footer"`
		FlowToken          string `json:"flowToken"`
		FlowCTA            string `json:"flowCta"`
		FlowAction         string `json:"flowAction"`
		FlowActionPayload  any    `json:"flowActionPayload"`
		FlowMessageVersion string `json:"flowMessageVersion"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Number == "" && in.Recipient == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "number or recipient is required"})
		return
	}
	result, err := h.svc.Send(c.Request.Context(), inst, c.Param("id"), channel.FlowMessage{
		To: in.Number, Recipient: in.Recipient, RecipientType: in.RecipientType,
		Header: in.Header, Body: in.Body, Footer: in.Footer,
		FlowToken: in.FlowToken, FlowCTA: in.FlowCTA, FlowAction: in.FlowAction,
		FlowActionPayload: in.FlowActionPayload, FlowMessageVersion: in.FlowMessageVersion,
	})
	if err != nil {
		writeFlowError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": result})
}
