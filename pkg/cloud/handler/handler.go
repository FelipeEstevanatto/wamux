// Package handler exposes the Cloud API HTTP surface: Meta's public webhook,
// app administration, instance credential/registration/diagnostics endpoints
// and template management.
package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/felipeestevanatto/wamux/pkg/audit"
	cloud_service "github.com/felipeestevanatto/wamux/pkg/cloud"
	cloud_flows "github.com/felipeestevanatto/wamux/pkg/cloud/flows"
	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc   *cloud_service.Service
	cfg   *config.Config
	flows *cloud_flows.Handler
}

func New(svc *cloud_service.Service, cfg *config.Config) *Handler {
	h := &Handler{svc: svc, cfg: cfg}
	if cfg != nil && cfg.CloudFlowsEnabled && svc != nil {
		flowSvc := cloud_flows.New(svc)
		svc.AddWebhookHook(flowSvc.HandleCloudChange)
		h.flows = cloud_flows.NewHandler(flowSvc)
	}
	return h
}

// RegisterFeatureRoutes mounts the separate opt-in programs on an already
// authenticated Cloud router group.
func (h *Handler) RegisterFeatureRoutes(r *gin.RouterGroup) {
	if h == nil {
		return
	}
	if h.flows != nil {
		h.flows.RegisterRoutes(r.Group("/flows"))
	}
}

// RegisterPublicFeatureRoutes mounts signed feature callbacks. Each callback
// verifies Meta's signature itself.
func (h *Handler) RegisterPublicFeatureRoutes(eng *gin.Engine) {
	if h == nil {
		return
	}
	if h.flows != nil {
		h.flows.RegisterPublicRoutes(eng)
	}
}

func (h *Handler) WebhookVerify(c *gin.Context) {
	mode := c.Query("hub.mode")
	token := c.Query("hub.verify_token")
	challenge := c.Query("hub.challenge")
	appID := c.Param("appId")
	if mode != "subscribe" {
		c.String(http.StatusBadRequest, "invalid mode")
		return
	}
	ok := false
	if appID != "" {
		ok = h.svc.VerifyTokenForApp(appID, token)
	} else {
		ok = h.svc.VerifyToken(token)
	}
	if !ok {
		c.String(http.StatusForbidden, "invalid token")
		return
	}
	c.String(http.StatusOK, challenge)
}

func (h *Handler) WebhookReceive(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "webhook processing panic"})
		}
	}()
	limit := int64(4 << 20)
	if h.cfg != nil && h.cfg.CloudAPIMaxWebhookBytes > 0 {
		limit = h.cfg.CloudAPIMaxWebhookBytes
	}
	data, err := io.ReadAll(io.LimitReader(c.Request.Body, limit+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if int64(len(data)) > limit {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "webhook payload too large"})
		return
	}
	err = h.svc.ReceiveWebhook(data, c.GetHeader("X-Hub-Signature-256"), c.Param("appId"))
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	case errors.Is(err, cloud_service.ErrInvalidSignature):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
	case errors.Is(err, cloud_service.ErrUnknownApp):
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown app"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func (h *Handler) ListApps(c *gin.Context) {
	apps, err := h.svc.Repository().ListApps()
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": apps})
}

