package groups

import (
	"errors"
	"net/http"

	cloud_service "github.com/felipeestevanatto/wamux/pkg/cloud"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	r.GET("", h.List)
	r.POST("", h.Create)
	r.GET("/events", h.ListEvents)
	r.GET("/:id", h.Get)
	r.DELETE("/:id", h.Delete)
	r.GET("/:id/participants", h.Participants)
	r.POST("/:id/participants", h.AddParticipants)
	r.DELETE("/:id/participants", h.RemoveParticipant)
	r.POST("/:id/invite-link", h.InviteLink)
	r.GET("/:id/join-requests", h.JoinRequests)
	r.POST("/:id/join-requests/:requestId", h.DecideJoinRequest)
	r.POST("/:id/settings", h.UpdateSettings)
	r.POST("/:id/messages", h.Send)
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
		writeGroupError(c, err)
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
		writeGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) Create(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	var in struct {
		Subject      string   `json:"subject"`
		Description  string   `json:"description"`
		Participants []string `json:"participants"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := h.svc.Create(c.Request.Context(), inst, in.Subject, in.Description, in.Participants)
	if err != nil {
		writeGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) Delete(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), inst, c.Param("id")); err != nil {
		writeGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handler) Participants(c *gin.Context) {
	h.raw(c, func(inst *instance_model.Instance) ([]byte, error) {
		return h.svc.Participants(c.Request.Context(), inst, c.Param("id"))
	})
}

func (h *Handler) AddParticipants(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	var in struct {
		Participants []string `json:"participants"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	raw, err := h.svc.AddParticipants(c.Request.Context(), inst, c.Param("id"), in.Participants)
	if err != nil {
		writeGroupError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *Handler) RemoveParticipant(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	participant := c.Query("participant")
	if participant == "" {
		participant = c.Query("participants")
	}
	raw, err := h.svc.RemoveParticipant(c.Request.Context(), inst, c.Param("id"), participant)
	if err != nil {
		writeGroupError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *Handler) InviteLink(c *gin.Context) {
	h.raw(c, func(inst *instance_model.Instance) ([]byte, error) {
		return h.svc.InviteLink(c.Request.Context(), inst, c.Param("id"))
	})
}

func (h *Handler) JoinRequests(c *gin.Context) {
	h.raw(c, func(inst *instance_model.Instance) ([]byte, error) {
		return h.svc.JoinRequests(c.Request.Context(), inst, c.Param("id"))
	})
}

func (h *Handler) DecideJoinRequest(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	var in struct {
		Action string `json:"action"`
	}
	_ = c.ShouldBindJSON(&in)
	raw, err := h.svc.DecideJoinRequest(c.Request.Context(), inst, c.Param("id"), c.Param("requestId"), in.Action)
	if err != nil {
		writeGroupError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *Handler) UpdateSettings(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	var in map[string]any
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	raw, err := h.svc.UpdateSettings(c.Request.Context(), inst, c.Param("id"), in)
	if err != nil {
		writeGroupError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *Handler) Send(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	var in map[string]any
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	raw, err := h.svc.Send(c.Request.Context(), inst, c.Param("id"), in)
	if err != nil {
		writeGroupError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *Handler) ListEvents(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	items, err := h.svc.ListEvents(inst.Id, c.Query("groupId"), 100)
	if err != nil {
		writeGroupError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) raw(c *gin.Context, fn func(*instance_model.Instance) ([]byte, error)) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	raw, err := fn(inst)
	if err != nil {
		writeGroupError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func writeGroupError(c *gin.Context, err error) {
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
