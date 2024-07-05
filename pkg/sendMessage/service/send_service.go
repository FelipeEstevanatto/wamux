package send_service

type SendService interface {
}

type sendService struct {
}

func NewSendService() SendService {
	return &sendService{}
}
