package websocket_producer

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	applog "github.com/felipeestevanatto/wamux/pkg/applog"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

const (
	// writeWait bounds one frame. Without it a subscriber whose TCP window is
	// closed holds the writer forever.
	writeWait = 10 * time.Second

	// pongWait is how long a silent connection is kept. pingPeriod must be shorter
	// so a healthy client always answers in time. This is what finds half-open
	// sockets, which never produce a read error on their own.
	pongWait   = 60 * time.Second
	pingPeriod = 50 * time.Second

	// sendQueueSize and maxQueuedBytes bound what one subscriber can hold. Bytes
	// matter as well as count: an event can carry base64 media.
	sendQueueSize  = 256
	maxQueuedBytes = 32 << 20

	// maxInboundBytes caps a frame from a subscriber. Inbound frames are ignored,
	// so there is no reason to buffer a large one.
	maxInboundBytes = 64 << 10
)

// client is one subscriber with its own bounded queue and writer goroutine.
//
// Produce runs in the goroutine that goes on to deliver the same event to the
// webhook, so it must never wait on a socket. It only enqueues; writePump is the
// sole writer, which also satisfies gorilla's one-concurrent-writer rule. A
// subscriber that cannot keep up is disconnected rather than waited for, and a
// browser reconnects.
type client struct {
	conn      *websocket.Conn
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
	queued    atomic.Int64
}

func newClient(conn *websocket.Conn) *client {
	return &client{
		conn: conn,
		send: make(chan []byte, sendQueueSize),
		done: make(chan struct{}),
	}
}

// enqueue hands msg to the writer without blocking. It reports false when the
// subscriber is closed or has fallen too far behind.
func (c *client) enqueue(msg []byte) bool {
	size := int64(len(msg))
	if c.queued.Add(size) > maxQueuedBytes {
		c.queued.Add(-size)
		return false
	}
	select {
	case c.send <- msg:
		return true
	case <-c.done:
	default:
	}
	c.queued.Add(-size)
	return false
}

// close stops the writer and closes the socket, which also ends the read loop.
// It is safe to call from any goroutine, any number of times.
func (c *client) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

// writePump is the only goroutine that writes to the connection.
func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer c.close()

	for {
		select {
		case msg := <-c.send:
			c.queued.Add(-int64(len(msg)))
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}

// readPump exists to notice a closed socket and to receive pongs; inbound frames
// are ignored.
func (c *client) readPump() {
	c.conn.SetReadLimit(maxInboundBytes)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

type websocketProducer struct {
	// clients holds every connection subscribed to a given instance. This is a
	// slice, not a single connection: previously a second subscriber replaced the
	// first, so opening the UI in two tabs silently cut delivery to the older one,
	// and either tab disconnecting removed the whole instance entry.
	clients       map[string][]*client
	broadcast     []*client
	clientsMux    sync.RWMutex
	loggerWrapper *logger_wrapper.LoggerManager
}

func NewWebsocketProducer(loggerWrapper *logger_wrapper.LoggerManager) *websocketProducer {
	return &websocketProducer{
		clients:       make(map[string][]*client),
		broadcast:     make([]*client, 0),
		loggerWrapper: loggerWrapper,
	}
}

// ServeWs upgrades an HTTP request and registers it for the lifetime of the
// connection. An empty instanceId subscribes to every instance's events.
func ServeWs(w http.ResponseWriter, r *http.Request, instanceId string, producer *websocketProducer) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		applog.Logger.LogError("Erro ao fazer upgrade da conexão websocket: %v", err)
		return
	}

	c := newClient(conn)
	if instanceId == "" {
		producer.addBroadcastClient(c)
	} else {
		producer.addClient(instanceId, c)
	}

	go c.writePump()

	// Deregistration is keyed on this exact connection so sibling subscribers of
	// the same instance keep receiving events.
	go func() {
		defer func() {
			if instanceId == "" {
				producer.removeBroadcastClient(c)
			} else {
				producer.removeClient(instanceId, c)
			}
			c.close()
		}()
		c.readPump()
	}()
}

