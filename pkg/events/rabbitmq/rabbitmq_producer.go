package rabbitmq_producer

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	applog "github.com/felipeestevanatto/wamux/pkg/applog"
	producer_interfaces "github.com/felipeestevanatto/wamux/pkg/events/interfaces"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

// publishChannels caps the channels kept open for reuse. Opening one per event cost
// two extra round trips each time; more than this are closed after use.
const publishChannels = 8

// publishTimeout bounds waiting for the broker to confirm one message.
const publishTimeout = 10 * time.Second

type rabbitMQProducer struct {
	// mu guards conn: events publish from many goroutines, and they would otherwise
	// race to reconnect and overwrite each other's connection.
	mu                 sync.Mutex
	conn               *amqp.Connection
	idle               chan *amqp.Channel
	amqpGlobalEnabled  bool
	amqpGlobalEvents   []string
	amqpSpecificEvents []string
	connStr            string
	maxRetries         int
	loggerWrapper      *logger_wrapper.LoggerManager
}

func NewRabbitMQProducer(
	conn *amqp.Connection,
	amqpGlobalEnabled bool,
	amqpGlobalEvents []string,
	amqpSpecificEvents []string,
	connStr string,
	loggerWrapper *logger_wrapper.LoggerManager,
) producer_interfaces.Producer {
	producer := &rabbitMQProducer{
		conn:               conn,
		amqpGlobalEnabled:  amqpGlobalEnabled,
		amqpGlobalEvents:   amqpGlobalEvents,
		amqpSpecificEvents: amqpSpecificEvents,
		connStr:            connStr,
		maxRetries:         3,
		loggerWrapper:      loggerWrapper,
		idle:               make(chan *amqp.Channel, publishChannels),
	}

	return producer
}

// maskConnectionString masks sensitive information in the connection string for logging
func (p *rabbitMQProducer) maskConnectionString(connStr string) string {
	if connStr == "" {
		return "empty"
	}

	parsedURL, err := url.Parse(connStr)
	if err != nil {
		return "invalid-url"
	}

	// Mask password if present
	if parsedURL.User != nil {
		if _, hasPassword := parsedURL.User.Password(); hasPassword {
			parsedURL.User = url.UserPassword(parsedURL.User.Username(), "***")
		}
	}

	return parsedURL.String()
}

// handleConnectionClose monitors connection close events and logs them
func (p *rabbitMQProducer) handleConnectionClose(conn *amqp.Connection) {
	closeChan := make(chan *amqp.Error, 1)
	conn.NotifyClose(closeChan)

	closeErr := <-closeChan
	if closeErr != nil {
		applog.Logger.LogWarn("RabbitMQ connection closed unexpectedly: %v", closeErr)
		applog.Logger.LogInfo("Connection will be re-established on next message send")
	} else {
		applog.Logger.LogInfo("RabbitMQ connection closed gracefully")
	}
}

func (p *rabbitMQProducer) reconnect() error {
	if p.connStr == "" {
		return fmt.Errorf("connection string is empty - RabbitMQ URL not configured")
	}

	applog.Logger.LogInfo("Starting RabbitMQ reconnection process with URL: %s", p.maskConnectionString(p.connStr))

	var err error
	for i := 0; i < 3; i++ {
		applog.Logger.LogInfo("Tentando reconectar ao RabbitMQ (tentativa %d/3)", i+1)

		// Create connection with heartbeat to prevent timeouts
		config := amqp.Config{
			Heartbeat: 30 * time.Second, // Send heartbeat every 30 seconds
			Locale:    "en_US",
		}

		p.conn, err = amqp.DialConfig(p.connStr, config)
		if err == nil {
			applog.Logger.LogInfo("Reconectado com sucesso ao RabbitMQ com heartbeat de 30s")

			// Set up connection close notification
			go p.handleConnectionClose(p.conn)
			return nil
		}

		applog.Logger.LogError("Falha na tentativa %d/3 de reconexão: %v", i+1, err)
		if i < 2 { // Don't sleep on the last attempt
			time.Sleep(time.Second * 2)
		}
	}
	return fmt.Errorf("falha ao reconectar após 3 tentativas: %v", err)
}

