package producer_interfaces

type Producer interface {
	Produce(queueName string, payload []byte, webhookUrl string) error
}
