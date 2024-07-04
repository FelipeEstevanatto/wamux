package message_model

type Message struct {
	Id        int    `json:"id" gorm:"primaryKey"`
	MessageID string `json:"message_id" gorm:"unique"`
	Timestamp string `json:"timestamp"`
	Status    string `json:"status"`
	Source    string `json:"source"`
}
