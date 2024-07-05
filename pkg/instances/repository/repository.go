package instance_repository

import (
	instance_model "github.com/Zapbox-API/evolution-go/pkg/instances/model"
	"gorm.io/gorm"
)

type InstanceRepository interface {
	Create(instance_model.Instance) error
	GetInstanceByID(instanceId string) (*instance_model.Instance, error)
	GetInstanceByToken(token string) (*instance_model.Instance, error)
	GetInstanceByName(name string) (*instance_model.Instance, error)
	Update(*instance_model.Instance) error
	UpdateConnected(userId int, status bool) error
	UpdateJid(userId int, jid string) error
	GetAllConnectedInstances() ([]*instance_model.Instance, error)
	GetAll() ([]*instance_model.Instance, error)
	Delete(instanceId string) error
	DeleteByName(name string) error
}

type instanceRepository struct {
	db *gorm.DB
}

func (i *instanceRepository) Create(instance instance_model.Instance) error {
	return i.db.Create(&instance).Error
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
	var instance instance_model.Instance
	err := i.db.Where("id = ?", instanceId).First(&instance).Error
	if err != nil {
		return nil, err
	}

	return &instance, nil
}

func (i *instanceRepository) GetInstanceByName(name string) (*instance_model.Instance, error) {
	var instance instance_model.Instance
	err := i.db.Where("name = ?", name).First(&instance).Error
	if err != nil {
		return nil, err
	}

	return &instance, nil
}

func (i *instanceRepository) Update(instance *instance_model.Instance) error {
	return i.db.Updates(&instance).Error
}

func (i *instanceRepository) UpdateConnected(userId int, status bool) error {
	return i.db.Model(&instance_model.Instance{}).Where("id = ?", userId).Update("connected", status).Error
}

func (i *instanceRepository) UpdateJid(userId int, jid string) error {
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

func (i *instanceRepository) GetAll() ([]*instance_model.Instance, error) {
	var instances []*instance_model.Instance
	err := i.db.Find(&instances).Error
	if err != nil {
		return nil, err
	}

	return instances, nil
}

func (i *instanceRepository) Delete(instanceId string) error {
	return i.db.Delete(&instance_model.Instance{}, instanceId).Error
}

func (i *instanceRepository) DeleteByName(name string) error {
	return i.db.Where("name = ?", name).Delete(&instance_model.Instance{}).Error
}

func NewInstanceRepository(db *gorm.DB) InstanceRepository {
	return &instanceRepository{db: db}
}
