package user_handler

import (
	"encoding/json"
	"net/http"

	instance_model "github.com/Zapbox-API/evolution-go/pkg/instance/model"
	user_service "github.com/Zapbox-API/evolution-go/pkg/user/service"
	"github.com/gin-gonic/gin"
)

type UserHandler interface {
	GetUser(ctx *gin.Context)
	CheckUser(ctx *gin.Context)
	GetAvatar(ctx *gin.Context)
	GetContacts(ctx *gin.Context)
	GetPrivacy(ctx *gin.Context)
	BlockContact(ctx *gin.Context)
	UnblockContact(ctx *gin.Context)
	GetBlockList(ctx *gin.Context)
	SetProfilePicture(ctx *gin.Context)
}

type userHandler struct {
	userService user_service.UserService
}

func (u *userHandler) GetUser(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.CheckUserStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(data.Phone) < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	uc, err := u.userService.GetUser(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responseData, err := json.Marshal(uc)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

func (u *userHandler) CheckUser(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.CheckUserStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(data.Phone) < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	uc, err := u.userService.CheckUser(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responseData, err := json.Marshal(uc)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

func (u *userHandler) GetAvatar(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.GetAvatarStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(data.Phone) < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	pic, err := u.userService.GetAvatar(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responseData, err := json.Marshal(pic)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

func (u *userHandler) GetContacts(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	contacts, err := u.userService.GetContacts(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responseData, err := json.Marshal(contacts)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

func (u *userHandler) GetPrivacy(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	privacy, err := u.userService.GetPrivacy(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responseData, err := json.Marshal(privacy)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

func (u *userHandler) BlockContact(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.BlockStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(data.Phone) < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	resp, err := u.userService.BlockContact(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responseData, err := json.Marshal(resp)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

func (u *userHandler) UnblockContact(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.BlockStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(data.Phone) < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	resp, err := u.userService.UnlockContact(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responseData, err := json.Marshal(resp)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

func (u *userHandler) GetBlockList(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	resp, err := u.userService.GetBlockList(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
}

func (u *userHandler) SetProfilePicture(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.SetProfilePictureStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Image == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "image is required"})
		return
	}

	resp, err := u.userService.SetProfilePicture(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !resp {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set profile picture"})
		return
	}

	responseData := gin.H{"image": data.Image}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

func NewUserHandler(
	userService user_service.UserService,
) UserHandler {
	return &userHandler{
		userService: userService,
	}
}
