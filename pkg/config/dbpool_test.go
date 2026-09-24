package config

import "testing"

// The DB pool size must be configurable (finding #7): the 25/5 default is a
// ceiling shared by every instance and the dashboard.
func TestParseDBPoolConfig(t *testing.T) {
	cases := []struct {
		name           string
		open, idle     string
		wantOpen, wantIdle int
	}{
		{"defaults", "", "", 25, 5},
		{"override", "80", "20", 80, 20},
		{"open zero rejected", "0", "", 25, 5},
		{"open negative rejected", "-1", "", 25, 5},
		{"open non-numeric rejected", "lots", "", 25, 5},
		{"idle negative rejected", "", "-3", 25, 5},
		{"idle zero allowed", "", "0", 25, 0},
		{"idle clamped to open", "10", "50", 10, 10},
		{"open set idle defaulted", "40", "", 40, 5},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			open, idle := parseDBPoolConfig(tc.open, tc.idle)
			if open != tc.wantOpen || idle != tc.wantIdle {
				t.Fatalf("parseDBPoolConfig(%q,%q) = %d,%d; want %d,%d",
					tc.open, tc.idle, open, idle, tc.wantOpen, tc.wantIdle)
			}
		})
	}
}