// connection returns a live connection, reconnecting if needed. Callers wait on the
// lock during a reconnect rather than each starting their own.
func (p *rabbitMQProducer) connection() (*amqp.Connection, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil || p.conn.IsClosed() {
		if err := p.reconnect(); err != nil {
			return nil, err
		}
	}
	return p.conn, nil
}

// acquire returns a channel in confirm mode, reusing an idle one when it can. A
// channel closed by an error or a lost connection is skipped.
func (p *rabbitMQProducer) acquire() (*amqp.Channel, error) {
	for {
		select {
		case channel := <-p.idle:
			if !channel.IsClosed() {
				return channel, nil
			}
			continue
		default:
		}
		break
	}

	conn, err := p.connection()
	if err != nil {
		return nil, fmt.Errorf("falha ao garantir conexão: %v", err)
	}
	channel, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir canal: %v", err)
	}
	if err := channel.Confirm(false); err != nil {
		channel.Close()
		return nil, fmt.Errorf("falha ao configurar confirms do canal: %v", err)
	}
	return channel, nil
}

func (p *rabbitMQProducer) release(channel *amqp.Channel) {
	select {
	case p.idle <- channel:
	default:
		channel.Close()
	}
}

// publishOnce declares the queue and publishes, returning only once the broker has
// confirmed the message. The declare stays on every publish: to the default exchange
// a queue deleted on the broker would swallow messages with a confirmed ack.
func (p *rabbitMQProducer) publishOnce(queueName string, payload []byte) error {
	channel, err := p.acquire()
	if err != nil {
		return err
	}

	args := amqp.Table{
		"x-queue-type": "quorum",
		"x-ha-policy":  "all", // Alta disponibilidade
	}
	if _, err := channel.QueueDeclare(queueName, true, false, false, false, args); err != nil {
		channel.Close()
		return fmt.Errorf("falha ao declarar fila %s: %v", queueName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()

	confirmation, err := channel.PublishWithDeferredConfirmWithContext(ctx, "", queueName, false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         payload,
		DeliveryMode: amqp.Persistent, // Garante persistência da mensagem
	})
	if err != nil {
		channel.Close()
		return fmt.Errorf("falha ao publicar: %v", err)
	}

	acked, err := confirmation.WaitContext(ctx)
	if err != nil {
		// The confirm may still arrive; this channel can no longer be trusted.
		channel.Close()
		return fmt.Errorf("sem confirmação do broker: %v", err)
	}
	p.release(channel)
	if !acked {
		return fmt.Errorf("broker recusou a mensagem")
	}
	return nil
}

func (p *rabbitMQProducer) Produce(
	queueName string,
	payload []byte,
	rabbitmqEnable string,
	userID string,
) error {
	if rabbitmqEnable != "global" && rabbitmqEnable != "enabled" && rabbitmqEnable != "true" {
		return nil
	}

	p.loggerWrapper.GetLogger(userID).LogInfo("[%s] RabbitMQ Producer - Starting produce for queue: %s", userID, queueName)

	if p.connStr == "" {
		return fmt.Errorf("RabbitMQ connection string is empty - check AMQP_URL configuration")
	}

	var err error
	for i := 0; i < p.maxRetries; i++ {
		if err = p.publishOnce(queueName, payload); err == nil {
			p.loggerWrapper.GetLogger(userID).LogInfo("[%s] Mensagem publicada com sucesso na fila: %s", userID, queueName)
			return nil
		}

		applog.Logger.LogWarn("[%s] Falha ao publicar mensagem (tentativa %d/%d): %v", userID, i+1, p.maxRetries, err)
		if i+1 < p.maxRetries {
			time.Sleep(time.Second * time.Duration(i+1))
		}
	}
	return fmt.Errorf("falha ao publicar mensagem após todas as tentativas: %v", err)
}

// CreateGlobalQueues cria todas as filas globais no startup da aplicação
func (p *rabbitMQProducer) CreateGlobalQueues() error {
	if !p.amqpGlobalEnabled {
		return nil
	}

	p.loggerWrapper.GetLogger("system").LogInfo("Creating global queues for enabled events")

	conn, err := p.connection()
	if err != nil {
		return fmt.Errorf("failed to ensure connection: %v", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open channel: %v", err)
	}
	defer channel.Close()

	args := amqp.Table{
		"x-queue-type": "quorum",
		"x-ha-policy":  "all", // Alta disponibilidade
	}

	createdQueues := 0

	// AMQP_SPECIFIC_EVENTS tem prioridade sobre AMQP_GLOBAL_EVENTS
	if len(p.amqpSpecificEvents) > 0 {
		p.loggerWrapper.GetLogger("system").LogInfo("Using AMQP_SPECIFIC_EVENTS (priority over AMQP_GLOBAL_EVENTS)")

		// Cria filas diretas para eventos específicos
		for _, eventName := range p.amqpSpecificEvents {
			queueName := strings.ToLower(eventName)

			_, err = channel.QueueDeclare(
				queueName, // name
				true,      // durable
				false,     // delete when unused
				false,     // exclusive
				false,     // no-wait
				args,      // arguments
			)
			if err != nil {
				p.loggerWrapper.GetLogger("system").LogError("Failed to create specific queue %s: %v", queueName, err)
				return fmt.Errorf("failed to create specific queue %s: %v", queueName, err)
			}
			p.loggerWrapper.GetLogger("system").LogInfo("Specific queue created: %s", queueName)
			createdQueues++
		}
	} else {
		p.loggerWrapper.GetLogger("system").LogInfo("Using AMQP_GLOBAL_EVENTS (fallback mode)")

		// Mapeia eventos globais para os eventos originais que precisam de filas (modo antigo)
		eventMap := map[string][]string{
			"MESSAGE":       {"message"},
			"SEND_MESSAGE":  {"sendmessage"},
			"READ_RECEIPT":  {"receipt"},
			"PRESENCE":      {"presence"},
			"HISTORY_SYNC":  {"historysync"},
			"CHAT_PRESENCE": {"chatpresence", "archive"},
			"CALL":          {"calloffer", "callaccept", "callterminate", "calloffernotice", "callrelaylatency"},
			"CONNECTION":    {"connected", "pairsuccess", "temporaryban", "loggedout", "connectfailure", "disconnected"},
			"LABEL":         {"labeledit", "labelassociationchat", "labelassociationmessage"},
			"CONTACT":       {"contact", "pushname"},
			"GROUP":         {"groupinfo", "joinedgroup"},
			"NEWSLETTER":    {"newsletterjoin", "newsletterleave"},
			"QRCODE":        {"qrcode", "qrtimeout", "qrsuccess"},
		}

		for _, globalEvent := range p.amqpGlobalEvents {
			if queueNames, exists := eventMap[globalEvent]; exists {
				for _, queueName := range queueNames {
					_, err = channel.QueueDeclare(
						queueName, // name
						true,      // durable
						false,     // delete when unused
						false,     // exclusive
						false,     // no-wait
						args,      // arguments
					)
					if err != nil {
						p.loggerWrapper.GetLogger("system").LogError("Failed to create global queue %s: %v", queueName, err)
						return fmt.Errorf("failed to create global queue %s: %v", queueName, err)
					}
					p.loggerWrapper.GetLogger("system").LogInfo("Global queue created: %s", queueName)
					createdQueues++
				}
			}
		}
	}

	p.loggerWrapper.GetLogger("system").LogInfo("Successfully created %d global queues", createdQueues)
	return nil
}
