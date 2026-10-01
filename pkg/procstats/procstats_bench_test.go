package procstats

import "testing"

func BenchmarkRead(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Read()
	}
}
