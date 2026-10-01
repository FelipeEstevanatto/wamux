// Package procstats reports THIS process's own resource usage, so the numbers
// stay meaningful on a shared machine.
//
// # WHY
//
// The dashboard's "system" panel read /proc/loadavg and /proc/meminfo. Inside a
// container those files show the HOST's values, not the container's — so if
// anything else runs on the box the readouts are useless, and they never say how
// much the API itself is using. What an operator actually asks is "how much RAM
// is THIS service using, and is it growing?".
//
// This package answers that from per-process sources:
//
//   - /proc/self/statm and /proc/self/status  -> RSS (resident), VmHWM (peak),
//     virtual size. Linux only; absent elsewhere.
//   - cgroup v2 (/sys/fs/cgroup/memory.current, memory.max, cpu.max) -> the
//     container's own memory usage and limit, when running under a cgroup.
//
// Everything is best-effort: a missing file simply yields an absent field. It
// reads are cheap enough to call on every scrape.
package procstats

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Snapshot is one reading of this process's resource usage.
type Snapshot struct {
	// Process (from /proc/self); zero when unavailable (non-Linux).
	RSSBytes     uint64 `json:"rssBytes"`
	PeakRSSBytes uint64 `json:"peakRssBytes"`
	VMSizeBytes  uint64 `json:"vmSizeBytes"`

	// Go runtime heap (always available).
	HeapAllocBytes uint64 `json:"heapAllocBytes"`
	HeapInuseBytes uint64 `json:"heapInuseBytes"`
	SysBytes       uint64 `json:"sysBytes"`
	NumGoroutines  int    `json:"numGoroutines"`
	NumGC          uint32 `json:"numGC"`
	GCPauseTotalNs uint64 `json:"gcPauseTotalNs"`

	// cgroup v2 (container limits); zero when not containerised/limited.
	CgroupMemoryBytes uint64 `json:"cgroupMemoryBytes"`
	CgroupMemoryLimit uint64 `json:"cgroupMemoryLimitBytes"`
	CgroupCPUQuota    int    `json:"cgroupCpuQuotaMilli"` // e.g. 1500 == 1.5 cores

	// Actual CPU time this process has consumed, from /proc/self/stat.
	CPUSeconds float64 `json:"cpuSeconds"`

	// Containerized reports whether a cgroup limit was found.
	Containerized bool `json:"containerized"`
}

// Read samples everything available right now.
func Read() Snapshot {
	var s Snapshot

	// Go runtime heap — always present.
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.HeapAllocBytes = m.Alloc
	s.HeapInuseBytes = m.HeapInuse
	s.SysBytes = m.Sys
	s.NumGoroutines = runtime.NumGoroutine()
	s.NumGC = m.NumGC
	s.GCPauseTotalNs = m.PauseTotalNs

	// Process RSS/virtual from /proc/self/statm (Linux).
	if rss, peak, vsz, ok := readStatm(); ok {
		s.RSSBytes = rss
		s.PeakRSSBytes = peak
		s.VMSizeBytes = vsz
	}
	// VmHWM (peak RSS) is more reliable than statm's high-water field; prefer it.
	if hwm, rss, ok := readStatus(); ok {
		s.PeakRSSBytes = hwm
		if rss > 0 {
			s.RSSBytes = rss
		}
	}
	if cpu, ok := readProcStatCPU(); ok {
		s.CPUSeconds = cpu
	}

	// cgroup v2 container limits.
	if used, ok := readUintFile("/sys/fs/cgroup/memory.current"); ok {
		s.CgroupMemoryBytes = used
	}
	if max, ok := readUintFile("/sys/fs/cgroup/memory.max"); ok {
		s.CgroupMemoryLimit = max
		s.Containerized = true
	}
	if quot, ok := readCPUMax("/sys/fs/cgroup/cpu.max"); ok {
		s.CgroupCPUQuota = quot
		s.Containerized = true
	}

	return s
}

var pageSize = uint64(os.Getpagesize())

// readStatm returns resident, peak(high-water approx) and virtual bytes.
// /proc/self/statm fields (pages): size resident shared text lib data dt
func readStatm() (rss, peak, vsz uint64, ok bool) {
	b, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, 0, 0, false
	}
	f := strings.Fields(string(b))
	if len(f) < 2 {
		return 0, 0, 0, false
	}
	size, err1 := strconv.ParseUint(f[0], 10, 64)
	r, err2 := strconv.ParseUint(f[1], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, 0, false
	}
	// statm has no high-water mark; leave peak to readStatus.
	return r * pageSize, 0, size * pageSize, true
}

// readStatus returns VmHWM (peak RSS) and VmRSS, in bytes.
func readStatus() (hwm, rss uint64, ok bool) {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		switch {
		case strings.HasPrefix(line, "VmHWM:"):
			hwm = parseKBLine(line)
		case strings.HasPrefix(line, "VmRSS:"):
			rss = parseKBLine(line)
		}
	}
	return hwm, rss, hwm > 0 || rss > 0
}

func parseKBLine(line string) uint64 {
	f := strings.Fields(line)
	if len(f) < 2 {
		return 0
	}
	kb, err := strconv.ParseUint(f[1], 10, 64)
	if err != nil {
		return 0
	}
	return kb * 1024
}

// readProcStatCPU returns total CPU seconds (user+system) this process consumed.
func readProcStatCPU() (float64, bool) {
	b, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, false
	}
	// Field 14 (utime) and 15 (stime), 1-indexed, in clock ticks. The comm field
	// (2) is parenthesised and may contain spaces, so split after the last ')'.
	s := string(b)
	close := strings.LastIndex(s, ")")
	if close < 0 {
		return 0, false
	}
	fields := strings.Fields(s[close+1:])
	// After ')', field index 0 == statm field 3, so utime is [11], stime [12].
	if len(fields) < 13 {
		return 0, false
	}
	utime, err1 := strconv.ParseUint(fields[11], 10, 64)
	stime, err2 := strconv.ParseUint(fields[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	// sysconf(_SC_CLK_TCK) is 100 on virtually every Linux; use that.
	return float64(utime+stime) / 100.0, true
}

func readUintFile(path string) (uint64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v := strings.TrimSpace(string(b))
	if v == "max" {
		return 0, false // no limit set
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// readCPUMax parses cgroup v2 "cpu.max" ("<quota> <period>") into milli-cores.
func readCPUMax(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	f := strings.Fields(string(b))
	if len(f) != 2 || f[0] == "max" {
		return 0, false
	}
	quota, err1 := strconv.Atoi(f[0])
	period, err2 := strconv.Atoi(f[1])
	if err1 != nil || err2 != nil || period == 0 {
		return 0, false
	}
	// quota/period cores, expressed in milli-cores.
	return quota * 1000 / period, true
}
