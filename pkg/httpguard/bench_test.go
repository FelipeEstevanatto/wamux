package httpguard

import (
	"strconv"
	"testing"
	"time"
)

func BenchmarkLimiterAllow(b *testing.B) {
	l := NewLimiter(1_000_000, time.Minute)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Allow("instance-" + strconv.Itoa(i%100))
	}
}

func BenchmarkSendGuardAcquireRelease(b *testing.B) {
	g := NewSendGuard(0, 1_000_000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g.acquire("inst")
		g.release("inst")
	}
}
