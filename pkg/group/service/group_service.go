package group_service

type GroupService interface {
}

type groupService struct {
}

func NewGroupService() GroupService {
	return &groupService{}
}
