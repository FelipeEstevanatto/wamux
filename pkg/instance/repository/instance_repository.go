package instance_repository

import (
	"fmt"

	instance_model "github.com/EvolutionAPI/evolution-go/pkg/instance/model"
	"github.com/gomessguii/logger"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type InstanceRepository interface {
	Create(instance instance_model.Instance) (*instance_model.Instance, error)
	GetInstanceByID(instanceId string) (*instance_model.Instance, error)
	GetInstanceByToken(token string) (*instance_model.Instance, error)
	Update(*instance_model.Instance) error
	UpdateConnected(userId string, status bool) error
	UpdateJid(userId string, jid string) error
	GetAllConnectedInstances() ([]*instance_model.Instance, error)
	GetAllConnectedInstancesByClientName(clientName string) ([]*instance_model.Instance, error)
	GetAll() ([]*instance_model.Instance, error)
	Delete(instanceId string) error
}

type instanceRepository struct {
	db *gorm.DB
}

func (i *instanceRepository) Create(instance instance_model.Instance) (*instance_model.Instance, error) {
	if err := i.db.Create(&instance).Error; err != nil {
		return nil, err
	}
	return &instance, nil
}

func (i *instanceRepository) GetInstanceByToken(token string) (*instance_model.Instance, error) {
	var instance instance_model.Instance
	err := i.db.Where("token = ?", token).First(&instance).Error
	if err != nil {
		return nil, err
	}

	return &instance, nil
}

func (i *instanceRepository) GetInstanceByID(instanceId string) (*instance_model.Instance, error) {
	// Valida o formato do UUID
	if _, err := uuid.Parse(instanceId); err != nil {
		return nil, fmt.Errorf("invalid UUID format: %v", err)
	}

	var instance instance_model.Instance
	err := i.db.Where("id = ?", instanceId).First(&instance).Error
	if err != nil {
		return nil, err
	}

	return &instance, nil
}

func (i *instanceRepository) Update(instance *instance_model.Instance) error {
	err := i.db.Save(&instance).Error
	if err != nil {
		logger.LogError("Error updating instance in DB: %v", err)
	}
	return err
}

func (i *instanceRepository) UpdateConnected(userId string, status bool) error {
	return i.db.Model(&instance_model.Instance{}).Where("id = ?", userId).Update("connected", status).Error
}

func (i *instanceRepository) UpdateJid(userId string, jid string) error {
	return i.db.Model(&instance_model.Instance{}).Where("id = ?", userId).Update("jid", jid).Error
}

func (i *instanceRepository) GetAllConnectedInstances() ([]*instance_model.Instance, error) {
	var instances []*instance_model.Instance
	err := i.db.Where("connected = ?", true).Find(&instances).Error
	if err != nil {
		return nil, err
	}

	return instances, nil
}

func (i *instanceRepository) GetAllConnectedInstancesByClientName(clientName string) ([]*instance_model.Instance, error) {
	var instances []*instance_model.Instance
	err := i.db.Where("connected = ? AND client_name = ?", true, clientName).Find(&instances).Error
	if err != nil {
		return nil, err
	}

	return instances, nil
}

func (i *instanceRepository) GetAll() ([]*instance_model.Instance, error) {
	var instances []*instance_model.Instance
	err := i.db.Find(&instances).Error
	if err != nil {
		return nil, err
	}

	return instances, nil
}

func (i *instanceRepository) Delete(instanceId string) error {
	return i.db.Where("id = ?", instanceId).Delete(&instance_model.Instance{}).Error
}

func NewInstanceRepository(db *gorm.DB) InstanceRepository {
	return &instanceRepository{db: db}
}
