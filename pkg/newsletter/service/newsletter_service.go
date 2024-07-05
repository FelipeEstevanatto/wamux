package newsletter_service

type NewsletterService interface {
}

type newsletterService struct {
}

func NewNewsletterService() NewsletterService {
	return &newsletterService{}
}
