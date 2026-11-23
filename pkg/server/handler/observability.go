package server_handler

import (
	"sync/atomic"
	"time"

	cloud_service "github.com/felipeestevanatto/wamux/pkg/cloud"
	"github.com/felipeestevanatto/wamux/pkg/metrics"
	"github.com/felipeestevanatto/wamux/pkg/procstats"
	whatsmeow_service "github.com/felipeestevanatto/wamux/pkg/whatsmeow/service"
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

// CloudHealthProvider is implemented by the Cloud service. Keeping the method
// optional means a server without Cloud credentials still boots.
type CloudHealthProvider interface {
	CloudHealth() cloud_service.HealthSnapshot
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
	cloud    CloudHealthProvider

	// lifetime counters incremented by the send path.
	sendsTotal   *metrics.Counter
	sendFailures *metrics.Counter

	// sendLatencyLastMs is a gauge because this is a dependency-free registry;
	// it carries the most recent send duration so alerts can still fire on a
	// stalled send. (A histogram would be the richer choice with a full client
	// library.)
	sendLatencyLastMs *metrics.Gauge

	// perInstanceConnected is 1/0 per instance id, so a scrape shows WHICH
	// instance is down, not just how many.
	perInstanceConnected *metrics.GaugeVec
	// Per-instance health: numeric verdict, reconnect count in the window, and
	// age of the last event. The verdict mapping is documented in Render.
	perInstanceHealth     *metrics.GaugeVec
	perInstanceReconnects *metrics.GaugeVec
	perInstanceEventAge   *metrics.GaugeVec
	// cleanupDeleted counts rows removed by the retention job, by table.
	cleanupDeleted *metrics.GaugeVec

	// Cloud API operational series. Label values are deliberately bounded:
	// operation/result pairs, message kind/result and webhook field/result.
	cloudGraph          *metrics.CounterVec2
	cloudGraphLastMs    *metrics.GaugeVec
	cloudSend           *metrics.CounterVec2
	cloudWebhook        *metrics.CounterVec2
	cloudSignatureFail  *metrics.Counter
	cloudUnknownField   *metrics.Counter
	cloudMedia          *metrics.CounterVec
	cloudWindowExpired  *metrics.Counter
	cloudUnknownOutcome *metrics.Counter
	cloudMediaDepth     *metrics.Gauge
	cloudRateLimit      *metrics.GaugeVec

	startedAt atomic.Int64
}

// NewObservability builds the registry and registers the static series. The
// provider (connections, pools) and webhook depth source are optional.
func (o *Observability) SetCloudHealth(p CloudHealthProvider) {
	if o != nil {
		o.cloud = p
	}
}

func NewObservability(provider MetricsProvider, webhook MetricsSource) *Observability {
	reg := metrics.NewRegistry()
	o := &Observability{
		registry:              reg,
		provider:              provider,
		webhook:               webhook,
		sendsTotal:            reg.Counter("evo_sends_total", "Messages accepted for sending"),
		sendFailures:          reg.Counter("evo_send_failures_total", "Send attempts that returned an error"),
		sendLatencyLastMs:     reg.Gauge("evo_send_latency_last_ms", "Duration of the most recent send, in milliseconds"),
		perInstanceConnected:  reg.GaugeVec("evo_instance_connected", "Whether an instance is connected (1) or not (0)", "instance"),
		perInstanceHealth:     reg.GaugeVec("evo_instance_health", "Verdict: 0=closed, 1=ok, 2=reconnecting, 3=looping, 4=stalled, 5=unknown", "instance"),
		perInstanceReconnects: reg.GaugeVec("evo_instance_reconnects_total", "Reconnect opens in the last 10 minutes", "instance"),
		perInstanceEventAge:   reg.GaugeVec("evo_instance_last_event_age_seconds", "Seconds since the last event handler started", "instance"),
		cleanupDeleted:        reg.GaugeVec("evo_cleanup_deleted_total", "Rows removed by the retention job, by table", "table"),
		cloudGraph:            reg.CounterVec2("wamux_cloud_graph_requests_total", "Cloud Graph requests by operation and result", "operation", "result"),
		cloudGraphLastMs:      reg.GaugeVec("wamux_cloud_graph_duration_ms_last", "Most recent Cloud Graph request duration, in milliseconds, by operation", "operation"),
		cloudSend:             reg.CounterVec2("wamux_cloud_send_total", "Cloud sends by message type and result", "type", "result"),
		cloudWebhook:          reg.CounterVec2("wamux_cloud_webhook_total", "Cloud webhook changes by field and result", "field", "result"),
		cloudSignatureFail:    reg.Counter("wamux_cloud_webhook_signature_failures_total", "Cloud webhook POSTs rejected because the signature did not verify"),
		cloudUnknownField:     reg.Counter("wamux_cloud_webhook_unknown_total", "Cloud webhook changes with a field this build does not type"),
		cloudMedia:            reg.CounterVec("wamux_cloud_media_jobs_total", "Cloud inbound media jobs by final status", "status"),
		cloudWindowExpired:    reg.Counter("wamux_cloud_window_expired_total", "Cloud sends rejected because the customer service window was closed"),
		cloudUnknownOutcome:   reg.Counter("wamux_cloud_send_outcome_unknown_total", "Cloud sends whose Graph outcome is unknown (timeout/network)"),
		cloudMediaDepth:       reg.Gauge("wamux_cloud_media_queue_depth", "Inbound media jobs waiting for a worker"),
		cloudRateLimit:        reg.GaugeVec("wamux_cloud_rate_limit_rps", "Effective Cloud send rate, by scope", "scope"),
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

func (o *Observability) ObserveCloudGraph(operation string, d time.Duration, err error) {
	if o == nil || o.cloudGraph == nil {
		return
	}
	result := "ok"
	if err != nil {
		result = "error"
	}
	o.cloudGraph.Inc(operation, result)
	o.cloudGraphLastMs.Set(operation, d.Milliseconds())
}

func (o *Observability) ObserveCloudSend(kind, result string) {
	if o != nil && o.cloudSend != nil {
		o.cloudSend.Inc(kind, result)
	}
}

func (o *Observability) ObserveCloudWebhook(field, result string) {
	if o != nil && o.cloudWebhook != nil {
		o.cloudWebhook.Inc(field, result)
	}
}

func (o *Observability) ObserveCloudWebhookSignatureFailure() {
	if o != nil && o.cloudSignatureFail != nil {
		o.cloudSignatureFail.Inc()
	}
}

func (o *Observability) ObserveCloudUnknownField(string) {
	if o != nil && o.cloudUnknownField != nil {
		o.cloudUnknownField.Inc()
	}
}

func (o *Observability) ObserveCloudWindowExpired() {
	if o != nil && o.cloudWindowExpired != nil {
		o.cloudWindowExpired.Inc()
	}
}

func (o *Observability) ObserveCloudMediaJob(status string) {
	if o != nil && o.cloudMedia != nil {
		o.cloudMedia.WithLabelInc(status)
	}
}

func (o *Observability) ObserveCloudRateLimit(scope string, rps float64) {
	if o != nil && o.cloudRateLimit != nil {
		o.cloudRateLimit.Set(scope, int64(rps+0.5))
	}
}

func (o *Observability) ObserveCloudUnknownOutcome() {
	if o != nil && o.cloudUnknownOutcome != nil {
		o.cloudUnknownOutcome.Inc()
	}
}

func (o *Observability) SetCloudMediaQueueDepth(depth int) {
	if o != nil && o.cloudMediaDepth != nil {
		o.cloudMediaDepth.Set(int64(depth))
	}
}

// Compile-time check that the server observability registry implements the
// Cloud observer without a heap adapter.
var _ cloud_service.Observer = (*Observability)(nil)

// AddCleanupDeleted records rows removed by the retention job. Called from the
// cleanup goroutine; the gauge vector is lock-protected and atomic.
func (o *Observability) AddCleanupDeleted(table string, n int64) {
	if o == nil || o.cleanupDeleted == nil || n <= 0 {
		return
	}
	o.cleanupDeleted.Add(table, n)
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

		// Per-instance series. Emit the current set and delete stale ones (an
		// instance that was removed) so the series do not grow forever.
		seen := make(map[string]struct{}, len(s.Instances))
		now := time.Now()
		for _, inst := range s.Instances {
			v := int64(0)
			if inst.Connected {
				v = 1
			}
			o.perInstanceConnected.Set(inst.InstanceID, v)
			o.perInstanceHealth.Set(inst.InstanceID, healthVerdictValue(inst.Verdict))
			o.perInstanceReconnects.Set(inst.InstanceID, int64(inst.ReconnectOpens))
			age := int64(0)
			if inst.LastEventAt != "" {
				if at, err := time.Parse(time.RFC3339, inst.LastEventAt); err == nil {
					age = int64(now.Sub(at).Seconds())
					if age < 0 {
						age = 0
					}
				}
			}
			o.perInstanceEventAge.Set(inst.InstanceID, age)
			seen[inst.InstanceID] = struct{}{}
		}
		o.perInstanceConnected.DeleteMissing(seen)
		o.perInstanceHealth.DeleteMissing(seen)
		o.perInstanceReconnects.DeleteMissing(seen)
		o.perInstanceEventAge.DeleteMissing(seen)
	}

	if o.webhook != nil {
		setGauge(o.registry, "evo_webhook_inflight", "Webhook deliveries currently in flight", int64(o.webhook.QueueDepth()))
	}

	// Process resources. These are per-process, not host values, so they stay
	// meaningful inside a container and can alert on a leak or an impending OOM.
	ps := procstats.Read()
	setGauge(o.registry, "evo_process_rss_bytes", "Resident set size of this process", int64(ps.RSSBytes))
	setGauge(o.registry, "evo_process_peak_rss_bytes", "Peak resident set size of this process", int64(ps.PeakRSSBytes))
	setGauge(o.registry, "evo_process_virtual_bytes", "Virtual memory size of this process", int64(ps.VMSizeBytes))
	setGauge(o.registry, "evo_process_heap_alloc_bytes", "Go heap bytes currently allocated", int64(ps.HeapAllocBytes))
	setGauge(o.registry, "evo_process_heap_inuse_bytes", "Go heap bytes in use", int64(ps.HeapInuseBytes))
	setGauge(o.registry, "evo_process_sys_bytes", "Go runtime bytes obtained from the OS", int64(ps.SysBytes))
	setGauge(o.registry, "evo_process_goroutines", "Goroutines currently running", int64(ps.NumGoroutines))
	setGauge(o.registry, "evo_process_cgroup_memory_bytes", "Container cgroup memory usage", int64(ps.CgroupMemoryBytes))
	setGauge(o.registry, "evo_process_cgroup_memory_limit_bytes", "Container cgroup memory limit", int64(ps.CgroupMemoryLimit))
	setGauge(o.registry, "evo_process_cgroup_cpu_quota_milli", "Container cgroup CPU quota in millicores", int64(ps.CgroupCPUQuota))
	setGauge(o.registry, "evo_uptime_seconds", "Seconds since this process started", int64(time.Since(time.Unix(o.startedAt.Load(), 0)).Seconds()))

	setCounter := func(name, help string, v int64) {
		o.registry.Counter(name, help).Set(v)
	}
	setCounter("evo_process_gc_count_total", "Go garbage collections completed", int64(ps.NumGC))
	setCounter("evo_process_gc_pause_milliseconds_total", "Cumulative Go GC pause time in milliseconds", int64(ps.GCPauseTotalNs/1e6))
	setCounter("evo_process_cpu_seconds_total", "CPU seconds consumed by this process", int64(ps.CPUSeconds))

	setGauge(o.registry, "evo_process_start_timestamp", "Unix time this process started", o.startedAt.Load())

	return o.registry.Render()
}

// healthVerdictValue maps a verdict to a stable numeric Prometheus value.
func healthVerdictValue(verdict string) int64 {
	switch verdict {
	case "ok":
		return 1
	case "reconnecting":
		return 2
	case "looping":
		return 3
	case "stalled":
		return 4
	case "closed":
		return 0
	default:
		return 5
	}
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

	if s.observability != nil && s.observability.cloud != nil {
		snap := s.observability.cloud.CloudHealth()
		cloudBody := gin.H{
			"credentials": snap.Credentials,
			"registered":  snap.Registered,
			"verified":    snap.Verified,
			"contacts":    snap.Contacts,
			"templates":   snap.Templates,
		}
		if !snap.LastWebhookAt.IsZero() {
			cloudBody["lastWebhookAt"] = snap.LastWebhookAt.UTC().Format(time.RFC3339)
			cloudBody["lastWebhookAgeSeconds"] = int64(time.Since(snap.LastWebhookAt).Seconds())
		}
		body["cloud"] = cloudBody
	}

	ctx.JSON(200, body)
}
