package websocket_service

type WebsocketService interface {
}

type websocketService struct {
}

func NewWebsocketService() WebsocketService {
	return &websocketService{}
}
