package procstats

import "testing"

func TestReadReturnsGoRuntimeData(t *testing.T) {
	s := Read()

	// These come from the Go runtime and are always present.
	if s.NumGoroutines < 1 {
		t.Fatalf("goroutines = %d, want >= 1", s.NumGoroutines)
	}
	if s.HeapAllocBytes == 0 {
		t.Fatalf("heap alloc = 0, expected a non-zero heap")
	}
}

// On Linux, /proc/self must yield an RSS for the running process.
func TestReadReturnsProcessRSSOnLinux(t *testing.T) {
	s := Read()
	if s.RSSBytes == 0 {
		t.Skip("RSS unavailable (non-Linux or restricted /proc)")
	}
	if s.VMSizeBytes < s.RSSBytes {
		t.Fatalf("virtual size %d < RSS %d; parsing looks wrong", s.VMSizeBytes, s.RSSBytes)
	}
	if s.CPUSeconds < 0 {
		t.Fatalf("negative cpu seconds: %f", s.CPUSeconds)
	}
}
