package user_handler

// HTTP handler for saving a contact to the device addressbook.
// See the rationale in pkg/user/service/save_contact.go.

import (
	"net/http"

	docmodels "github.com/evolution-foundation/evolution-go/pkg/docmodels"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	user_service "github.com/evolution-foundation/evolution-go/pkg/user/service"
	"github.com/gin-gonic/gin"
)

// Keep docmodels referenced so the package import is not dropped
// (swag reads Go source, not pre-processed, and needs the alias to be in scope).
var _ = docmodels.Envelope{}

// Save a contact to the device addressbook
// @Summary Save a contact
// @Description Save/update a contact in the WhatsApp contact list (app state), asking the
// @Description primary device to also store it in the system addressbook.
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.SaveContactStruct true "Contact data"
// @Success 200 {object} docmodels.Envelope "success"
// @Failure 400 {object} docmodels.ErrorResponse "Error on validation"
// @Failure 500 {object} docmodels.ErrorResponse "Internal server error"
// @Router /user/savecontact [post]
func (u *userHandler) SaveContact(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.SaveContactStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" || data.FullName == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "number and fullName are required"})
		return
	}

	if err := u.userService.SaveContact(data, instance); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}
