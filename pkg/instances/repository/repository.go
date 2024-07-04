package instance_repository

import (
	instance_model "github.com/Zapbox-API/evolution-go/pkg/instances/model"
	"gorm.io/gorm"
)

type InstanceRepository interface {
	Create(instance_model.Instance) error
	GetInstanceByToken(token string) (*instance_model.Instance, error)
	Update(*instance_model.Instance) error
	UpdateConnected(userId int, status bool) error
	UpdateJid(userId int, jid string) error
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

func (i *instanceRepository) Update(instance *instance_model.Instance) error {
	return i.db.Updates(&instance).Error
}

func (i *instanceRepository) UpdateConnected(userId int, status bool) error {
	return i.db.Model(&instance_model.Instance{}).Where("id = ?", userId).Update("connected", status).Error
}

func (i *instanceRepository) UpdateJid(userId int, jid string) error {
	return i.db.Model(&instance_model.Instance{}).Where("id = ?", userId).Update("jid", jid).Error
}

func NewInstanceRepository(db *gorm.DB) InstanceRepository {
	return &instanceRepository{db: db}
}