func (h *Handler) SaveApp(c *gin.Context) {
	var in struct {
		AppID        string `json:"appId"`
		AppSecret    string `json:"appSecret"`
		VerifyToken  string `json:"verifyToken"`
		GraphVersion string `json:"graphVersion"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.AppID == "" {
		in.AppID = c.Param("appId")
	}
	if err := h.svc.SaveApp(in.AppID, in.AppSecret, in.VerifyToken, in.GraphVersion); err != nil {
		writeError(c, err)
		return
	}
	audit.Record("admin", "cloud.app.save", in.AppID, map[string]any{"graphVersion": in.GraphVersion})
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{
		"appId": in.AppID, "graphVersion": in.GraphVersion,
		"appSecretSet": true, "verifyTokenSet": true,
	}})
}

func (h *Handler) DeleteApp(c *gin.Context) {
	appID := c.Param("appId")
	if err := h.svc.DeleteApp(appID); err != nil {
		writeError(c, err)
		return
	}
	audit.Record("admin", "cloud.app.delete", appID, nil)
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handler) GetCredentials(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": h.svc.CredentialSummary(inst)})
}

func (h *Handler) SetCredentials(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	var in cloud_service.CredentialsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.SaveCredentials(inst.Id, in); err != nil {
		writeError(c, err)
		return
	}
	audit.Record("instance", "cloud.credentials.update", inst.Id, map[string]any{
		"wabaId": in.WABAID, "phoneNumberId": in.PhoneNumberID,
	})
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": h.svc.CredentialSummary(inst)})
}

func (h *Handler) Verify(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	status, err := h.svc.Connect(inst)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": status})
}

func (h *Handler) Register(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	var in struct {
		PIN string `json:"pin"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status, err := h.svc.Register(inst, in.PIN)
	if err != nil {
		writeError(c, err)
		return
	}
	audit.Record("instance", "cloud.phone.register", inst.Id, nil)
	c.JSON(http.StatusOK, gin.H{"data": status})
}

func (h *Handler) MarkRead(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	var in struct {
		MessageID string `json:"messageId"`
		Typing    bool   `json:"typing"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), inst, in.MessageID, in.Typing); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handler) Diagnostics(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	diag, err := h.svc.Diagnostics(inst)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": diag})
}

func (h *Handler) ListTemplates(c *gin.Context) {
	inst, _ := c.MustGet("instance").(*instance_model.Instance)
	items, err := h.svc.ListTemplates(inst)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) SyncTemplates(c *gin.Context) {
	inst, _ := c.MustGet("instance").(*instance_model.Instance)
	items, err := h.svc.SyncTemplates(inst)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) CreateTemplate(c *gin.Context) {
	inst, _ := c.MustGet("instance").(*instance_model.Instance)
	var in cloud_service.TemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := h.svc.CreateTemplate(inst, in)
	if err != nil {
		writeError(c, err)
		return
	}
	audit.Record("instance", "cloud.template.create", inst.Id, map[string]any{"name": in.Name, "language": in.Language})
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) UpdateTemplate(c *gin.Context) {
	inst, _ := c.MustGet("instance").(*instance_model.Instance)
	var in cloud_service.TemplateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := h.svc.UpdateTemplate(inst, c.Param("name"), c.Param("language"), in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) DeleteTemplate(c *gin.Context) {
	inst, _ := c.MustGet("instance").(*instance_model.Instance)
	if err := h.svc.DeleteTemplate(inst, c.Param("name"), c.Param("language")); err != nil {
		writeError(c, err)
		return
	}
	audit.Record("instance", "cloud.template.delete", inst.Id, map[string]any{"name": c.Param("name")})
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func writeError(c *gin.Context, err error) {
	var apiErr *cloud_service.APIError
	if errors.As(err, &apiErr) {
		body := gin.H{"error": apiErr.Code, "message": apiErr.Message}
		if len(apiErr.Details) > 0 {
			body["details"] = apiErr.Details
		}
		status := apiErr.Status
		if status == 0 {
			status = http.StatusInternalServerError
		}
		if apiErr.RetryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(apiErr.RetryAfter))
		}
		c.JSON(status, body)
		return
	}
	if strings.Contains(strings.ToLower(err.Error()), "required") {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func (h *Handler) GetContact(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	detail, err := h.svc.ContactDetail(inst.Id, c.Param("contactId"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": detail})
}

func (h *Handler) ListIdentityEvents(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	events, err := h.svc.ListIdentityEvents(inst.Id, limit)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": events})
}

func (h *Handler) ReconcileContact(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	var in struct {
		WaID     string `json:"waId"`
		OldBSUID string `json:"oldBsuid"`
		NewBSUID string `json:"newBsuid"`
		Parent   string `json:"parentBsuid"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.ApplyIdentityUpdate(inst.Id, cloud_service.IdentityUpdate{
		WAID: in.WaID, OldBSUID: in.OldBSUID, NewBSUID: in.NewBSUID,
		ParentBSUID: in.Parent, EventType: "manual_reconcile",
	}); err != nil {
		writeError(c, err)
		return
	}
	audit.Record("instance", "cloud.contact.reconcile", inst.Id, map[string]any{"waId": in.WaID})
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handler) ListAccountEvents(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	events, err := h.svc.ListAccountEvents(inst.Id, c.Query("field"), limit, offset)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": events})
}

func (h *Handler) GetAccountEvent(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	event, err := h.svc.GetAccountEvent(inst.Id, c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": event})
}

func (h *Handler) GetTemplate(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	item, err := h.svc.GetTemplate(inst, c.Param("name"), c.Param("language"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

// --- Business Management ----------------------------------------------------

func (h *Handler) ListBusinessPhoneNumbers(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	items, err := h.svc.ListBusinessPhoneNumbers(inst)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) GetBusinessPhone(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	item, err := h.svc.GetBusinessPhone(inst, c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) RegisterBusinessPhone(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	var in struct {
		PIN string `json:"pin"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.RegisterBusinessPhone(inst, c.Param("id"), in.PIN); err != nil {
		writeError(c, err)
		return
	}
	audit.Record("instance", "cloud.business.phone.register", inst.Id, map[string]any{"phoneNumberId": c.Param("id")})
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handler) DeregisterBusinessPhone(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	if err := h.svc.DeregisterBusinessPhone(inst, c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	audit.Record("instance", "cloud.business.phone.deregister", inst.Id, map[string]any{"phoneNumberId": c.Param("id")})
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handler) RequestBusinessCode(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	var in struct {
		Method   string `json:"method"`
		Language string `json:"language"`
	}
	_ = c.ShouldBindJSON(&in)
	if err := h.svc.RequestBusinessCode(inst, c.Param("id"), in.Method, in.Language); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handler) VerifyBusinessCode(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	var in struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.VerifyBusinessCode(inst, c.Param("id"), in.Code); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *Handler) GetBusinessWABA(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	item, err := h.svc.GetBusinessWABA(inst)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) GetBusinessAnalytics(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	raw, err := h.svc.GetBusinessAnalytics(inst, c.Query("start"), c.Query("end"), c.Query("granularity"), c.Query("phoneNumbers"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

func (h *Handler) ListBusinessCatalogs(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	items, err := h.svc.ListBusinessCatalogs(inst)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) CreateBusinessCatalog(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := h.svc.CreateBusinessCatalog(inst, in.Name)
	if err != nil {
		writeError(c, err)
		return
	}
	audit.Record("instance", "cloud.business.catalog.create", inst.Id, map[string]any{"name": in.Name})
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) ListBusinessProducts(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	items, err := h.svc.ListBusinessProducts(inst, c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (h *Handler) CreateBusinessProduct(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	var in map[string]any
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := h.svc.CreateBusinessProduct(inst, c.Param("id"), in)
	if err != nil {
		writeError(c, err)
		return
	}
	audit.Record("instance", "cloud.business.product.create", inst.Id, map[string]any{"catalogId": c.Param("id")})
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (h *Handler) Deregister(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	status, err := h.svc.Deregister(inst)
	if err != nil {
		writeError(c, err)
		return
	}
	audit.Record("instance", "cloud.phone.deregister", inst.Id, nil)
	c.JSON(http.StatusOK, gin.H{"data": status})
}

// --- Embedded Signup --------------------------------------------------------

func (h *Handler) StartEmbeddedSignup(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	start, err := h.svc.StartEmbeddedSignup(inst)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": start})
}

// EmbeddedCallback is public because Meta redirects the browser here. The
// one-time state binds the result to the instance that started the flow.
func (h *Handler) EmbeddedCallback(c *gin.Context) {
	assets, err := h.svc.CompleteEmbeddedSignup(c.Query("code"), c.Query("state"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"state": c.Query("state"), "assets": assets}})
}

func (h *Handler) EmbeddedAssets(c *gin.Context) {
	assets, err := h.svc.EmbeddedAssets(c.Query("state"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": assets})
}

func (h *Handler) SelectEmbeddedAsset(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	var in struct {
		State         string `json:"state"`
		WABAID        string `json:"wabaId"`
		PhoneNumberID string `json:"phoneNumberId"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status, err := h.svc.SelectEmbeddedAsset(inst, in.State, in.WABAID, in.PhoneNumberID)
	if err != nil {
		writeError(c, err)
		return
	}
	audit.Record("instance", "cloud.embedded.select", inst.Id, map[string]any{
		"wabaId": in.WABAID, "phoneNumberId": in.PhoneNumberID,
	})
	c.JSON(http.StatusOK, gin.H{"data": status})
}

// AdminDiagnostics lets an operator run doctor against an instance id without
// knowing that instance's token. It is mounted behind AuthAdmin.
func (h *Handler) AdminDiagnostics(c *gin.Context) {
	inst, err := h.svc.ResolveInstance(c.Param("instanceId"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}
	diag, err := h.svc.Diagnostics(inst)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": diag})
}

func (h *Handler) ListBusinessWABAs(c *gin.Context) {
	inst, ok := c.MustGet("instance").(*instance_model.Instance)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}
	owned := c.DefaultQuery("scope", "owned") != "client"
	items, err := h.svc.ListBusinessWABAs(inst, owned)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}
