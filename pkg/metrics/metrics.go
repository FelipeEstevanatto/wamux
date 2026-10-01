// Package metrics is a small, dependency-free metrics registry that renders the
// Prometheus text exposition format.
//
// # WHY THIS EXISTS
//
// The service had rich logs and a /server/stats JSON blob, but nothing an
// operator could scrape or alert on. When 300 instances are up and one wedges,
// there was no counter to graph and no gauge to alert on — only log lines. A
// full client library (prometheus/client_golang) would work but pulls a large
// dependency tree into a binary that otherwise has few; the exposition format is
// simple enough to emit directly, and it keeps the hot paths allocation-free.
//
// # WHAT IT EXPOSES
//
// Only series that answer an operational question: is a connection down, is a
// queue backing up, are we dropping work, how slow are sends. Deliberately small
// so it stays cheap to maintain and cheap to scrape.
package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Registry holds the metrics for one process. Counters and gauges are atomic;
// the label sets are fixed at registration time, so reads never allocate on the
// hot path beyond a map lookup for the labelled families.
type Registry struct {
	// scalar counters/gauges keyed by metric name.
	scalars sync.Map // string -> *scalar

	// labelled families: name -> *labelledFamily
	families sync.Map // string -> *labelledFamily
}

type scalarKind int

const (
	kindCounter scalarKind = iota
	kindGauge
)

type scalar struct {
	name string
	help string
	kind scalarKind
	val  atomic.Int64
}

type labelledFamily struct {
	name      string
	help      string
	labelName string
	kind      scalarKind
	mu        sync.RWMutex
	series    map[string]*atomic.Int64 // key -> value; key is the label value
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Counter registers (once) a monotonically increasing metric and returns it.
func (r *Registry) Counter(name, help string) *Counter {
	return &Counter{s: r.scalar(name, help, kindCounter)}
}

// Gauge registers (once) a metric that can go up and down.
func (r *Registry) Gauge(name, help string) *Gauge {
	return &Gauge{s: r.scalar(name, help, kindGauge)}
}

func (r *Registry) scalar(name, help string, kind scalarKind) *scalar {
	if existing, ok := r.scalars.Load(name); ok {
		return existing.(*scalar)
	}
	s := &scalar{name: name, help: help, kind: kind}
	actual, _ := r.scalars.LoadOrStore(name, s)
	return actual.(*scalar)
}

// CounterVec is a counter with a fixed label name.
func (r *Registry) CounterVec(name, help, label string) *CounterVec {
	return &CounterVec{fam: r.family(name, help, label, kindCounter), label: label}
}

// GaugeVec is a gauge with a fixed label name.
func (r *Registry) GaugeVec(name, help, label string) *GaugeVec {
	return &GaugeVec{fam: r.family(name, help, label, kindGauge), label: label}
}

func (r *Registry) family(name, help, label string, kind scalarKind) *labelledFamily {
	if existing, ok := r.families.Load(name); ok {
		return existing.(*labelledFamily)
	}
	f := &labelledFamily{name: name, help: help, labelName: label, kind: kind, series: map[string]*atomic.Int64{}}
	actual, _ := r.families.LoadOrStore(name, f)
	return actual.(*labelledFamily)
}

// Counter is a single integer counter.
type Counter struct{ s *scalar }

// Inc adds one.
func (c *Counter) Inc() { c.Add(1) }

// Add adds n.
func (c *Counter) Add(n int64) { c.s.val.Add(n) }

// Value reads the current total.
func (c *Counter) Value() int64 { return c.s.val.Load() }

// Gauge is a single integer gauge.
type Gauge struct{ s *scalar }

// Set replaces the value.
func (g *Gauge) Set(n int64) { g.s.val.Store(n) }

// Add adds n (may be negative).
func (g *Gauge) Add(n int64) { g.s.val.Add(n) }

// Value reads the current value.
func (g *Gauge) Value() int64 { return g.s.val.Load() }

// CounterVec is a counter partitioned by one label.
type CounterVec struct {
	fam   *labelledFamily
	label string
}

// WithLabelInc increments the series for labelValue.
func (c *CounterVec) WithLabelInc(labelValue string) {
	c.fam.value(c.label, labelValue).Add(1)
}

// GaugeVec is a gauge partitioned by one label.
type GaugeVec struct {
	fam   *labelledFamily
	label string
}

// Set sets the value for labelValue.
func (g *GaugeVec) Set(labelValue string, n int64) {
	g.fam.value(g.label, labelValue).Store(n)
}

// Add adjusts the value for labelValue.
func (g *GaugeVec) Add(labelValue string, n int64) {
	g.fam.value(g.label, labelValue).Add(n)
}

// Delete drops a series (e.g. an instance that no longer exists).
func (g *GaugeVec) Delete(labelValue string) {
	key := labelValue
	g.fam.mu.Lock()
	delete(g.fam.series, key)
	g.fam.mu.Unlock()
}

func (f *labelledFamily) value(_, labelValue string) *atomic.Int64 {
	f.mu.RLock()
	v, ok := f.series[labelValue]
	f.mu.RUnlock()
	if ok {
		return v
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if v, ok := f.series[labelValue]; ok {
		return v
	}
	v = &atomic.Int64{}
	f.series[labelValue] = v
	return v
}

// Render writes the whole registry in the Prometheus text exposition format.
//
// Example:
//
//	# HELP evo_connections_up Connected WhatsApp instances
//	# TYPE evo_connections_up gauge
//	evo_connections_up 3
func (r *Registry) Render() string {
	var b strings.Builder

	// Collect and sort names so the output is stable (scrape-diff friendly and
	// easy to eyeball).
	var scalarNames, familyNames []string
	r.scalars.Range(func(k, _ any) bool { scalarNames = append(scalarNames, k.(string)); return true })
	r.families.Range(func(k, _ any) bool { familyNames = append(familyNames, k.(string)); return true })
	sort.Strings(scalarNames)
	sort.Strings(familyNames)

	for _, name := range scalarNames {
		v, _ := r.scalars.Load(name)
		s := v.(*scalar)
		writeHeader(&b, name, s.help, s.kind)
		fmt.Fprintf(&b, "%s %d\n", name, s.val.Load())
	}

	for _, name := range familyNames {
		v, _ := r.families.Load(name)
		f := v.(*labelledFamily)
		writeHeader(&b, name, f.help, f.kind)

		f.mu.RLock()
		keys := make([]string, 0, len(f.series))
		for k := range f.series {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "%s{%s=\"%s\"} %d\n", name, f.labelName, escape(k), f.series[k].Load())
		}
		f.mu.RUnlock()
	}

	return b.String()
}

// escape escapes the label value per the exposition format (backslash, quote,
// newline). Instance ids and JIDs are well-formed, but tokens can be arbitrary.
func escape(s string) string {
	if !strings.ContainsAny(s, "\\\"\n") {
		return s
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(s)
}

func writeHeader(b *strings.Builder, name, help string, kind scalarKind) {
	if help != "" {
		fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	}
	typ := "counter"
	if kind == kindGauge {
		typ = "gauge"
	}
	fmt.Fprintf(b, "# TYPE %s %s\n", name, typ)
}
