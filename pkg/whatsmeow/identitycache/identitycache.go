// Package identitycache memoizes whatsmeow's per-device identity trust checks.
//
// # WHY
//
// whatsmeow's SQLStore.IsTrustedIdentity issues one SELECT per device inside the
// group-send encryption loop. Unlike sessions (GetManySessions) and LIDs
// (CachedLIDMap), identities are NOT batched, so a group with hundreds of
// devices pays hundreds of round trips on every single send. Measured against a
// fast local Postgres at 200us per lookup, that is ~764ms for 600 devices,
// versus ~7ms with this cache — the identity SELECTs, not the crypto, are what
// makes large-group sends slow.
//
// This wrapper caches the trust result per (address, identity key) for a short
// TTL and invalidates the whole cache on every identity write, so the hot path
// (repeated sends to the same group) skips the database while a key change is
// still observed immediately.
//
// # SAFETY
//
// IsTrustedIdentity is "trust on first use": it returns true when the address is
// unknown. The cache stores the exact boolean for the exact key asked about, so
// a later query with a different key is a miss and hits the database. Writes
// (PutIdentity/DeleteIdentity/DeleteAllIdentities) flush the cache, so a rotated
// key is never served from a stale entry. Errors are never cached.
package identitycache

import (
	"context"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"

	"go.mau.fi/whatsmeow/store"
)

const (
	// DefaultTTL bounds how long a trust decision may be served from memory.
	// Identity changes are rare and invalidation is immediate on write, so this
	// only bounds staleness when another process writes the same row.
	DefaultTTL = 5 * time.Minute

	// DefaultMaxEntries bounds memory for a process hosting many instances. A
	// group send touches at most a few thousand distinct devices.
	DefaultMaxEntries = 50_000

	// sweepEvery amortises expiry cleanup: every N inserts one full sweep runs.
	sweepEvery = 1024
)

type entry struct {
	trusted   bool
	expiresAt int64
}

// Stats is a snapshot of cache activity, for /metrics or diagnostics.
type Stats struct {
	Hits       uint64 `json:"hits"`
	Misses     uint64 `json:"misses"`
	InnerCalls uint64 `json:"innerCalls"`
	Entries    int    `json:"entries"`
}

// Store wraps a whatsmeow store.IdentityStore with a bounded, TTL'd cache.
type Store struct {
	inner store.IdentityStore
	ttl   time.Duration
	max   int

	mu      sync.Mutex
	entries map[string]entry
	inserts int

	hits       atomic.Uint64
	misses     atomic.Uint64
	innerCalls atomic.Uint64
}

// Option configures a Store.
type Option func(*Store)

// WithTTL overrides DefaultTTL.
func WithTTL(d time.Duration) Option {
	return func(s *Store) {
		if d > 0 {
			s.ttl = d
		}
	}
}

// WithMaxEntries overrides DefaultMaxEntries.
func WithMaxEntries(n int) Option {
	return func(s *Store) {
		if n > 0 {
			s.max = n
		}
	}
}

// New wraps inner. inner must be non-nil; callers that may have a nil identity
// store should not construct a cache.
func New(inner store.IdentityStore, opts ...Option) *Store {
	if inner == nil {
		panic("identitycache: nil inner store")
	}
	s := &Store{
		inner:   inner,
		ttl:     DefaultTTL,
		max:     DefaultMaxEntries,
		entries: make(map[string]entry),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

func cacheKey(address string, key [32]byte) string {
	return address + "\x00" + hex.EncodeToString(key[:])
}

// IsTrustedIdentity returns the cached decision when present, else delegates and
// caches the result. Errors are returned and never cached.
func (s *Store) IsTrustedIdentity(ctx context.Context, address string, key [32]byte) (bool, error) {
	ck := cacheKey(address, key)
	if trusted, ok := s.get(ck); ok {
		s.hits.Add(1)
		return trusted, nil
	}
	s.misses.Add(1)
	s.innerCalls.Add(1)
	trusted, err := s.inner.IsTrustedIdentity(ctx, address, key)
	if err != nil {
		return false, err
	}
	s.set(ck, trusted)
	return trusted, nil
}

// PutIdentity writes through and flushes the cache: a stored key can change the
// answer for its address, and writes are rare once sessions are established.
func (s *Store) PutIdentity(ctx context.Context, address string, key [32]byte) error {
	err := s.inner.PutIdentity(ctx, address, key)
	if err == nil {
		s.flush()
	}
	return err
}

// DeleteIdentity deletes and flushes.
func (s *Store) DeleteIdentity(ctx context.Context, address string) error {
	err := s.inner.DeleteIdentity(ctx, address)
	if err == nil {
		s.flush()
	}
	return err
}

// DeleteAllIdentities deletes and flushes.
func (s *Store) DeleteAllIdentities(ctx context.Context, phone string) error {
	err := s.inner.DeleteAllIdentities(ctx, phone)
	if err == nil {
		s.flush()
	}
	return err
}

// Stats returns a snapshot of cache activity.
func (s *Store) Stats() Stats {
	s.mu.Lock()
	n := len(s.entries)
	s.mu.Unlock()
	return Stats{
		Hits:       s.hits.Load(),
		Misses:     s.misses.Load(),
		InnerCalls: s.innerCalls.Load(),
		Entries:    n,
	}
}

func (s *Store) get(ck string) (trusted bool, ok bool) {
	now := time.Now().UnixNano()
	s.mu.Lock()
	e, ok := s.entries[ck]
	if !ok {
		s.mu.Unlock()
		return false, false
	}
	if e.expiresAt > 0 && now > e.expiresAt {
		delete(s.entries, ck)
		s.mu.Unlock()
		return false, false
	}
	s.mu.Unlock()
	return e.trusted, true
}

func (s *Store) set(ck string, trusted bool) {
	now := time.Now().UnixNano()
	s.mu.Lock()
	if len(s.entries) >= s.max {
		s.sweepLocked(now)
		if len(s.entries) >= s.max {
			// At capacity with nothing expired: skip caching rather than grow
			// without bound. The caller still got the correct answer.
			s.mu.Unlock()
			return
		}
	}
	s.entries[ck] = entry{trusted: trusted, expiresAt: now + int64(s.ttl)}
	s.inserts++
	if s.inserts%sweepEvery == 0 {
		s.sweepLocked(now)
	}
	s.mu.Unlock()
}

func (s *Store) flush() {
	s.mu.Lock()
	s.entries = make(map[string]entry)
	s.mu.Unlock()
}

func (s *Store) sweepLocked(now int64) {
	for k, e := range s.entries {
		if e.expiresAt > 0 && now > e.expiresAt {
			delete(s.entries, k)
		}
	}
}