func (p *websocketProducer) addBroadcastClient(c *client) {
	p.clientsMux.Lock()
	p.broadcast = append(p.broadcast, c)
	n := len(p.broadcast)
	p.clientsMux.Unlock()
	applog.Logger.LogInfo("Cliente broadcast websocket adicionado (total: %d)", n)
}

func (p *websocketProducer) removeBroadcastClient(c *client) {
	p.clientsMux.Lock()
	p.broadcast = drop(p.broadcast, c)
	n := len(p.broadcast)
	p.clientsMux.Unlock()
	applog.Logger.LogInfo("Cliente broadcast websocket removido (total: %d)", n)
}

func (p *websocketProducer) addClient(instanceID string, c *client) {
	p.clientsMux.Lock()
	p.clients[instanceID] = append(p.clients[instanceID], c)
	n := len(p.clients[instanceID])
	p.clientsMux.Unlock()
	p.loggerWrapper.GetLogger(instanceID).LogInfo(
		"Cliente websocket adicionado para instância: %s (total: %d)", instanceID, n)
}

func (p *websocketProducer) removeClient(instanceID string, c *client) {
	p.clientsMux.Lock()
	remaining := drop(p.clients[instanceID], c)
	if len(remaining) == 0 {
		delete(p.clients, instanceID)
	} else {
		p.clients[instanceID] = remaining
	}
	p.clientsMux.Unlock()
	p.loggerWrapper.GetLogger(instanceID).LogInfo(
		"Cliente websocket removido para instância: %s (restantes: %d)", instanceID, len(remaining))
}

// drop returns a new slice without target. It allocates rather than filtering in
// place, so a snapshot taken by a concurrent Produce can never observe a mutated
// backing array.
func drop(list []*client, target *client) []*client {
	out := make([]*client, 0, len(list))
	for _, c := range list {
		if c != target {
			out = append(out, c)
		}
	}
	return out
}

func (p *websocketProducer) Produce(queueName string, payload []byte, instanceID string, _ string) error {
	p.clientsMux.RLock()
	targets := append([]*client(nil), p.clients[instanceID]...)
	targets = append(targets, p.broadcast...)
	p.clientsMux.RUnlock()

	// A payload can carry a whole video as base64, so do not copy and encode it
	// for nobody.
	if len(targets) == 0 {
		return nil
	}

	// Encoded once: every subscriber receives the same bytes.
	frame, err := json.Marshal(map[string]interface{}{
		"queue":   strings.ToLower(queueName),
		"payload": string(payload),
	})
	if err != nil {
		return err
	}

	var failed []*client
	for _, c := range targets {
		if !c.enqueue(frame) {
			failed = append(failed, c)
		}
	}

	if len(failed) > 0 {
		p.loggerWrapper.GetLogger(instanceID).LogError(
			"Desconectando %d cliente(s) websocket lentos da instância %s", len(failed), instanceID)
		p.clientsMux.Lock()
		for _, c := range failed {
			if remaining := drop(p.clients[instanceID], c); len(remaining) == 0 {
				delete(p.clients, instanceID)
			} else {
				p.clients[instanceID] = remaining
			}
			p.broadcast = drop(p.broadcast, c)
		}
		p.clientsMux.Unlock()
		for _, c := range failed {
			c.close()
		}
	}

	if n := len(targets) - len(failed); n > 0 {
		p.loggerWrapper.GetLogger(instanceID).LogInfo(
			"Mensagem websocket enfileirada para %d cliente(s) da instância %s na fila %s", n, instanceID, queueName)
	}

	// Deliberately nil even when a subscriber was dropped: a dead browser tab is
	// not a delivery failure for the event.
	return nil
}

// CreateGlobalQueues não faz nada para websocket producer
func (p *websocketProducer) CreateGlobalQueues() error {
	return nil
}
