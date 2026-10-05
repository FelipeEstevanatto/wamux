package benchmarks

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/felipeestevanatto/wamux/pkg/whatsmeow/identitycache"
	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// Scenario: the identity trust lookup against a REAL Postgres, cached vs
// uncached. This is the before/after for pkg/whatsmeow/identitycache.
//
//	EVO_BENCH_POSTGRES_DSN='postgresql://user:pass@localhost:55432/bench?sslmode=disable' \
//	  go test -run=^$ -bench='IdentityLookupPostgres' ./benchmarks/ -benchtime=5x

const benchIdentityDevices = 600

func identityKey(i int) [32]byte {
	var k [32]byte
	k[0] = byte(i)
	k[1] = byte(i >> 8)
	return k
}

func identityDevices() []string {
	out := make([]string, benchIdentityDevices)
	for i := range out {
		out[i] = fmt.Sprintf("bench-%d:1", i)
	}
	return out
}

// ensureBenchDevice creates the owning device row the identity table's foreign
// key requires. It is a no-op when the row already exists.
func ensureBenchDevice(ctx context.Context, container *sqlstore.Container, jid types.JID) error {
	if d, err := container.GetDevice(ctx, jid); err != nil {
		return err
	} else if d != nil {
		return nil
	}
	d := container.NewDevice()
	d.ID = &jid
	// The device row has NOT NULL adv_* columns even for an unpaired bench
	// device, so use empty (non-nil) blobs.
	d.Account = &waAdv.ADVSignedDeviceIdentity{
		Details:             []byte{0},
		AccountSignature:    make([]byte, 64),
		AccountSignatureKey: make([]byte, 32),
		DeviceSignature:     make([]byte, 64),
	}
	return container.PutDevice(ctx, d)
}

// benchPostgresIdentityStore opens a real Postgres, seeds benchIdentityDevices
// identities and returns the raw SQLStore plus a cleanup.
func benchPostgresIdentityStore(b *testing.B) (store.IdentityStore, []string, func()) {
	b.Helper()
	dsn := os.Getenv("EVO_BENCH_POSTGRES_DSN")
	if dsn == "" {
		b.Skip("set EVO_BENCH_POSTGRES_DSN to run the real identity-cache comparison")
	}
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "postgres", dsn, waLog.Noop)
	if err != nil {
		b.Fatalf("open sqlstore: %v", err)
	}
	jid := types.NewJID("bench", types.DefaultUserServer)
	if err := ensureBenchDevice(ctx, container, jid); err != nil {
		container.Close()
		b.Fatalf("ensure device: %v", err)
	}
	raw := sqlstore.NewSQLStore(container, jid)

	devices := identityDevices()
	for i, addr := range devices {
		if err := raw.PutIdentity(ctx, addr, identityKey(i)); err != nil {
			container.Close()
			b.Fatalf("seed identity: %v", err)
		}
	}
	cleanup := func() {
		for _, addr := range devices {
			_ = raw.DeleteIdentity(ctx, addr)
		}
		_ = container.Close()
	}
	return raw, devices, cleanup
}

func BenchmarkIdentityLookupPostgresUncached(b *testing.B) {
	raw, devices, cleanup := benchPostgresIdentityStore(b)
	defer cleanup()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j, addr := range devices {
			if _, err := raw.IsTrustedIdentity(ctx, addr, identityKey(j)); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkIdentityLookupPostgresCached(b *testing.B) {
	raw, devices, cleanup := benchPostgresIdentityStore(b)
	defer cleanup()
	cache := identitycache.New(raw)
	ctx := context.Background()
	// warm
	for j, addr := range devices {
		if _, err := cache.IsTrustedIdentity(ctx, addr, identityKey(j)); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j, addr := range devices {
			if _, err := cache.IsTrustedIdentity(ctx, addr, identityKey(j)); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.StopTimer()
	b.Logf("cache stats after %d ops: %+v", b.N, cache.Stats())
}

// TestIdentityCachePostgres validates invalidation end to end against a real
// database, so the cached benchmark is known to reflect database state.
func TestIdentityCachePostgres(t *testing.T) {
	dsn := os.Getenv("EVO_BENCH_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set EVO_BENCH_POSTGRES_DSN to run the real identity-cache validation")
	}
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "postgres", dsn, waLog.Noop)
	if err != nil {
		t.Fatalf("open sqlstore: %v", err)
	}
	defer container.Close()
	jid := types.NewJID("bench", types.DefaultUserServer)
	if err := ensureBenchDevice(ctx, container, jid); err != nil {
		t.Fatalf("ensure device: %v", err)
	}
	raw := sqlstore.NewSQLStore(container, jid)
	const addr = "bench-validation:1"
	defer raw.DeleteIdentity(ctx, addr)

	if err := raw.PutIdentity(ctx, addr, identityKey(1)); err != nil {
		t.Fatalf("put: %v", err)
	}
	cache := identitycache.New(raw)
	if trusted, err := cache.IsTrustedIdentity(ctx, addr, identityKey(1)); err != nil || !trusted {
		t.Fatalf("want trusted: %v %v", trusted, err)
	}
	if trusted, _ := cache.IsTrustedIdentity(ctx, addr, identityKey(2)); trusted {
		t.Fatal("key(2) must be untrusted")
	}
	// Rotate behind the cache's back: the wrapper's own PutIdentity must
	// invalidate, but writing through the raw store then checking would serve a
	// stale entry, so rotate through the wrapper.
	if err := cache.PutIdentity(ctx, addr, identityKey(2)); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if trusted, err := cache.IsTrustedIdentity(ctx, addr, identityKey(2)); err != nil || !trusted {
		t.Fatalf("want trusted after rotation: %v %v", trusted, err)
	}
}
