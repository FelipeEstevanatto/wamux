package gctune

import (
	"runtime/debug"
	"sync/atomic"
	"testing"
	"time"
)

func TestComputeMemoryLimit(t *testing.T) {
	const gib = 1 << 30
	cases := []struct {
		name     string
		cgroup   uint64
		explicit int64
		auto     bool
		fraction float64
		want     int64
		wantSrc  string
	}{
		{"explicit", 0, 128 << 20, true, 0, 128 << 20, "config"},
		{"disabled explicit", 0, -1, true, 0, -1, "disabled"},
		{"auto from cgroup", 4 * gib, 0, true, 0.85, int64(4*gib) * 85 / 100, "cgroup"},
		{"no cgroup limit", 0, 0, true, 0, -1, "no-cgroup-limit"},
		{"auto disabled", 4 * gib, 0, false, 0, -1, "disabled"},
		{"min floor", 32 << 20, 0, true, 0.5, MinMemoryLimitBytes, "cgroup"},
		{"bad fraction falls back", 4 * gib, 0, true, 2, int64(4*gib) * 85 / 100, "cgroup"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, src := ComputeMemoryLimit(tc.cgroup, tc.explicit, tc.auto, tc.fraction)
			if got != tc.want || src != tc.wantSrc {
				t.Fatalf("got (%d, %q), want (%d, %q)", got, src, tc.want, tc.wantSrc)
			}
		})
	}
}

// saveRestoreRuntime snapshots the runtime limits and restores them after the
// test, so gctune tests do not leak a soft limit into other packages.
func saveRestoreRuntime(t *testing.T) {
	t.Helper()
	prevLimit := debug.SetMemoryLimit(-1) // returns previous, disables
	prevGC := debug.SetGCPercent(100)
	t.Cleanup(func() {
		debug.SetMemoryLimit(prevLimit)
		debug.SetGCPercent(prevGC)
	})
}

func TestApplyExplicitLimitAndGC(t *testing.T) {
	saveRestoreRuntime(t)

	res := Apply(Config{
		GoMemoryLimitMB:  128,
		GCPercent:        50,
		AutoFromCgroup:   true,
		CgroupLimitBytes: 4 << 30,
	})
	if res.MemoryLimitBytes != 128<<20 || res.MemoryLimitSource != "config" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.GCPercent != 50 {
		t.Fatalf("GCPercent not applied: %+v", res)
	}

	// Read the live values back (Set returns the previous value).
	cur := debug.SetMemoryLimit(-1)
	debug.SetMemoryLimit(cur)
	if cur != 128<<20 {
		t.Fatalf("runtime memory limit = %d, want %d", cur, 128<<20)
	}
	prevGC := debug.SetGCPercent(100)
	debug.SetGCPercent(prevGC)
	if prevGC != 50 {
		t.Fatalf("runtime GC percent = %d, want 50", prevGC)
	}
}

func TestApplyDoesNotOverrideEnv(t *testing.T) {
	saveRestoreRuntime(t)

	res := Apply(Config{
		GoMemoryLimitMB: 128,
		GCPercent:       50,
		EnvLimitSet:     true,
		EnvGCSet:        true,
	})
	if res.MemoryLimitBytes != -1 || res.MemoryLimitSource != "env(GOMEMLIMIT)" {
		t.Fatalf("expected env to win for the limit, got %+v", res)
	}
	if res.GCPercent != 0 {
		t.Fatalf("expected env to win for GC percent, got %+v", res)
	}
}

func TestApplyAutoFromCgroup(t *testing.T) {
	saveRestoreRuntime(t)

	res := Apply(Config{AutoFromCgroup: true, CgroupLimitBytes: 2 << 30})
	want := int64(2<<30) * 85 / 100
	if res.MemoryLimitBytes != want || res.MemoryLimitSource != "cgroup" {
		t.Fatalf("got %+v, want limit %d from cgroup", res, want)
	}
}

func TestApplyDisabled(t *testing.T) {
	saveRestoreRuntime(t)
	res := Apply(Config{GoMemoryLimitMB: -1, AutoFromCgroup: true, CgroupLimitBytes: 2 << 30})
	if res.MemoryLimitBytes != -1 || res.MemoryLimitSource != "disabled" {
		t.Fatalf("got %+v, want disabled", res)
	}
}

func TestShouldReclaim(t *testing.T) {
	if ShouldReclaim(1<<20, 64<<20) {
		t.Fatal("small idle heap should not trigger")
	}
	if !ShouldReclaim(64<<20, 64<<20) {
		t.Fatal("idle heap at threshold should trigger")
	}
	if !ShouldReclaim(1, 0) {
		t.Fatal("zero threshold means always")
	}
}

func TestReclaimerRunsAndStops(t *testing.T) {
	var calls atomic.Int64
	prev := freeOSMemory
	freeOSMemory = func() { calls.Add(1) }
	t.Cleanup(func() { freeOSMemory = prev })

	r := StartReclaimer(5*time.Millisecond, 0)
	if r == nil {
		t.Fatal("reclaimer should start with a positive interval")
	}
	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if calls.Load() == 0 {
		t.Fatal("reclaimer never fired")
	}
	r.Stop()

	after := calls.Load()
	time.Sleep(20 * time.Millisecond)
	if calls.Load() != after {
		t.Fatal("reclaimer fired after Stop")
	}

	if StartReclaimer(0, 0) != nil {
		t.Fatal("zero interval should disable the reclaimer")
	}
}
