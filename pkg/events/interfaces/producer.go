package producer_interfaces

type Producer interface {
	Produce(queueName string, payload []byte, webhookUrl string, userID string) error
	CreateGlobalQueues() error
}

// SignedProducer is implemented by producers that can authenticate a delivered
// payload with an HMAC key. Only the HTTP webhook producer needs it; queue
// producers (AMQP, NATS) and the WebSocket producer keep the plain Producer
// contract. Callers type-assert so adding signing did not change every
// implementation.
type SignedProducer interface {
	Producer
	// ProduceSigned behaves like Produce but adds the "x-hmac-signature"
	// header holding the hex HMAC-SHA256 of the payload under hmacKey. An empty
	// key produces an unsigned delivery, identical to Produce.
	ProduceSigned(queueName string, payload []byte, webhookUrl string, userID string, hmacKey []byte) error
}

// DepthReporter is implemented by producers that can report their in-flight
// depth, for the /metrics endpoint.
type DepthReporter interface {
	QueueDepth() int
}
