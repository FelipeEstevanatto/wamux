package server_handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cloud_service "github.com/felipeestevanatto/wamux/pkg/cloud"
	whatsmeow_service "github.com/felipeestevanatto/wamux/pkg/whatsmeow/service"
	"github.com/gin-gonic/gin"
)

type fakeMetricsProvider struct {
	stats whatsmeow_service.RuntimeStats
}

func (f fakeMetricsProvider) RuntimeStats() whatsmeow_service.RuntimeStats { return f.stats }

type fakeDepthSource struct{ depth int }

func (f fakeDepthSource) QueueDepth() int { return f.depth }

func TestMetricsHandlerRendersLiveValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &serverHandler{
		observability: NewObservability(
			fakeMetricsProvider{stats: whatsmeow_service.RuntimeStats{
				InstancesTotal:       10,
				InstancesConnected:   8,
				PersistQueueDepth:    3,
				PersistQueueCapacity: 4096,
				PersistDropped:       7,
				BgDropped:            1,
			}},
			fakeDepthSource{depth: 4},
		),
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.MetricsHandler(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type = %q", ct)
	}

	body := w.Body.String()
	for _, want := range []string{
		"evo_instances_total 10",
		"evo_connections_up 8",
		"evo_persist_queue_depth 3",
		"evo_persist_dropped_total 7",
		"evo_webhook_inflight 4",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestMetricsExposeProcessResources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &serverHandler{observability: NewObservability(nil, nil)}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.MetricsHandler(c)

	body := w.Body.String()
	for _, want := range []string{
		"evo_process_rss_bytes",
		"evo_process_heap_alloc_bytes",
		"evo_process_goroutines",
		"evo_process_cpu_seconds_total",
		"evo_process_cgroup_memory_bytes",
		"evo_uptime_seconds",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
}

func TestHealthHandlerReportsDegradedWhenNoConnections(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &serverHandler{
		version:  "0.8.1",
		overview: fakeHealthOverview{stats: whatsmeow_service.RuntimeStats{InstancesTotal: 5, InstancesConnected: 0}},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.HealthHandler(c)

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "degraded" {
		t.Fatalf("status = %v, want degraded", body["status"])
	}
}

func TestHealthHandlerOKWhenConnected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &serverHandler{
		version:  "0.8.1",
		overview: fakeHealthOverview{stats: whatsmeow_service.RuntimeStats{InstancesTotal: 5, InstancesConnected: 3}},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.HealthHandler(c)

	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["status"] != "ok" {
		t.Fatalf("status = %v, want ok", body["status"])
	}
	inst, _ := body["instances"].(map[string]interface{})
	if inst == nil || inst["connected"] != float64(3) {
		t.Fatalf("instances block wrong: %+v", body["instances"])
	}
}

// fakeHealthOverview satisfies both OverviewProvider and MetricsProvider.
type fakeHealthOverview struct {
	stats whatsmeow_service.RuntimeStats
}

func (f fakeHealthOverview) GetInstanceOverview(string) (*whatsmeow_service.InstanceOverview, error) {
	return nil, nil
}
func (f fakeHealthOverview) ResolveChats([]string) map[string]whatsmeow_service.ChatIdentity {
	return nil
}
func (f fakeHealthOverview) ResolveContacts(string, []string) map[string]whatsmeow_service.ChatIdentity {
	return nil
}
func (f fakeHealthOverview) ResolveSenders(string, string, []string) map[string]whatsmeow_service.ChatIdentity {
	return nil
}
func (f fakeHealthOverview) WhatsAppWebVersion() string { return "" }
func (f fakeHealthOverview) RuntimeStats() whatsmeow_service.RuntimeStats {
	return f.stats
}

type fakeCloudHealth struct{ snap cloud_service.HealthSnapshot }

func (f fakeCloudHealth) CloudHealth() cloud_service.HealthSnapshot { return f.snap }

func TestCloudMetricsAndHealthShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	o := NewObservability(nil, nil)
	o.ObserveCloudGraph("messages", 12*time.Millisecond, nil)
	o.ObserveCloudGraph("messages", 20*time.Millisecond, errors.New("boom"))
	o.ObserveCloudSend("text", "ok")
	o.ObserveCloudWebhook("messages", "ok")
	o.ObserveCloudWebhookSignatureFailure()
	o.ObserveCloudUnknownField("future")
	o.ObserveCloudWindowExpired()
	o.ObserveCloudMediaJob("stored")
	o.ObserveCloudRateLimit("phone", 7.5)
	o.ObserveCloudUnknownOutcome()
	o.SetCloudMediaQueueDepth(4)
	o.SetCloudHealth(fakeCloudHealth{snap: cloud_service.HealthSnapshot{
		Credentials: 2, Registered: 1, Verified: 2, Contacts: 10, Templates: 3,
		LastWebhookAt: time.Now().Add(-time.Minute),
	}})
	h := &serverHandler{observability: o, version: "test"}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.MetricsHandler(c)
	body := w.Body.String()
	for _, want := range []string{
		`wamux_cloud_graph_requests_total{operation="messages",result="ok"} 1`,
		`wamux_cloud_graph_requests_total{operation="messages",result="error"} 1`,
		`wamux_cloud_send_total{type="text",result="ok"} 1`,
		`wamux_cloud_webhook_signature_failures_total 1`,
		`wamux_cloud_webhook_unknown_total{field="future"} 1`,
		`wamux_cloud_window_expired_total 1`,
		`wamux_cloud_media_jobs_total{status="stored"} 1`,
		`wamux_cloud_media_queue_depth 4`,
		`wamux_cloud_send_outcome_unknown_total 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, "5511") {
		t.Fatalf("metrics leaked a phone number:\n%s", body)
	}

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	h.HealthHandler(c)
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	cloud, ok := payload["cloud"].(map[string]any)
	if !ok || cloud["credentials"].(float64) != 2 || cloud["lastWebhookAgeSeconds"] == nil {
		t.Fatalf("health = %+v", payload)
	}
}
