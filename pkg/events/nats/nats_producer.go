package nats_producer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	applog "github.com/felipeestevanatto/wamux/pkg/applog"
	producer_interfaces "github.com/felipeestevanatto/wamux/pkg/events/interfaces"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	"github.com/nats-io/nats.go"
	"strings"
)

type natsProducer struct {
	conn              *nats.Conn
	natsGlobalEnabled bool
	natsGlobalEvents  []string
	loggerWrapper     *logger_wrapper.LoggerManager

	// JetStream is optional. When js is set, Produce waits for the stream ack
	// and passes a message id so a retry inside the duplicate window is a
	// no-op. A missing stream falls back to core NATS rather than dropping.
	js     nats.JetStreamContext
	stream string
}

// Option configures the NATS producer.
type Option func(*natsProducer)

// WithJetStream publishes through the named stream instead of core NATS.
func WithJetStream(stream string) Option {
	return func(p *natsProducer) { p.stream = strings.TrimSpace(stream) }
}

func NewNatsProducer(
	url string,
	natsGlobalEnabled bool,
	natsGlobalEvents []string,
	loggerWrapper *logger_wrapper.LoggerManager,
	opts ...Option,
) producer_interfaces.Producer {
	if strings.TrimSpace(url) == "" {
		producer := &natsProducer{loggerWrapper: loggerWrapper}
		for _, opt := range opts {
			opt(producer)
		}
		return producer
	}

	conn, err := nats.Connect(url)
	if err != nil {
		applog.Logger.LogError("Failed to connect to NATS: %v", err)
		producer := &natsProducer{loggerWrapper: loggerWrapper}
		for _, opt := range opts {
			opt(producer)
		}
		return producer
	}

	producer := &natsProducer{
		conn:              conn,
		natsGlobalEnabled: natsGlobalEnabled,
		natsGlobalEvents:  natsGlobalEvents,
		loggerWrapper:     loggerWrapper,
	}
	for _, opt := range opts {
		opt(producer)
	}
	if producer.stream != "" {
		js, err := conn.JetStream()
		if err != nil {
			applog.Logger.LogError("NATS JetStream is not available, falling back to core NATS: %v", err)
		} else if _, err := js.StreamInfo(producer.stream); err != nil {
			applog.Logger.LogError("NATS stream %q is not available, falling back to core NATS: %v", producer.stream, err)
		} else {
			producer.js = js
			applog.Logger.LogInfo("NATS JetStream enabled on stream %s", producer.stream)
		}
	}
	return producer
}

func (p *natsProducer) Produce(
	queueName string,
	payload []byte,
	natsEnable string,
	userID string,
) error {
	p.loggerWrapper.GetLogger(userID).LogInfo("[%s] NATS Producer - Starting produce for subject: %s", userID, queueName)
	p.loggerWrapper.GetLogger(userID).LogInfo("[%s] NATS Producer - Global enabled: %v", userID, p.natsGlobalEnabled)

	if p.conn == nil {
		p.loggerWrapper.GetLogger(userID).LogWarn("[%s] NATS connection is nil", userID)
		return nil
	}

	if natsEnable == "global" {
		p.loggerWrapper.GetLogger(userID).LogInfo("[%s] Publishing to global subject: %s", userID, queueName)
		if err := p.publish(queueName, payload); err != nil {
			p.loggerWrapper.GetLogger(userID).LogError("[%s] Failed to publish message to subject %s: %v", userID, queueName, err)
			return err
		}
		p.loggerWrapper.GetLogger(userID).LogInfo("[%s] Message published successfully to subject: %s", userID, queueName)
	}

	if natsEnable == "enabled" || natsEnable == "true" {
		if err := p.publish(queueName, payload); err != nil {
			p.loggerWrapper.GetLogger(userID).LogError("[%s] Failed to publish message to instance subject %s: %v", userID, queueName, err)
			return err
		}
		p.loggerWrapper.GetLogger(userID).LogInfo("[%s] Message published successfully to instance subject: %s", userID, queueName)
	}

	return nil
}

// publish uses JetStream when it was configured successfully, otherwise core
// NATS. The message id is a hash of the exact payload: a caller retrying the
// same event inside JetStream's duplicate window does not create a second copy.
func (p *natsProducer) publish(subject string, payload []byte) error {
	if p.js != nil {
		sum := sha256.Sum256(payload)
		_, err := p.js.Publish(subject, payload, nats.MsgId(hex.EncodeToString(sum[:])))
		if err == nil {
			return nil
		}
		// A stream that does not cover this subject is a configuration error,
		// not a reason to lose the event: fall back to core NATS and keep going.
		p.loggerWrapper.GetLogger("").LogWarn("NATS JetStream publish failed for %s, using core NATS: %v", subject, err)
	}
	return p.conn.Publish(subject, payload)
}

// Drain flushes publishes still sitting in the client buffer and closes the
// connection. Without it a SIGTERM can drop events that Publish accepted but
// the server had not received yet.
func (p *natsProducer) Drain(ctx context.Context) error {
	if p.conn == nil {
		return nil
	}
	err := p.conn.FlushWithContext(ctx)
	p.conn.Close()
	return err
}

// CreateGlobalQueues não faz nada para NATS producer pois os subjects são criados dinamicamente
func (p *natsProducer) CreateGlobalQueues() error {
	return nil
}
