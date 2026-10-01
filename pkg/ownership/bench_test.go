package ownership

import "testing"

func BenchmarkLockKey(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = lockKey("2a2ddce2-99cf-44d1-b36b-addfefeb6596")
	}
}
