package identitycache

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeStore is an in-memory identity store that counts how often the database
// path is hit, so the tests can prove the cache actually removes calls.
type fakeStore struct {
	mu    sync.Mutex
	ident map[string][32]byte
	calls int
	err   error
}

func newFake() *fakeStore { return &fakeStore{ident: make(map[string][32]byte)} }

func (f *fakeStore) IsTrustedIdentity(_ context.Context, address string, key [32]byte) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return false, f.err
	}
	existing, ok := f.ident[address]
	if !ok {
		return true, nil // trust on first use
	}
	return existing == key, nil
}

func (f *fakeStore) PutIdentity(_ context.Context, address string, key [32]byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.ident[address] = key
	return nil
}

func (f *fakeStore) DeleteIdentity(_ context.Context, address string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	delete(f.ident, address)
	return nil
}

func (f *fakeStore) DeleteAllIdentities(_ context.Context, phone string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	for k := range f.ident {
		if strings.HasPrefix(k, phone+":") {
			delete(f.ident, k)
		}
	}
	return nil
}

func (f *fakeStore) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func key(b byte) [32]byte {
	var k [32]byte
	k[0] = b
	return k
}

func TestCacheHitAvoidsInner(t *testing.T) {
	inner := newFake()
	s := New(inner)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		trusted, err := s.IsTrustedIdentity(ctx, "a:1", key(1))
		if err != nil || !trusted {
			t.Fatalf("want trusted, got %v, %v", trusted, err)
		}
	}
	if got := inner.callCount(); got != 1 {
		t.Fatalf("inner called %d times, want 1", got)
	}
	if st := s.Stats(); st.Hits != 4 || st.Misses != 1 || st.InnerCalls != 1 {
		t.Fatalf("unexpected stats: %+v", st)
	}
}

func TestDifferentKeysAreSeparateEntries(t *testing.T) {
	inner := newFake()
	_ = inner.PutIdentity(context.Background(), "a:1", key(1))
	s := New(inner)
	ctx := context.Background()

	if trusted, _ := s.IsTrustedIdentity(ctx, "a:1", key(1)); !trusted {
		t.Fatal("key(1) should be trusted")
	}
	if trusted, _ := s.IsTrustedIdentity(ctx, "a:1", key(2)); trusted {
		t.Fatal("key(2) must not be trusted")
	}
	if got := inner.callCount(); got != 2 {
		t.Fatalf("inner called %d times, want 2", got)
	}
}

func TestPutIdentityInvalidates(t *testing.T) {
	inner := newFake()
	s := New(inner)
	ctx := context.Background()

	// Untrusted against the stored key.
	if err := inner.PutIdentity(ctx, "a:1", key(1)); err != nil {
		t.Fatal(err)
	}
	if trusted, _ := s.IsTrustedIdentity(ctx, "a:1", key(2)); trusted {
		t.Fatal("key(2) should be untrusted")
	}
	before := inner.callCount()

	// Rotate the stored key through the wrapper; the cache must not serve the
	// stale answer.
	if err := s.PutIdentity(ctx, "a:1", key(2)); err != nil {
		t.Fatal(err)
	}
	if trusted, _ := s.IsTrustedIdentity(ctx, "a:1", key(2)); !trusted {
		t.Fatal("key(2) should now be trusted")
	}
	if inner.callCount() == before {
		t.Fatal("expected a database call after invalidation")
	}
}

func TestDeleteInvalidates(t *testing.T) {
	inner := newFake()
	s := New(inner)
	ctx := context.Background()

	if err := s.PutIdentity(ctx, "a:1", key(1)); err != nil {
		t.Fatal(err)
	}
	if trusted, _ := s.IsTrustedIdentity(ctx, "a:1", key(1)); !trusted {
		t.Fatal("should be trusted")
	}
	if err := s.DeleteIdentity(ctx, "a:1"); err != nil {
		t.Fatal(err)
	}
	before := inner.callCount()
	// After delete the address is unknown -> trust on first use.
	if trusted, _ := s.IsTrustedIdentity(ctx, "a:1", key(1)); !trusted {
		t.Fatal("deleted identity should read as trusted (unknown)")
	}
	if inner.callCount() == before {
		t.Fatal("expected a fresh database call after delete")
	}
}

