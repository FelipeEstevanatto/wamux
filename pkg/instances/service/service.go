package instance_service

import (
	instances_model "github.com/Zapbox-API/evolution-go/pkg/instances/model"
	instance_repository "github.com/Zapbox-API/evolution-go/pkg/instances/repository"
)

type InstanceService interface {
	GetInstanceByToken(token string) (*instances_model.Instance, error)
}

type instance struct {
	instanceRepository instance_repository.InstanceRepository
}

func (i instance) GetInstanceByToken(token string) (*instances_model.Instance, error) {
	return i.instanceRepository.GetInstanceByToken(token)
}

func NewInstanceService(instanceRepository instance_repository.InstanceRepository) InstanceService {
	return &instance{instanceRepository: instanceRepository}
}
