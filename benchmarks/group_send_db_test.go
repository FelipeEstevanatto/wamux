package benchmarks

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.mau.fi/libsignal/session"
	"go.mau.fi/libsignal/tests"
	"go.mau.fi/libsignal/util/keyhelper"
)

// Scenario: the one per-device database round trip that whatsmeow still pays
// inside the group-send loop.
//
// Sessions and LIDs are batched (GetManySessions / CachedLIDMap), but
// SQLStore.IsTrustedIdentity issues a SELECT per device and is NOT batched, so
// it runs once per distinct device on every group send. This benchmark models a
// ~1ms database round trip per device (time.Sleep is used, so the real per-call
// cost is Linux's ~1ms timer granularity, i.e. a slow/remote database) and
// compares it with the real pkg/whatsmeow/identitycache. The authoritative
// before/after against Postgres is BenchmarkIdentityLookupPostgres*.

const benchDBLatency = 1 * time.Millisecond

func newLatencySender(n int, cached bool) (*benchSender, []*session.Cipher) {
	ikp, _ := keyhelper.GenerateIdentityKeyPair()
	store := &latencyIdentityStore{
		InMemoryIdentityKey: tests.NewInMemoryIdentityKey(ikp, keyhelper.GenerateRegistrationID()),
		perCall:             benchDBLatency,
		cache:               cached,
	}
	s := newBenchSender(store)
	return s, s.sessions(benchRemotes(n))
}

// BenchmarkGroupSendDBLatencyUncached pays benchDBLatency per device per send.
func BenchmarkGroupSendDBLatencyUncached(b *testing.B) {
	for _, n := range []int{100, 300, 600} {
		b.Run(fmt.Sprintf("devices=%d", n), func(b *testing.B) {
			ctx := context.Background()
			s, ciphers := newLatencySender(n, false)
			msg := benchMsg()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := s.groupCipher.Encrypt(ctx, msg); err != nil {
					b.Fatal(err)
				}
				skdm := latestSKDM(b, s)
				for _, c := range ciphers {
					if _, err := c.Encrypt(ctx, skdm); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

// BenchmarkGroupSendDBLatencyCached removes the per-device round trip after the
// cache warms, so the delta against the uncached run is the cost of the missing
// identity cache.
func BenchmarkGroupSendDBLatencyCached(b *testing.B) {
	for _, n := range []int{100, 300, 600} {
		b.Run(fmt.Sprintf("devices=%d", n), func(b *testing.B) {
			ctx := context.Background()
			s, ciphers := newLatencySender(n, true)
			msg := benchMsg()
			// Warm the cache with one full send.
			for _, c := range ciphers {
				if _, err := c.Encrypt(ctx, latestSKDM(b, s)); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := s.groupCipher.Encrypt(ctx, msg); err != nil {
					b.Fatal(err)
				}
				skdm := latestSKDM(b, s)
				for _, c := range ciphers {
					if _, err := c.Encrypt(ctx, skdm); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
