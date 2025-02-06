package rabbitmq_producer

import (
	producer_interfaces "github.com/EvolutionAPI/evolution-go/pkg/events/interfaces"
	"github.com/gomessguii/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

type rabbitMQProducer struct {
	conn              *amqp.Connection
	amqpGlobalEnabled bool
	amqpGlobalEvents  []string
}

func NewRabbitMQProducer(
	conn *amqp.Connection,
	amqpGlobalEnabled bool,
	amqpGlobalEvents []string,
) producer_interfaces.Producer {
	producer := &rabbitMQProducer{
		conn:              conn,
		amqpGlobalEnabled: amqpGlobalEnabled,
		amqpGlobalEvents:  amqpGlobalEvents,
	}

	return producer
}

func (p *rabbitMQProducer) Produce(
	queueName string,
	payload []byte,
	rabbitmqEnable string,
	userID string,
) error {
	logger.LogInfo("[%s] RabbitMQ Producer - Starting produce for queue: %s", userID, queueName)
	logger.LogInfo("[%s] RabbitMQ Producer - Global enabled: %v", userID, p.amqpGlobalEnabled)

	if p.conn == nil {
		logger.LogWarn("[%s] RabbitMQ connection is nil", userID)
		return nil
	}

	channel, err := p.conn.Channel()
	if err != nil {
		logger.LogError("[%s] Failed to open channel: %v", userID, err)
		return err
	}
	defer channel.Close()

	args := amqp.Table{
		"x-queue-type": "quorum",
	}

	if rabbitmqEnable == "global" {
		logger.LogInfo("[%s] Declaring global queue: %s", userID, queueName)

		_, err = channel.QueueDeclare(
			queueName, // name
			true,      // durable
			false,     // delete when unused
			false,     // exclusive
			false,     // no-wait
			args,      // arguments
		)
		if err != nil {
			logger.LogError("[%s] Failed to declare queue %s: %v", userID, queueName, err)
			return err
		}

		logger.LogInfo("[%s] Publishing message to queue: %s", userID, queueName)

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
			logger.LogError("[%s] Failed to publish message to queue %s: %v", userID, queueName, err)
			return err
		}

		logger.LogInfo("[%s] Message published successfully to queue: %s", userID, queueName)
	}

	if rabbitmqEnable == "enabled" {
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

		logger.LogInfo("[%s] Message enqueued successfully to queue %s", userID, queueName)
	}
	return nil
}
