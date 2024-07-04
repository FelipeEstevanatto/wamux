package instance_model

type Instance struct {
	Id         int    `json:"id" gorm:"primaryKey"`
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
