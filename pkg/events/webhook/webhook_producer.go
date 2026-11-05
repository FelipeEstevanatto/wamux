package webhook_producer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	producer_interfaces "github.com/felipeestevanatto/wamux/pkg/events/interfaces"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	"github.com/felipeestevanatto/wamux/pkg/webhooksign"
)

const (
	// deliveryTimeout bounds one attempt end to end. Without it a webhook
	// endpoint that accepts the connection and never answers pins a goroutine
	// forever, and the retry below never gets to run.
	deliveryTimeout = 30 * time.Second

	// maxDeliveryAttempts counts the first try plus retries.
	maxDeliveryAttempts = 5

	// retryBaseDelay is doubled each attempt up to retryMaxDelay, so a briefly
	// unavailable endpoint is retried quickly while a long outage backs off.
	retryBaseDelay = 2 * time.Second
	retryMaxDelay  = 60 * time.Second

	// maxResponseBytes caps what is read back. The body is only logged, so a
	// misbehaving endpoint must not be able to stream unbounded data into memory.
	maxResponseBytes = 8 * 1024

	// Delivery capacity is budgeted per endpoint, never shared. A dead endpoint
	// retries for minutes, so a shared budget meant one tenant's outage dropped
	// every other tenant's events.
	//
	// endpointConcurrency bounds goroutines and sockets per URL.
	endpointConcurrency = 8

	// endpointQueueSize and maxEndpointQueueBytes bound the backlog per URL. Bytes
	// matter as well as count: an event can carry base64 media.
	endpointQueueSize     = 256
	maxEndpointQueueBytes = 64 << 20
)

// errQueueFull is the dead-letter reason for an event that never got a delivery
// attempt because its endpoint's backlog was full.
var errQueueFull = errors.New("webhook endpoint backlog is full")

// delivery is one event bound for one URL.
type delivery struct {
	url     string
	payload []byte
	userID  string
	hmacKey []byte
}

// endpoint is one URL's backlog and running workers, guarded by webhookProducer.mu.
// It only exists while it has work.
type endpoint struct {
	backlog      []delivery
	backlogBytes int
	workers      int
}

type webhookProducer struct {
	url           string
	client        *http.Client
	loggerWrapper *logger_wrapper.LoggerManager

	mu        sync.Mutex
	endpoints map[string]*endpoint
	// pending counts deliveries queued or being attempted, for QueueDepth.
	pending atomic.Int64

	// Retry knobs, defaulted from the constants above. They are fields rather
	// than plain constants so tests can shrink the waits instead of sleeping
	// through a real backoff.
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration

	// deadLetter, when set, receives webhooks that failed permanently (all
	// attempts exhausted, or a non-retryable response) so an operator can
	// inspect/replay them. errorQueue is the destination queue name.
	deadLetter producer_interfaces.Producer
	errorQueue string
}

// Option configures a webhookProducer.
type Option func(*webhookProducer)

// WithDeadLetterQueue routes permanently failed deliveries to publisher under
// queueName. It is normally wired only when RabbitMQ is configured.
func WithDeadLetterQueue(publisher producer_interfaces.Producer, queueName string) Option {
	return func(p *webhookProducer) {
		p.deadLetter = publisher
		p.errorQueue = queueName
	}
}

