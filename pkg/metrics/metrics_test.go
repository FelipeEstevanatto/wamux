package metrics

import (
	"strings"
	"testing"
)

func TestCounterAndGaugeRender(t *testing.T) {
	r := NewRegistry()
	c := r.Counter("evo_sends_total", "Messages sent")
	c.Add(3)
	c.Inc()

	g := r.Gauge("evo_connections_up", "Connected instances")
	g.Set(2)

	out := r.Render()
	for _, want := range []string{
		"# HELP evo_sends_total Messages sent",
		"# TYPE evo_sends_total counter",
		"evo_sends_total 4",
		"# TYPE evo_connections_up gauge",
		"evo_connections_up 2",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q in:\n%s", want, out)
		}
	}
}

func TestLabelledFamilies(t *testing.T) {
	r := NewRegistry()
	vec := r.CounterVec("evo_http_requests_total", "HTTP requests", "group")
	vec.WithLabelInc("send")
	vec.WithLabelInc("send")
	vec.WithLabelInc("instance")

	out := r.Render()
	if !strings.Contains(out, `evo_http_requests_total{group="send"} 2`) {
		t.Fatalf("send series wrong:\n%s", out)
	}
	if !strings.Contains(out, `evo_http_requests_total{group="instance"} 1`) {
		t.Fatalf("instance series wrong:\n%s", out)
	}
}

func TestLabelValuesAreEscaped(t *testing.T) {
	r := NewRegistry()
	vec := r.CounterVec("x_total", "x", "id")
	vec.WithLabelInc(`a"b\c`)

	out := r.Render()
	if !strings.Contains(out, `x_total{id="a\"b\\c"} 1`) {
		t.Fatalf("label value not escaped:\n%s", out)
	}
}

// Output must be stable regardless of insertion order, so scrapes diff cleanly.
func TestRenderIsSorted(t *testing.T) {
	r := NewRegistry()
	r.Gauge("b_metric", "").Set(1)
	r.Gauge("a_metric", "").Set(1)

	out := r.Render()
	if strings.Index(out, "a_metric") > strings.Index(out, "b_metric") {
		t.Fatalf("metrics not sorted:\n%s", out)
	}
}

func TestRegisteringSameMetricIsIdempotent(t *testing.T) {
	r := NewRegistry()
	a := r.Counter("c_total", "one")
	a.Inc()
	// Registering again must return the same underlying counter, not reset it.
	b := r.Counter("c_total", "one")
	if b.Value() != 1 {
		t.Fatalf("re-register reset the counter: %d", b.Value())
	}
}

func TestGaugeVecDelete(t *testing.T) {
	r := NewRegistry()
	vec := r.GaugeVec("evo_instance_x", "x", "instance")
	vec.Set("abc", 5)
	vec.Delete("abc")
	if strings.Contains(r.Render(), "abc") {
		t.Fatalf("deleted series still rendered:\n%s", r.Render())
	}
}
