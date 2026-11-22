package whatsmeow_service

import (
	"testing"
	"time"
)

func TestShouldEvictIdle(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	for _, tc := range []struct {
		name      string
		idle      time.Duration
		minutes   int
		connected bool
		pending   bool
		pairing   bool
		want      bool
	}{
		{"disabled", time.Hour, 0, false, false, false, false},
		{"still connected", time.Hour, 30, true, false, false, false},
		{"reconnect pending", time.Hour, 30, false, true, false, false},
		{"mid pairing", time.Hour, 30, false, false, true, false},
		{"not idle long enough", 29 * time.Minute, 30, false, false, false, false},
		{"idle", 30 * time.Minute, 30, false, false, false, true},
		{"idle longer", 3 * time.Hour, 30, false, false, false, true},
		{"no disconnect timestamp", 0, 30, false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			since := now.Add(-tc.idle)
			if tc.idle == 0 {
				since = time.Time{}
			}
			if got := shouldEvictIdle(now, since, tc.minutes, tc.connected, tc.pending, tc.pairing); got != tc.want {
				t.Fatalf("shouldEvictIdle = %v, want %v", got, tc.want)
			}
		})
	}
}
