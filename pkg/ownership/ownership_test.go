package ownership

import (
	"context"
	"testing"
)

func TestUnsupportedGuardAlwaysClaims(t *testing.T) {
	// No db: single-node behaviour. Every claim succeeds and Release is a no-op.
	g := NewGuard(nil, "")
	if g.Supported() {
		t.Fatalf("nil db should not be supported")
	}

	held, holder, err := g.Claim(context.Background(), "instance-1")
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if !held.Held {
		t.Fatalf("unsupported guard should always claim")
	}
	if err := holder.Release(context.Background()); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

func TestSQLiteGuardIsUnsupported(t *testing.T) {
	g := NewGuard(nil, "sqlite")
	if g.Supported() {
		t.Fatalf("sqlite guard should not be supported")
	}
	held, _, err := g.Claim(context.Background(), "x")
	if err != nil || !held.Held {
		t.Fatalf("sqlite claim = %+v, %v; want claimed", held, err)
	}
}

func TestLockKeyIsStableAndDistinct(t *testing.T) {
	a1 := lockKey("abc")
	a2 := lockKey("abc")
	if a1 != a2 {
		t.Fatalf("lock key not stable: %d != %d", a1, a2)
	}
	if lockKey("abc") == lockKey("abd") {
		t.Fatalf("different ids should (almost surely) hash differently")
	}
}