func TestDeleteAllInvalidates(t *testing.T) {
	inner := newFake()
	s := New(inner)
	ctx := context.Background()
	if err := s.PutIdentity(ctx, "123:1", key(1)); err != nil {
		t.Fatal(err)
	}
	if trusted, _ := s.IsTrustedIdentity(ctx, "123:1", key(1)); !trusted {
		t.Fatal("should be trusted")
	}
	if err := s.DeleteAllIdentities(ctx, "123"); err != nil {
		t.Fatal(err)
	}
	before := inner.callCount()
	if trusted, _ := s.IsTrustedIdentity(ctx, "123:1", key(1)); !trusted {
		t.Fatal("after delete-all the address should be unknown/trusted")
	}
	if inner.callCount() == before {
		t.Fatal("expected a fresh database call after delete-all")
	}
}

func TestErrorsAreNotCached(t *testing.T) {
	inner := newFake()
	s := New(inner)
	ctx := context.Background()
	inner.err = errors.New("db down")

	if _, err := s.IsTrustedIdentity(ctx, "a:1", key(1)); err == nil {
		t.Fatal("expected error")
	}
	inner.err = nil
	if trusted, err := s.IsTrustedIdentity(ctx, "a:1", key(1)); err != nil || !trusted {
		t.Fatalf("recovery failed: %v, %v", trusted, err)
	}
}

func TestTTLExpiryRefetches(t *testing.T) {
	inner := newFake()
	s := New(inner, WithTTL(15*time.Millisecond))
	ctx := context.Background()

	if _, err := s.IsTrustedIdentity(ctx, "a:1", key(1)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, err := s.IsTrustedIdentity(ctx, "a:1", key(1)); err != nil {
		t.Fatal(err)
	}
	if got := inner.callCount(); got != 2 {
		t.Fatalf("inner called %d times, want 2 after TTL expiry", got)
	}
}

func TestMaxEntriesIsRespected(t *testing.T) {
	inner := newFake()
	s := New(inner, WithMaxEntries(2), WithTTL(time.Hour))
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		if _, err := s.IsTrustedIdentity(ctx, fmt.Sprintf("a:%d", i), key(byte(i))); err != nil {
			t.Fatal(err)
		}
	}
	if st := s.Stats(); st.Entries > 2 {
		t.Fatalf("entries grew past max: %d", st.Entries)
	}
}

func TestConcurrentUse(t *testing.T) {
	inner := newFake()
	s := New(inner)
	ctx := context.Background()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				if _, err := s.IsTrustedIdentity(ctx, fmt.Sprintf("a:%d", i%16), key(byte(i%4))); err != nil {
					t.Error(err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

// slowStore models a Postgres round trip on every IsTrustedIdentity call.
type slowStore struct {
	*fakeStore
	delay time.Duration
}

func (s *slowStore) IsTrustedIdentity(ctx context.Context, address string, k [32]byte) (bool, error) {
	time.Sleep(s.delay)
	return s.fakeStore.IsTrustedIdentity(ctx, address, k)
}

func BenchmarkIsTrustedUncached(b *testing.B) {
	inner := &slowStore{fakeStore: newFake(), delay: 200 * time.Microsecond}
	ctx := context.Background()
	devices := benchDevices(600)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, d := range devices {
			if _, err := inner.IsTrustedIdentity(ctx, d, key(1)); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkIsTrustedCached(b *testing.B) {
	inner := &slowStore{fakeStore: newFake(), delay: 200 * time.Microsecond}
	s := New(inner)
	ctx := context.Background()
	devices := benchDevices(600)
	// warm
	for _, d := range devices {
		if _, err := s.IsTrustedIdentity(ctx, d, key(1)); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, d := range devices {
			if _, err := s.IsTrustedIdentity(ctx, d, key(1)); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkIsTrustedCachedParallel exercises the cache under concurrent
// send/decrypt callers, where the internal lock is the only contention point.
func BenchmarkIsTrustedCachedParallel(b *testing.B) {
	inner := &slowStore{fakeStore: newFake(), delay: 200 * time.Microsecond}
	s := New(inner)
	ctx := context.Background()
	devices := benchDevices(600)
	for _, d := range devices {
		if _, err := s.IsTrustedIdentity(ctx, d, key(1)); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if _, err := s.IsTrustedIdentity(ctx, devices[i%len(devices)], key(1)); err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

func benchDevices(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("dev-%d:1", i)
	}
	return out
}