func NewWebhookProducer(
	url string,
	loggerWrapper *logger_wrapper.LoggerManager,
	opts ...Option,
) producer_interfaces.Producer {
	transport := (http.DefaultTransport.(*http.Transport)).Clone()
	// Webhooks are usually a handful of endpoints receiving many events, so keep
	// connections warm instead of paying a TLS handshake per delivery.
	transport.MaxIdleConnsPerHost = 16

	p := &webhookProducer{
		url:           url,
		client:        &http.Client{Transport: transport, Timeout: deliveryTimeout},
		endpoints:     make(map[string]*endpoint),
		loggerWrapper: loggerWrapper,
		maxAttempts:   maxDeliveryAttempts,
		baseDelay:     retryBaseDelay,
		maxDelay:      retryMaxDelay,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Produce fans one event out to the global webhook and the instance's own.
//
// Delivery is asynchronous, so a nil return means "accepted for delivery", not
// "delivered" — the outcome is reported through the logs.
func (p *webhookProducer) Produce(
	queueName string,
	payload []byte,
	webhookUrl string,
	userID string,
) error {
	return p.produce(queueName, payload, webhookUrl, userID, nil)
}

// ProduceSigned is Produce plus an HMAC-SHA256 signature over the exact bytes
// that are sent, so a consumer can authenticate the body. An empty key falls
// back to the unsigned behaviour of Produce.
func (p *webhookProducer) ProduceSigned(
	queueName string,
	payload []byte,
	webhookUrl string,
	userID string,
	hmacKey []byte,
) error {
	return p.produce(queueName, payload, webhookUrl, userID, hmacKey)
}

func (p *webhookProducer) produce(
	queueName string,
	payload []byte,
	webhookUrl string,
	userID string,
	hmacKey []byte,
) error {
	// The queue name is meaningless for HTTP delivery: it names an AMQP queue,
	// and the webhook body already carries the event. It used to be parsed here
	// and anything without a dot was dropped, which silently discarded events
	// posted under a bare name such as "sendstatus".
	if p.url != "" {
		p.deliver(p.url, payload, userID, hmacKey)
	}

	// Multiple webhooks per instance. The instance's Webhook field may contain
	// several URLs (newline/comma/semicolon separated, or a JSON array); the same
	// payload is delivered to each. Fully backwards compatible with one URL.
	// A URL equal to the global webhook is skipped: it already got the event.
	for _, url := range splitWebhookURLs(webhookUrl) {
		if url == p.url {
			continue
		}
		p.deliver(url, payload, userID, hmacKey)
	}

	return nil
}

// splitWebhookURLs splits an instance's webhook field into one or more URLs.
// Accepts a JSON array (["https://a","https://b"]) or a newline/comma/semicolon
// separated list. Empty entries, duplicates and the "disabled" marker are
// dropped.
func splitWebhookURLs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "disabled" {
		return nil
	}

	var parts []string
	if strings.HasPrefix(raw, "[") {
		var arr []string
		if err := json.Unmarshal([]byte(raw), &arr); err == nil {
			parts = arr
		}
	}
	if parts == nil {
		parts = strings.FieldsFunc(raw, func(r rune) bool {
			return r == '\n' || r == '\r' || r == ',' || r == ';'
		})
	}

	out := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "disabled" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// deliver queues one delivery on its endpoint. It never blocks. An event that
// does not fit means that endpoint is far behind, so it goes to the dead-letter
// queue rather than growing memory or affecting any other endpoint.
func (p *webhookProducer) deliver(url string, payload []byte, userID string, hmacKey []byte) {
	if p.enqueue(delivery{url: url, payload: payload, userID: userID, hmacKey: hmacKey}) {
		return
	}

	p.loggerWrapper.GetLogger(userID).LogError(
		"[%s] webhook dropped, endpoint backlog is full - url: %s", userID, url,
	)
	p.publishDeadLetter(url, payload, userID, 0, nil, errQueueFull)
}

// enqueue adds d to its endpoint's backlog, starting a worker if the endpoint has
// capacity. It reports false when the backlog is full.
func (p *webhookProducer) enqueue(d delivery) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	ep, ok := p.endpoints[d.url]
	if !ok {
		ep = &endpoint{}
		p.endpoints[d.url] = ep
	}

	// A lone oversized event is still accepted: the byte budget exists to stop a
	// backlog building up, not to make a large event undeliverable.
	overBudget := ep.backlogBytes > 0 && ep.backlogBytes+len(d.payload) > maxEndpointQueueBytes
	if len(ep.backlog) >= endpointQueueSize || overBudget {
		return false
	}

	ep.backlog = append(ep.backlog, d)
	ep.backlogBytes += len(d.payload)
	p.pending.Add(1)

	if ep.workers < endpointConcurrency {
		ep.workers++
		go p.drain(d.url, ep)
	}
	return true
}

// drain delivers from one endpoint's backlog until it is empty. Retries run here,
// so a failing endpoint only ever occupies its own workers.
func (p *webhookProducer) drain(url string, ep *endpoint) {
	for {
		d, ok := p.next(url, ep)
		if !ok {
			return
		}
		p.sendWebhookWithRetry(d.url, d.payload, d.userID, d.hmacKey)
		p.pending.Add(-1)
	}
}

// next pops the oldest delivery, or retires this worker when there is none.
// Popping and retiring share the lock with enqueue, so a delivery can never be
// left behind by a worker that has just decided to exit.
func (p *webhookProducer) next(url string, ep *endpoint) (delivery, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(ep.backlog) == 0 {
		ep.workers--
		if ep.workers == 0 {
			delete(p.endpoints, url)
		}
		return delivery{}, false
	}

	d := ep.backlog[0]
	ep.backlog[0] = delivery{} // release the payload
	ep.backlog = ep.backlog[1:]
	ep.backlogBytes -= len(d.payload)
	return d, true
}

func (p *webhookProducer) sendWebhookWithRetry(url string, body []byte, userID string, hmacKey []byte) {
	logger := p.loggerWrapper.GetLogger(userID)
	delay := p.baseDelay

	var lastStatus int
	var lastResponse []byte
	var lastErr error

	for attempt := 1; attempt <= p.maxAttempts; attempt++ {
		statusCode, responseBody, retryable, err := p.sendWebhook(url, body, hmacKey)
		lastStatus, lastResponse, lastErr = statusCode, responseBody, err
		if err == nil {
			logger.LogInfo(
				"[%s] webhook delivered - url: %s, status: %d, attempt: %d, response: %s",
				userID, url, statusCode, attempt, string(responseBody),
			)
			return
		}

		// 4xx means the endpoint understood the request and refused it. Retrying
		// cannot change that, and five attempts a minute apart only hammer it.
		if !retryable {
			logger.LogError(
				"[%s] webhook rejected, not retrying - url: %s, status: %d, error: %v, response: %s",
				userID, url, statusCode, err, string(responseBody),
			)
			p.publishDeadLetter(url, body, userID, lastStatus, lastResponse, lastErr)
			return
		}

		if attempt == p.maxAttempts {
			break
		}

		logger.LogWarn(
			"[%s] webhook failed, retrying in %s - url: %s, attempt: %d/%d, error: %v",
			userID, delay, url, attempt, p.maxAttempts, err,
		)
		time.Sleep(delay)

		if delay < p.maxDelay {
			delay *= 2
			if delay > p.maxDelay {
				delay = p.maxDelay
			}
		}
	}

	logger.LogError("[%s] webhook failed after %d attempts - url: %s", userID, p.maxAttempts, url)
	p.publishDeadLetter(url, body, userID, lastStatus, lastResponse, lastErr)
}

// publishDeadLetter sends a permanently failed delivery to the configured
// dead-letter queue. It is a no-op when no queue is configured, and never
// fails the caller: the webhook has already failed, so a dead-letter failure
// is only logged.
func (p *webhookProducer) publishDeadLetter(url string, body []byte, userID string, statusCode int, responseBody []byte, err error) {
	if p.deadLetter == nil || p.errorQueue == "" {
		return
	}

	payload := json.RawMessage(body)
	if !json.Valid(body) {
		if quoted, qErr := json.Marshal(string(body)); qErr == nil {
			payload = quoted
		}
	}

	errText := ""
	if err != nil {
		errText = err.Error()
	}

	envelope, mErr := json.Marshal(map[string]any{
		"url":         url,
		"userID":      userID,
		"payload":     payload,
		"statusCode":  statusCode,
		"response":    string(responseBody),
		"attemptTime": time.Now().UTC().Format(time.RFC3339),
		"error":       errText,
	})
	if mErr != nil {
		p.loggerWrapper.GetLogger(userID).LogError("[%s] failed to encode dead-letter payload for %s: %v", userID, url, mErr)
		return
	}

	if perr := p.deadLetter.Produce(p.errorQueue, envelope, "enabled", userID); perr != nil {
		p.loggerWrapper.GetLogger(userID).LogError("[%s] failed to publish webhook to dead-letter queue %s: %v", userID, p.errorQueue, perr)
		return
	}
	p.loggerWrapper.GetLogger(userID).LogWarn("[%s] webhook moved to dead-letter queue %s - url: %s", userID, p.errorQueue, url)
}

// sendWebhook performs one attempt. retryable reports whether trying again could
// plausibly succeed: network failures and 5xx yes, an outright refusal no.
func (p *webhookProducer) sendWebhook(url string, body []byte, hmacKey []byte) (status int, response []byte, retryable bool, err error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		// A malformed URL will not fix itself on the next attempt.
		return 0, nil, false, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "WaMux-Webhook/1.0")
	if len(hmacKey) > 0 {
		req.Header.Set(webhooksign.SignatureHeader, webhooksign.Sign(hmacKey, body))
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, nil, true, err
	}
	defer resp.Body.Close()

	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if readErr != nil {
		return resp.StatusCode, nil, true, fmt.Errorf("failed to read response: %w", readErr)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp.StatusCode, responseBody, false, nil
	}

	// 408 and 429 are the two 4xx that do resolve on their own.
	retryable = resp.StatusCode >= 500 ||
		resp.StatusCode == http.StatusRequestTimeout ||
		resp.StatusCode == http.StatusTooManyRequests

	return resp.StatusCode, responseBody, retryable, fmt.Errorf("received non-2xx response: %s", resp.Status)
}

// CreateGlobalQueues does nothing for the webhook producer.
// QueueDepth reports how many webhook deliveries are queued or being attempted,
// across every endpoint. A sustained high value means an endpoint is slow.
func (p *webhookProducer) QueueDepth() int {
	return int(p.pending.Load())
}

func (p *webhookProducer) CreateGlobalQueues() error {
	return nil
}
