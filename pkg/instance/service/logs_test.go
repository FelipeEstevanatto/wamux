package instance_service

import (
	"testing"
	"time"
)

// The instance id becomes a path segment under the log directory, so anything
// that is not a UUID must be rejected before the filesystem is touched.
func TestGetLogsRejectsNonUUID(t *testing.T) {
	svc := instances{}

	for _, id := range []string{"../../etc", "..%2F..%2Ftmp", "not-a-uuid", ""} {
		if _, err := svc.GetLogs(id, time.Time{}, time.Time{}, "", 0); err == nil {
			t.Fatalf("GetLogs(%q) should return an error", id)
		}
	}
}

func TestClampLogLimit(t *testing.T) {
	cases := []struct{ in, want int }{
		{-5, 100},
		{0, 100},
		{10, 10},
		{1000, 1000},
		{1 << 20, 1000},
	}
	for _, tc := range cases {
		if got := clampLogLimit(tc.in); got != tc.want {
			t.Errorf("clampLogLimit(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
