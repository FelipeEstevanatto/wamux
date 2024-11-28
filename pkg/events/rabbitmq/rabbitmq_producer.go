package rabbitmq_producer

import (
	"strings"

	producer_interfaces "github.com/EvolutionAPI/evolution-go/pkg/events/interfaces"
	"github.com/gomessguii/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

type rabbitMQProducer struct {
	conn              *amqp.Connection
	amqpGlobalEnabled bool
}

func NewRabbitMQProducer(
	conn *amqp.Connection,
	amqpGlobalEnabled bool,
) producer_interfaces.Producer {
	return &rabbitMQProducer{
		conn:              conn,
		amqpGlobalEnabled: amqpGlobalEnabled,
	}
}

func (p *rabbitMQProducer) Produce(
	queueName string,
	payload []byte,
	rabbitmqEnable string,
	userID string,
) error {
	if queueName == "" {
		return nil
	}

	queueName = strings.ToLower(queueName)

	channel, err := p.conn.Channel()
	if err != nil {
		return err
	}
	defer func(channel *amqp.Channel) {
		err := channel.Close()
		if err != nil {
			logger.LogError("[%s] failed to close amqp channel", userID, err)
		}
	}(channel)

	args := amqp.Table{
		"x-queue-type": "quorum",
	}

	if p.amqpGlobalEnabled {
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

	if rabbitmqEnable == "enabled" {
		instanceQueueName := "instance." + queueName
		_, err = channel.QueueDeclare(
			instanceQueueName, // name
			true,              // durable
			false,             // delete when unused
			false,             // exclusive
			false,             // no-wait
			args,              // arguments (x-queue-type: quorum)
		)
		if err != nil {
			return err
		}

		err = channel.Publish(
			"",                // exchange
			instanceQueueName, // routing key
			false,             // mandatory
			false,             // immediate
			amqp.Publishing{
				ContentType: "application/json",
				Body:        payload,
			})

		if err != nil {
			return err
		}

		logger.LogInfo("[%s] Message enqueued successfully to queue %s", userID, instanceQueueName)
	}
	return nil
}
