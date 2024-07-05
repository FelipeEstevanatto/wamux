package community_service

type ChatService interface {
}

type communityService struct {
}

func NewChatService() ChatService {
	return &communityService{}
}
