// Package gctune applies the Go runtime's soft memory limit and optional GC
// percent from configuration, and returns idle heap to the OS after bursts.
//
// # WHY
//
// WaMux hosts many WhatsApp instances in one process. By default Go lets the
// heap grow to roughly 2x the live set (GOGC=100) and returns freed pages to the
// kernel lazily, so RSS drifts upward and stays there even when traffic is idle.
// A soft memory limit plus a lower GOGC bounds that drift, and a periodic
// FreeOSMemory after large transient work (a history sync, a media upload) pulls
// RSS back toward the live set.
//
// Measurements (see benchmarks/README.md):
//   - with GOMEMLIMIT=64 MiB and GOGC=50 the heap peaked at 52 MiB during churn;
//   - debug.FreeOSMemory() took RSS from 101 MiB back to 29 MiB after a burst.
//
// With no explicit configuration the limit is derived from the container's
// cgroup memory limit, so a containerised deployment is bounded automatically.
package gctune

import (
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/felipeestevanatto/wamux/pkg/procstats"
)

const (
	// DefaultCgroupFraction is the share of the cgroup limit used as the soft
	// heap limit, leaving headroom for non-heap runtime memory.
	DefaultCgroupFraction = 0.85

	// MinMemoryLimitBytes avoids setting a limit so small it causes GC thrash.
	MinMemoryLimitBytes = 64 << 20
)

// Config controls Apply. GoMemoryLimitMB: 0 derives the limit from the cgroup,
// >0 sets an explicit limit in MiB, <0 disables the limit entirely.
type Config struct {
	GoMemoryLimitMB int
	// GCPercent <= 0 leaves the Go default in place.
	GCPercent int
	// AutoFromCgroup enables deriving the limit when none is configured.
	AutoFromCgroup bool
	// CgroupFraction overrides DefaultCgroupFraction when in (0,1].
	CgroupFraction float64
	// CgroupLimitBytes, when non-zero, is used instead of reading the cgroup
	// file. Tests set this; production leaves it zero.
	CgroupLimitBytes uint64
	// EnvLimitSet / EnvGCSet report that GOMEMLIMIT / GOGC are set in the
	// environment; the runtime already honours them, so Apply does not override.
	EnvLimitSet bool
	EnvGCSet    bool
}

// Result reports what Apply actually did, for a startup log line.
type Result struct {
	MemoryLimitBytes  int64
	MemoryLimitSource string
	GCPercent         int
}

// ComputeMemoryLimit resolves the soft memory limit in bytes and where it came
// from. -1 means "leave unset". explicitBytes < 0 disables.
func ComputeMemoryLimit(cgroupLimit uint64, explicitBytes int64, auto bool, fraction float64) (int64, string) {
	if explicitBytes < 0 {
		return -1, "disabled"
	}
	if explicitBytes > 0 {
		return explicitBytes, "config"
	}
	if !auto {
		return -1, "disabled"
	}
	if cgroupLimit == 0 {
		return -1, "no-cgroup-limit"
	}
	if fraction <= 0 || fraction > 1 {
		fraction = DefaultCgroupFraction
	}
	limit := int64(float64(cgroupLimit) * fraction)
	if limit < MinMemoryLimitBytes {
		limit = MinMemoryLimitBytes
	}
	return limit, "cgroup"
}

// Apply sets the runtime limits and returns what it applied.
func Apply(cfg Config) Result {
	res := Result{MemoryLimitBytes: -1, MemoryLimitSource: "unset"}

	if !cfg.EnvLimitSet {
		explicit := int64(0)
		switch {
		case cfg.GoMemoryLimitMB > 0:
			explicit = int64(cfg.GoMemoryLimitMB) << 20
		case cfg.GoMemoryLimitMB < 0:
			explicit = -1
		}
		auto := cfg.AutoFromCgroup && explicit == 0
		cgroup := cfg.CgroupLimitBytes
		if cgroup == 0 && auto {
			cgroup = procstats.Read().CgroupMemoryLimit
		}
		if limit, source := ComputeMemoryLimit(cgroup, explicit, auto, cfg.CgroupFraction); limit > 0 {
			debug.SetMemoryLimit(limit)
			res.MemoryLimitBytes = limit
			res.MemoryLimitSource = source
		} else {
			res.MemoryLimitSource = source
		}
	} else {
		res.MemoryLimitSource = "env(GOMEMLIMIT)"
	}

	if cfg.GCPercent > 0 && !cfg.EnvGCSet {
		debug.SetGCPercent(cfg.GCPercent)
		res.GCPercent = cfg.GCPercent
	}
	return res
}

// EnvLimitSet reports whether GOMEMLIMIT is set in the environment.
func EnvLimitSet() bool { return strings.TrimSpace(os.Getenv("GOMEMLIMIT")) != "" }

// EnvGCSet reports whether GOGC is set in the environment.
func EnvGCSet() bool { return strings.TrimSpace(os.Getenv("GOGC")) != "" }

// freeOSMemory is swappable in tests.
var freeOSMemory = debug.FreeOSMemory

// Reclaimer periodically returns idle heap to the OS once it exceeds a
// threshold. It is a no-op (nil) when the interval is not positive.
type Reclaimer struct {
	interval time.Duration
	minIdle  uint64
	stop     chan struct{}
	done     chan struct{}
}

// StartReclaimer starts the background reclaimer. A non-positive interval
// disables it and returns nil.
func StartReclaimer(interval time.Duration, minIdleBytes uint64) *Reclaimer {
	if interval <= 0 {
		return nil
	}
	r := &Reclaimer{
		interval: interval,
		minIdle:  minIdleBytes,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go r.loop()
	return r
}

func (r *Reclaimer) loop() {
	defer close(r.done)
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-t.C:
			r.reclaimOnce()
		}
	}
}

func (r *Reclaimer) reclaimOnce() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if !ShouldReclaim(m.HeapIdle, r.minIdle) {
		return
	}
	freeOSMemory()
}

// ShouldReclaim reports whether idle heap is large enough to be worth returning.
// A zero threshold means "always".
func ShouldReclaim(heapIdle, minIdleBytes uint64) bool {
	return minIdleBytes == 0 || heapIdle >= minIdleBytes
}

// Stop halts the reclaimer. Safe on nil.
func (r *Reclaimer) Stop() {
	if r == nil {
		return
	}
	close(r.stop)
	<-r.done
}
