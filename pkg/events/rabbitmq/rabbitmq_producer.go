package rabbitmq_producer

import (
	producer_interfaces "github.com/Zapbox-API/evolution-go/pkg/events/interfaces"
	"github.com/gomessguii/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

type rabbitMQProducer struct {
	conn *amqp.Connection
}

func NewRabbitMQProducer(
	conn *amqp.Connection,
) producer_interfaces.Producer {
	return &rabbitMQProducer{
		conn: conn,
	}
}

func (p *rabbitMQProducer) Produce(
	queueName string,
	payload []byte,
) error {
	channel, err := p.conn.Channel()
	if err != nil {
		return err
	}
	defer func(channel *amqp.Channel) {
		err := channel.Close()
		if err != nil {
			logger.LogError("failed to close amqp channel", err)
		}
	}(channel)

	args := amqp.Table{
		"x-queue-type": "quorum",
	}

	_, err = channel.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		args,      // arguments (x-queue-type: quorum)
	)
	if err != nil {
		return err
	}

	err = channel.Publish(
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        payload,
		})
	if err != nil {
		return err
	}
	return nil
}
