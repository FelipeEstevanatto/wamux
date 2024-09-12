package instance_model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Instance struct {
	Id         string `json:"id" gorm:"type:uuid;primaryKey"`
	Name       string `json:"name"`
	Token      string `json:"token" gorm:"unique"`
	Webhook    string `json:"webhook"`
	Jid        string `json:"jid"`
	Qrcode     string `json:"qrcode" gorm:"type:text"`
	Connected  bool   `json:"connected"`
	Expiration int64  `json:"expiration"`
	Events     string `json:"events"`
	OsName     string `json:"os_name"`
	Proxy      string `json:"proxy"`
}

func (m *Instance) BeforeCreate(tx *gorm.DB) (err error) {
	if m.Id == "" {
		m.Id = uuid.New().String()
	}
	return
}
