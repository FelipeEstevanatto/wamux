package chat_service

type ChatService interface {
}

type chatService struct {
}

func NewChatService() ChatService {
	return &chatService{}
}
