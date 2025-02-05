package rabbitmq_producer

import (
	"fmt"
	"strings"

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

	// Declara as filas globais se estiver habilitado
	if amqpGlobalEnabled {
		producer.declareGlobalQueues()
	}

	return producer
}

func (p *rabbitMQProducer) declareGlobalQueues() {
	if p.conn == nil {
		return
	}

	channel, err := p.conn.Channel()
	if err != nil {
		logger.LogError("Failed to open channel for declaring global queues: %v", err)
		return
	}
	defer channel.Close()

	args := amqp.Table{
		"x-queue-type": "quorum",
	}

	// Declara uma fila global para cada tipo de evento configurado
	for _, eventType := range p.amqpGlobalEvents {
		queueName := strings.ToLower(fmt.Sprintf("%s", eventType))

		_, err = channel.QueueDeclare(
			queueName, // name
			true,      // durable
			false,     // delete when unused
			false,     // exclusive
			false,     // no-wait
			args,      // arguments
		)
		if err != nil {
			logger.LogError("Failed to declare global queue %s: %v", queueName, err)
			continue
		}

		logger.LogInfo("Global queue %s declared successfully", queueName)
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
