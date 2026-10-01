package server_handler

import (
	"sync/atomic"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/metrics"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"github.com/gin-gonic/gin"
)

// MetricsProvider is the slice of the services the observability layer samples.
// Declaring it as an interface keeps the handler testable with a fake.
type MetricsProvider interface {
	RuntimeStats() whatsmeow_service.RuntimeStats
}

// MetricsSource is anything that can report a live count of in-flight work
// (the webhook producer's delivery semaphore). Optional: nil means "not wired".
type MetricsSource interface {
	QueueDepth() int
}

// Observability holds the process metrics registry and the sampler that fills
// it.
//
// Gauges are refreshed lazily on each scrape rather than by a background ticker:
// the values are cheap to read (atomic counters + map sizes), and sampling only
// when someone is looking keeps an idle server idle.
//
// It is constructed by main so the same instance can also serve as the send
// observer, wiring send outcomes and the /metrics scrape to one registry.
type Observability struct {
	registry *metrics.Registry
	provider MetricsProvider
	webhook  MetricsSource

	// lifetime counters incremented by the send path.
	sendsTotal   *metrics.Counter
	sendFailures *metrics.Counter

	// sendLatencyLastMs is a gauge because this is a dependency-free registry;
	// it carries the most recent send duration so alerts can still fire on a
	// stalled send. (A histogram would be the richer choice with a full client
	// library.)
	sendLatencyLastMs *metrics.Gauge

	startedAt atomic.Int64
}

// NewObservability builds the registry and registers the static series. The
// provider (connections, pools) and webhook depth source are optional.
func NewObservability(provider MetricsProvider, webhook MetricsSource) *Observability {
	reg := metrics.NewRegistry()
	o := &Observability{
		registry:          reg,
		provider:          provider,
		webhook:           webhook,
		sendsTotal:        reg.Counter("evo_sends_total", "Messages accepted for sending"),
		sendFailures:      reg.Counter("evo_send_failures_total", "Send attempts that returned an error"),
		sendLatencyLastMs: reg.Gauge("evo_send_latency_last_ms", "Duration of the most recent send, in milliseconds"),
	}
	o.startedAt.Store(time.Now().Unix())
	return o
}

// ObserveSend implements send_service.SendObserver: it records the send
// outcome and the most recent duration. Cheap and lock-free (atomics only), as
// it runs on the send path.
func (o *Observability) ObserveSend(d time.Duration, err error) {
	if o == nil {
		return
	}
	o.sendsTotal.Inc()
	if err != nil {
		o.sendFailures.Inc()
	}
	o.sendLatencyLastMs.Set(d.Milliseconds())
}

// Render refreshes the live gauges and returns the Prometheus exposition text.
func (o *Observability) Render() string {
	if o == nil {
		return "# observability disabled\n"
	}

	if o.provider != nil {
		s := o.provider.RuntimeStats()
		setGauge(o.registry, "evo_instances_total", "Instances known to this process", int64(s.InstancesTotal))
		setGauge(o.registry, "evo_connections_up", "Connected WhatsApp instances", int64(s.InstancesConnected))
		setGauge(o.registry, "evo_persist_queue_depth", "Messages waiting to be written to the database", int64(s.PersistQueueDepth))
		setGauge(o.registry, "evo_persist_queue_capacity", "Capacity of the message persistence queue", int64(s.PersistQueueCapacity))
		setGauge(o.registry, "evo_persist_dropped_total", "Messages whose persistence queue was full (written inline instead)", int64(s.PersistDropped))
		setGauge(o.registry, "evo_bg_dropped_total", "Background jobs dropped because the pool was full", int64(s.BgDropped))
	}

	if o.webhook != nil {
		setGauge(o.registry, "evo_webhook_inflight", "Webhook deliveries currently in flight", int64(o.webhook.QueueDepth()))
	}

	setGauge(o.registry, "evo_process_start_timestamp", "Unix time this process started", o.startedAt.Load())

	return o.registry.Render()
}

// setGauge registers a gauge on first use and sets it. Registering lazily keeps
// new series additive: adding a metric does not require touching a central list.
func setGauge(reg *metrics.Registry, name, help string, v int64) {
	reg.Gauge(name, help).Set(v)
}

// MetricsHandler serves GET /metrics in the Prometheus text format.
func (s *serverHandler) MetricsHandler(ctx *gin.Context) {
	ctx.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	ctx.String(200, s.observability.Render())
}

// HealthHandler serves GET /server/health: a compact liveness/readiness summary
// that is cheap for an orchestrator or uptime check to poll. Unlike /metrics it
// is human-oriented JSON.
func (s *serverHandler) HealthHandler(ctx *gin.Context) {
	body := gin.H{
		"status":  "ok",
		"version": s.version,
		"uptime":  int64(time.Since(s.startTime).Seconds()),
	}

	if s.overview != nil {
		if mp, ok := s.overview.(MetricsProvider); ok {
			rs := mp.RuntimeStats()
			body["instances"] = gin.H{
				"total":     rs.InstancesTotal,
				"connected": rs.InstancesConnected,
			}
			body["persist"] = gin.H{
				"queueDepth": rs.PersistQueueDepth,
				"capacity":   rs.PersistQueueCapacity,
				"dropped":    rs.PersistDropped,
			}
			// A process with known instances but none connected is degraded, not
			// healthy — surface that so a check can act on it.
			if rs.InstancesTotal > 0 && rs.InstancesConnected == 0 {
				body["status"] = "degraded"
				body["reason"] = "no connected instances"
			}
		}
	}

	if s.configError != "" {
		body["status"] = "degraded"
		body["error"] = s.configError
	}

	ctx.JSON(200, body)
}
