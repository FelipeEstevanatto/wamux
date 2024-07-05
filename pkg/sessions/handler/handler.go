package session_handler

import (
	"net/http"

	"github.com/Zapbox-API/evolution-go/pkg/config"
	instance_model "github.com/Zapbox-API/evolution-go/pkg/instances/model"
	session_service "github.com/Zapbox-API/evolution-go/pkg/sessions/service"
	"github.com/gin-gonic/gin"
)

type SessionHandler interface {
	Create(data *gin.Context)
	Connect(data *gin.Context)
	Disconnect(data *gin.Context)
	Logout(data *gin.Context)
	Delete(data *gin.Context)
	Status(data *gin.Context)
	Qr(data *gin.Context)
	All(data *gin.Context)
	Pair(data *gin.Context)
	DeleteProxy(data *gin.Context)
}

type sessionHandler struct {
	config         *config.Config
	sessionService session_service.SessionService
}

func (s *sessionHandler) Create(ctx *gin.Context) {
	var data *session_service.CreateStruct
	err := ctx.ShouldBindBodyWithJSON(&data)

	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if data.Token == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	if data.Proxy != nil {
		if data.Proxy.Port == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "proxy port is required"})
			return
		}

		if data.Proxy.Password == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "proxy password is required"})
			return
		}

		if data.Proxy.Username == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "proxy username is required"})
			return
		}

		if data.Proxy.Address == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "proxy address is required"})
			return
		}
	}

	err = s.sessionService.Create(data)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": data})
}

func (s *sessionHandler) Connect(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *session_service.ConnectStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updateInstance, err := s.sessionService.Connect(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Set("instance", updateInstance)

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": updateInstance})
}

func (s *sessionHandler) Disconnect(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	updateInstance, err := s.sessionService.Disconnect(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Set("instance", updateInstance)

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": updateInstance})
}

func (s *sessionHandler) Logout(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	updateInstance, err := s.sessionService.Logout(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Set("instance", updateInstance)

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": updateInstance})
}

func (s *sessionHandler) Status(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	status, err := s.sessionService.Status(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": status})
}

func (s *sessionHandler) Qr(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	qrcode, err := s.sessionService.GetQr(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": qrcode})
}

func (s *sessionHandler) Pair(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *session_service.PairStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone is required"})
		return
	}

	pairingCode, err := s.sessionService.Pair(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": pairingCode})
}

func (s *sessionHandler) All(ctx *gin.Context) {
	instances, err := s.sessionService.GetAll()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": instances})
}

func (s *sessionHandler) Delete(ctx *gin.Context) {
	instanceName := ctx.Param("instanceName")

	if instanceName == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "instanceName is required"})
		return
	}

	err := s.sessionService.Delete(instanceName)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (s *sessionHandler) DeleteProxy(ctx *gin.Context) {
	instanceName := ctx.Param("instanceName")

	if instanceName == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	err := s.sessionService.RemoveProxy(instanceName)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

func NewSessionHandler(sessionService session_service.SessionService, config *config.Config) SessionHandler {
	return &sessionHandler{sessionService: sessionService, config: config}
}
