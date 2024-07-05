package user_handler

import "github.com/gin-gonic/gin"

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
}

// BlockContact implements UserHandler.
func (u *userHandler) BlockContact(ctx *gin.Context) {
	panic("unimplemented")
}

// CheckUser implements UserHandler.
func (u *userHandler) CheckUser(ctx *gin.Context) {
	panic("unimplemented")
}

// GetAvatar implements UserHandler.
func (u *userHandler) GetAvatar(ctx *gin.Context) {
	panic("unimplemented")
}

// GetBlockList implements UserHandler.
func (u *userHandler) GetBlockList(ctx *gin.Context) {
	panic("unimplemented")
}

// GetContacts implements UserHandler.
func (u *userHandler) GetContacts(ctx *gin.Context) {
	panic("unimplemented")
}

// GetPrivacy implements UserHandler.
func (u *userHandler) GetPrivacy(ctx *gin.Context) {
	panic("unimplemented")
}

// GetUser implements UserHandler.
func (u *userHandler) GetUser(ctx *gin.Context) {
	panic("unimplemented")
}

// SetProfilePicture implements UserHandler.
func (u *userHandler) SetProfilePicture(ctx *gin.Context) {
	panic("unimplemented")
}

// UnblockContact implements UserHandler.
func (u *userHandler) UnblockContact(ctx *gin.Context) {
	panic("unimplemented")
}

func NewUserHandler() UserHandler {
	return &userHandler{}
}
