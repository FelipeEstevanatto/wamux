package benchmarks

import (
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
)

// TestClientMemory measures the live-heap overhead of N idle whatsmeow clients.
// It is the lower bound on per-session cost: a connected instance adds websocket
// buffers, populated caches and history, but the client struct itself is tiny.
func TestClientMemory(t *testing.T) {
	const n = 200
	runtime.GC()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	clients := make([]*whatsmeow.Client, 0, n)
	for i := 0; i < n; i++ {
		// Distinct Device per client; all-backed stores are shared no-ops.
		d := *store.NoopDevice
		clients = append(clients, whatsmeow.NewClient(&d, nil))
	}

	runtime.GC()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	delta := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	per := float64(delta) / float64(n)
	if per <= 0 {
		t.Fatalf("measured non-positive per-client heap: %.2f bytes", per)
	}
	// Guardrail: the client struct is on the order of tens of KiB. If it ever
	// crosses 1 MiB, something per-client and large was added and deserves a
	// look before it multiplies by the number of instances.
	if per > 1<<20 {
		t.Fatalf("per-client heap %.1f KiB exceeds 1 MiB guardrail", per/1024)
	}
	t.Logf("clients=%d per_client=%.2f KiB (heap delta %d bytes)", n, per/1024, delta)
	runtime.KeepAlive(clients)
}

// BenchmarkNewClient measures the allocation cost of starting one session.
func BenchmarkNewClient(b *testing.B) {
	b.ReportAllocs()
	var sink *whatsmeow.Client
	for i := 0; i < b.N; i++ {
		d := *store.NoopDevice
		sink = whatsmeow.NewClient(&d, nil)
	}
	runtime.KeepAlive(sink)
}

// readRSSBytes returns the process resident set size on Linux, or ok=false.
func readRSSBytes() (uint64, bool) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, false
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, false
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return pages * uint64(os.Getpagesize()), true
}

// TestFreeOSMemoryReclaimsTransientHeap validates the "return memory after a
// burst" lever: a large transient allocation (a history sync, a media upload)
// leaves the heap grown because Go's scavenger returns pages lazily. Calling
// debug.FreeOSMemory after the burst pulls RSS back toward baseline.
func TestFreeOSMemoryReclaimsTransientHeap(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates 64 MiB transiently")
	}
	base, ok := readRSSBytes()
	if !ok {
		t.Skip("RSS is only available on Linux")
	}

	hold := make([]byte, 64<<20)
	for i := 0; i < len(hold); i += 4096 {
		hold[i] = 1
	}
	peak, _ := readRSSBytes()
	runtime.KeepAlive(hold)
	hold = nil

	debug.FreeOSMemory()
	after, _ := readRSSBytes()

	t.Logf("rss base=%d MiB peak=%d MiB after FreeOSMemory=%d MiB",
		base>>20, peak>>20, after>>20)
	if after > base+(32<<20) {
		t.Fatalf("RSS did not return toward baseline after FreeOSMemory: base=%d MiB after=%d MiB",
			base>>20, after>>20)
	}
}

// TestGOMEMLIMITBoundsHeap validates the operational GC lever: with a soft
// memory limit and a lower GOGC, a churn storm cannot let the live heap drift
// far past the limit. This is the knob a multi-instance deployment should set.
func TestGOMEMLIMITBoundsHeap(t *testing.T) {
	if testing.Short() {
		t.Skip("allocates ~400 MiB transiently")
	}
	// Retain a realistic baseline of live data.
	hold := make([]byte, 32<<20)
	for i := 0; i < len(hold); i += 4096 {
		hold[i] = 1
	}
	runtime.KeepAlive(hold)

	const limit = int64(64 << 20)
	oldLimit := debug.SetMemoryLimit(limit)
	oldPct := debug.SetGCPercent(50)
	t.Cleanup(func() {
		debug.SetMemoryLimit(oldLimit)
		debug.SetGCPercent(oldPct)
	})

	var peak uint64
	for i := 0; i < 400; i++ {
		tmp := make([]byte, 1<<20)
		tmp[0] = byte(i)
		runtime.KeepAlive(tmp)
		if i%25 == 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if m.HeapAlloc > peak {
				peak = m.HeapAlloc
			}
		}
	}

	t.Logf("peak heap=%d MiB under GOMEMLIMIT=%d MiB (GOGC=50)", peak>>20, limit>>20)
	if peak > uint64(limit)+(64<<20) {
		t.Fatalf("heap drifted past the soft limit: peak=%d MiB limit=%d MiB",
			peak>>20, limit>>20)
	}
	runtime.KeepAlive(hold)
}
