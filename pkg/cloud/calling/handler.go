package calling

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
	r.POST("/calls", h.Start)
	r.GET("/calls/:id", h.Get)
	r.POST("/calls/:id/actions", h.Action)
	r.POST("/calls/:id/media", h.MediaUnsupported)
	r.GET("/permissions", h.Permissions)
	r.POST("/permissions", h.UpdatePermissions)
	r.GET("/settings", h.Settings)
	r.POST("/settings", h.UpdateSettings)
	r.GET("/events", h.ListEvents)
}

func (h *Handler) instance(c *gin.Context) (*instance_model.Instance, bool) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return nil, false
	}
	return inst, true
}

func (h *Handler) Start(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	var in map[string]any
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	raw, err := h.svc.Start(c.Request.Context(), inst, in)
	if err != nil {
		writeCallError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *Handler) Get(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	raw, err := h.svc.Get(c.Request.Context(), inst, c.Param("id"))
	if err != nil {
		writeCallError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *Handler) Action(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	var in struct {
		Action string         `json:"action"`
		Extra  map[string]any `json:"extra"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	raw, err := h.svc.Action(c.Request.Context(), inst, c.Param("id"), in.Action, in.Extra)
	if err != nil {
		writeCallError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

// MediaUnsupported is intentionally explicit: accepting a WebRTC SDP/offer
// without a TURN/SFU service would be a silent failure.
func (h *Handler) MediaUnsupported(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error":   "not_implemented_on_channel",
		"message": "call media requires the separate WebRTC/SIP service; this control plane does not terminate media",
	})
}

func (h *Handler) Permissions(c *gin.Context) {
	h.raw(c, func(inst *instance_model.Instance) ([]byte, error) {
		return h.svc.Permissions(c.Request.Context(), inst)
	})
}

func (h *Handler) UpdatePermissions(c *gin.Context) {
	h.update(c, func(inst *instance_model.Instance, payload map[string]any) ([]byte, error) {
		return h.svc.UpdatePermissions(c.Request.Context(), inst, payload)
	})
}

func (h *Handler) Settings(c *gin.Context) {
	h.raw(c, func(inst *instance_model.Instance) ([]byte, error) {
		return h.svc.Settings(c.Request.Context(), inst)
	})
}

func (h *Handler) UpdateSettings(c *gin.Context) {
	h.update(c, func(inst *instance_model.Instance, payload map[string]any) ([]byte, error) {
		return h.svc.UpdateSettings(c.Request.Context(), inst, payload)
	})
}

func (h *Handler) ListEvents(c *gin.Context) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	items, err := h.svc.ListEvents(inst.Id, c.Query("callId"), 100)
	if err != nil {
		writeCallError(c, err)
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
		writeCallError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *Handler) update(c *gin.Context, fn func(*instance_model.Instance, map[string]any) ([]byte, error)) {
	inst, ok := h.instance(c)
	if !ok {
		return
	}
	var in map[string]any
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	raw, err := fn(inst, in)
	if err != nil {
		writeCallError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func writeCallError(c *gin.Context, err error) {
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
