package metrics

import "testing"

func BenchmarkCounterInc(b *testing.B) {
	r := NewRegistry()
	c := r.Counter("c_total", "x")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Inc()
	}
}

func BenchmarkGaugeVecSet(b *testing.B) {
	r := NewRegistry()
	g := r.GaugeVec("g", "x", "instance")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.Set("instance-1", 1)
	}
}

// Render runs on every scrape; it must stay cheap even with many series.
func BenchmarkRenderWithManySeries(b *testing.B) {
	r := NewRegistry()
	g := r.GaugeVec("g", "x", "instance")
	for i := 0; i < 500; i++ {
		g.Set("instance-"+string(rune('a'+i%26))+string(rune('a'+(i/26)%26)), 1)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Render()
	}
}
