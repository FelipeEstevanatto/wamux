package rabbitmq_producer

import (
	"fmt"
	"time"

	producer_interfaces "github.com/EvolutionAPI/evolution-go/pkg/events/interfaces"
	logger_wrapper "github.com/EvolutionAPI/evolution-go/pkg/logger"
	"github.com/gomessguii/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

type rabbitMQProducer struct {
	conn              *amqp.Connection
	amqpGlobalEnabled bool
	amqpGlobalEvents  []string
	connStr           string
	maxRetries        int
	loggerWrapper     *logger_wrapper.LoggerManager
}

func NewRabbitMQProducer(
	conn *amqp.Connection,
	amqpGlobalEnabled bool,
	amqpGlobalEvents []string,
	connStr string,
	loggerWrapper *logger_wrapper.LoggerManager,
) producer_interfaces.Producer {
	producer := &rabbitMQProducer{
		conn:              conn,
		amqpGlobalEnabled: amqpGlobalEnabled,
		amqpGlobalEvents:  amqpGlobalEvents,
		connStr:           connStr,
		maxRetries:        3,
	}

	return producer
}

func (p *rabbitMQProducer) reconnect() error {
	var err error
	for i := 0; i < 3; i++ {
		logger.LogInfo("Tentando reconectar ao RabbitMQ (tentativa %d/3)", i+1)
		p.conn, err = amqp.Dial(p.connStr)
		if err == nil {
			logger.LogInfo("Reconectado com sucesso ao RabbitMQ")
			return nil
		}
		time.Sleep(time.Second * 2)
	}
	return fmt.Errorf("falha ao reconectar após 3 tentativas: %v", err)
}

func (p *rabbitMQProducer) ensureConnection() error {
	if p.conn == nil || p.conn.IsClosed() {
		return p.reconnect()
	}
	return nil
}

func (p *rabbitMQProducer) publishWithRetry(
	channel *amqp.Channel,
	queueName string,
	payload []byte,
	userID string,
) error {
	var err error
	for i := 0; i < p.maxRetries; i++ {
		err = channel.Publish(
			"",        // exchange
			queueName, // routing key
			false,     // mandatory
			false,     // immediate
			amqp.Publishing{
				ContentType:  "application/json",
				Body:         payload,
				DeliveryMode: amqp.Persistent, // Garante persistência da mensagem
			})

		if err == nil {
			return nil
		}

		logger.LogWarn("[%s] Falha ao publicar mensagem (tentativa %d/%d): %v",
			userID, i+1, p.maxRetries, err)

		// Se o erro for de conexão, tenta reconectar
		if err.Error() == "Exception (504) Reason: \"channel/connection is not open\"" {
			if err := p.ensureConnection(); err != nil {
				continue
			}

			// Cria novo canal após reconexão
			channel, err = p.conn.Channel()
			if err != nil {
				continue
			}
		}

		time.Sleep(time.Second * time.Duration(i+1))
	}
	return err
}

func (p *rabbitMQProducer) Produce(
	queueName string,
	payload []byte,
	rabbitmqEnable string,
	userID string,
) error {
	p.loggerWrapper.GetLogger(userID).LogInfo("[%s] RabbitMQ Producer - Starting produce for queue: %s", userID, queueName)

	if err := p.ensureConnection(); err != nil {
		return fmt.Errorf("falha ao garantir conexão: %v", err)
	}

	channel, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("falha ao abrir canal: %v", err)
	}
	defer channel.Close()

	// Configura confirmação de publicação
	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("falha ao configurar confirms do canal: %v", err)
	}

	args := amqp.Table{
		"x-queue-type": "quorum",
		"x-ha-policy":  "all", // Alta disponibilidade
	}

	if rabbitmqEnable == "global" || rabbitmqEnable == "enabled" {
		_, err = channel.QueueDeclare(
			queueName, // name
			true,      // durable
			false,     // delete when unused
			false,     // exclusive
			false,     // no-wait
			args,      // arguments
		)
		if err != nil {
			return fmt.Errorf("falha ao declarar fila %s: %v", queueName, err)
		}

		err = p.publishWithRetry(channel, queueName, payload, userID)
		if err != nil {
			return fmt.Errorf("falha ao publicar mensagem após todas as tentativas: %v", err)
		}

		p.loggerWrapper.GetLogger(userID).LogInfo("[%s] Mensagem publicada com sucesso na fila: %s", userID, queueName)
	}

	return nil
}
