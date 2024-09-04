package producer_interfaces

type Producer interface {
	Produce(queueName string, payload []byte) error
}
