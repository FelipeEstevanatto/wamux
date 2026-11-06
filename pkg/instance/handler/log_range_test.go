package instance_handler

import (
	"testing"
	"time"
)

// At 22:40 in Sao Paulo it is already the next day in UTC. Reading the local date as
// a UTC day ended the range at 20:59 local, hiding the rest of the evening's logs.
func TestLogDateRangeDefaultsIncludeNowInEveryTimezone(t *testing.T) {
	for name, offset := range map[string]int{"west": -3, "utc": 0, "east": 9} {
		t.Run(name, func(t *testing.T) {
			zone := time.FixedZone(name, offset*3600)
			for _, now := range []time.Time{
				time.Date(2026, 11, 5, 22, 40, 0, 0, zone),
				time.Date(2026, 11, 5, 1, 10, 0, 0, zone),
				time.Date(2026, 12, 31, 23, 59, 0, 0, zone),
			} {
				start, end := logDateRange("", "", now)

				if now.After(end) {
					t.Errorf("%s: range ends before now, at %s", now, end)
				}
				if got := now.Sub(start); got < 7*24*time.Hour-time.Minute {
					t.Errorf("%s: range starts only %s ago", now, got)
				}
			}
		})
	}
}

func TestLogDateRangeUsesTheRequestedDays(t *testing.T) {
	start, end := logDateRange("2026-01-02", "2026-01-03", time.Now())

	if want := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC); !start.Equal(want) {
		t.Errorf("start = %s", start)
	}
	if want := time.Date(2026, 1, 3, 23, 59, 59, 999999999, time.UTC); !end.Equal(want) {
		t.Errorf("end = %s", end)
	}
}

func TestLogDateRangeIgnoresUnreadableDates(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	start, end := logDateRange("yesterday", "soon", now)

	if !start.Equal(now.AddDate(0, 0, -7)) || end.Day() != 5 {
		t.Fatalf("start %s, end %s", start, end)
	}
}
