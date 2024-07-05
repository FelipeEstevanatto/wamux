package newsletter_handler

import "github.com/gin-gonic/gin"

type NewsletterHandler interface {
	CreateNewsletter(ctx *gin.Context)
	ListNewsletter(ctx *gin.Context)
	GetNewsletter(ctx *gin.Context)
	GetNewsletterInvite(ctx *gin.Context)
	SubscribeNewsletter(ctx *gin.Context)
	GetNewsletterMessages(ctx *gin.Context)
}

type newsletterHandler struct {
}

// CreateNewsletter implements NewsletterHandler.
func (n *newsletterHandler) CreateNewsletter(ctx *gin.Context) {
	panic("unimplemented")
}

// GetNewsletter implements NewsletterHandler.
func (n *newsletterHandler) GetNewsletter(ctx *gin.Context) {
	panic("unimplemented")
}

// GetNewsletterInvite implements NewsletterHandler.
func (n *newsletterHandler) GetNewsletterInvite(ctx *gin.Context) {
	panic("unimplemented")
}

// GetNewsletterMessages implements NewsletterHandler.
func (n *newsletterHandler) GetNewsletterMessages(ctx *gin.Context) {
	panic("unimplemented")
}

// ListNewsletter implements NewsletterHandler.
func (n *newsletterHandler) ListNewsletter(ctx *gin.Context) {
	panic("unimplemented")
}

// SubscribeNewsletter implements NewsletterHandler.
func (n *newsletterHandler) SubscribeNewsletter(ctx *gin.Context) {
	panic("unimplemented")
}

func NewNewsletterHandler() NewsletterHandler {
	return &newsletterHandler{}
}
