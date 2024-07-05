package label_service

type LabelService interface {
}

type labelService struct {
}

func NewLabelService() LabelService {
	return &labelService{}
}
