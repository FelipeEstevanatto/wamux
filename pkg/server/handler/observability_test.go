package server_handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
func (f fakeHealthOverview) WhatsAppWebVersion() string { return "" }
func (f fakeHealthOverview) RuntimeStats() whatsmeow_service.RuntimeStats {
	return f.stats
}
