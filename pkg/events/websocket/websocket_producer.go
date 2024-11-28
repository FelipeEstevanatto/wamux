package websocket_producer

import (
	"strings"
	"sync"

	producer_interfaces "github.com/EvolutionAPI/evolution-go/pkg/events/interfaces"
	"github.com/gomessguii/logger"
	"github.com/gorilla/websocket"
)

type websocketProducer struct {
	clients    map[string]*websocket.Conn
	clientsMux sync.RWMutex
}

func NewWebsocketProducer() producer_interfaces.Producer {
	return &websocketProducer{
		clients:    make(map[string]*websocket.Conn),
		clientsMux: sync.RWMutex{},
	}
}

func (p *websocketProducer) AddClient(instanceID string, conn *websocket.Conn) {
	p.clientsMux.Lock()
	defer p.clientsMux.Unlock()
	p.clients[instanceID] = conn
}

func (p *websocketProducer) RemoveClient(instanceID string) {
	p.clientsMux.Lock()
	defer p.clientsMux.Unlock()
	delete(p.clients, instanceID)
}

func (p *websocketProducer) Produce(queueName string, payload []byte, _ string, userID string) error {
	instanceID := strings.Split(queueName, ".")[0]

	p.clientsMux.RLock()
	client, exists := p.clients[instanceID]
	p.clientsMux.RUnlock()

	if !exists {
		return nil
	}

	err := client.WriteMessage(websocket.TextMessage, payload)
	if err != nil {
		logger.LogError("[%s] failed to send websocket message", userID, "error", err)
		return err
	}

	logger.LogInfo("[%s] Message sent to websocket successfully to queue %s", userID, queueName)

	return nil
}
