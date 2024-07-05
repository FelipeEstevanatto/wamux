package instance_handler

import (
	"net/http"

	"github.com/Zapbox-API/evolution-go/pkg/config"
	instance_model "github.com/Zapbox-API/evolution-go/pkg/instances/model"
	instance_service "github.com/Zapbox-API/evolution-go/pkg/instances/service"
	"github.com/gin-gonic/gin"
)

type InstanceHandler interface {
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

type instanceHandler struct {
	config          *config.Config
	instanceService instance_service.InstanceService
}

func (i *instanceHandler) Create(ctx *gin.Context) {
	var data *instance_service.CreateStruct
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

	err = i.instanceService.Create(data)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": data})
}

func (i *instanceHandler) Connect(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *instance_service.ConnectStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updateInstance, err := i.instanceService.Connect(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Set("instance", updateInstance)

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": updateInstance})
}

func (i *instanceHandler) Disconnect(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	updateInstance, err := i.instanceService.Disconnect(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Set("instance", updateInstance)

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": updateInstance})
}

func (i *instanceHandler) Logout(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	updateInstance, err := i.instanceService.Logout(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Set("instance", updateInstance)

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": updateInstance})
}

func (i *instanceHandler) Status(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	status, err := i.instanceService.Status(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": status})
}

func (i *instanceHandler) Qr(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	qrcode, err := i.instanceService.GetQr(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": qrcode})
}

func (i *instanceHandler) Pair(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *instance_service.PairStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone is required"})
		return
	}

	pairingCode, err := i.instanceService.Pair(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": pairingCode})
}

func (i *instanceHandler) All(ctx *gin.Context) {
	instances, err := i.instanceService.GetAll()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": instances})
}

func (i *instanceHandler) Delete(ctx *gin.Context) {
	instanceName := ctx.Param("instanceName")

	if instanceName == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "instanceName is required"})
		return
	}

	err := i.instanceService.Delete(instanceName)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (i *instanceHandler) DeleteProxy(ctx *gin.Context) {
	instanceName := ctx.Param("instanceName")

	if instanceName == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	err := i.instanceService.RemoveProxy(instanceName)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

func NewInstanceHandler(instanceService instance_service.InstanceService, config *config.Config) InstanceHandler {
	return &instanceHandler{instanceService: instanceService, config: config}
}
