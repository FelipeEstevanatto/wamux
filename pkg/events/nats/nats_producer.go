package nats_producer

import (
	"fmt"

	producer_interfaces "github.com/EvolutionAPI/evolution-go/pkg/events/interfaces"
	"github.com/gomessguii/logger"
	"github.com/nats-io/nats.go"
)

type natsProducer struct {
	conn              *nats.Conn
	natsGlobalEnabled bool
	natsGlobalEvents  []string
}

func NewNatsProducer(
	url string,
	natsGlobalEnabled bool,
	natsGlobalEvents []string,
) producer_interfaces.Producer {
	conn, err := nats.Connect(url)
	if err != nil {
		logger.LogError("Failed to connect to NATS: %v", err)
		return &natsProducer{
			conn:              nil,
			natsGlobalEnabled: false,
			natsGlobalEvents:  nil,
		}
	}

	return &natsProducer{
		conn:              conn,
		natsGlobalEnabled: natsGlobalEnabled,
		natsGlobalEvents:  natsGlobalEvents,
	}
}

func (p *natsProducer) Produce(
	queueName string,
	payload []byte,
	natsEnable string,
	userID string,
) error {
	logger.LogInfo("[%s] NATS Producer - Starting produce for subject: %s", userID, queueName)
	logger.LogInfo("[%s] NATS Producer - Global enabled: %v", userID, p.natsGlobalEnabled)

	if p.conn == nil {
		logger.LogWarn("[%s] NATS connection is nil", userID)
		return nil
	}

	if p.natsGlobalEnabled {
		logger.LogInfo("[%s] Publishing to global subject: %s", userID, queueName)
		err := p.conn.Publish(queueName, payload)
		if err != nil {
			logger.LogError("[%s] Failed to publish message to subject %s: %v", userID, queueName, err)
			return err
		}
		logger.LogInfo("[%s] Message published successfully to subject: %s", userID, queueName)
	}

	if natsEnable == "enabled" {
		instanceSubject := fmt.Sprintf("instance.%s", queueName)
		err := p.conn.Publish(instanceSubject, payload)
		if err != nil {
			logger.LogError("[%s] Failed to publish message to instance subject %s: %v", userID, instanceSubject, err)
			return err
		}
		logger.LogInfo("[%s] Message published successfully to instance subject: %s", userID, instanceSubject)
	}

	return nil
}
