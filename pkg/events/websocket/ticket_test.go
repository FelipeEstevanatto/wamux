package websocket_producer

import (
	"strings"
	"testing"
	"time"
)

func TestTicketWorksUntilItExpires(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	ticket, err := IssueTicket("key", now)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		at   time.Time
		want bool
	}{
		{"just issued", now, true},
		{"last moment", now.Add(TicketTTL - time.Second), true},
		{"expired", now.Add(TicketTTL), false},
		{"long expired", now.Add(time.Hour), false},
	} {
		if got := ValidTicket("key", ticket, tc.at); got != tc.want {
			t.Errorf("%s: ValidTicket = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTicketRejectsForgeries(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	ticket, err := IssueTicket("key", now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(ticket, ".")
	later := time.Unix(9_999_999_999, 0)

	for name, forged := range map[string]string{
		"empty":             "",
		"no dots":           "abc",
		"unsigned":          parts[0] + "." + parts[1],
		"wrong signature":   parts[0] + "." + parts[1] + "." + strings.Repeat("0", 64),
		"extended expiry":   "9999999999." + parts[1] + "." + parts[2],
		"other nonce":       parts[0] + ".00." + parts[2],
		"signature only":    parts[2],
		"not a number":      "soon." + parts[1] + "." + signTicket("key", "soon."+parts[1]),
		"missing the nonce": "9999999999." + signTicket("key", "9999999999"),
	} {
		if ValidTicket("key", forged, now) {
			t.Errorf("%s: accepted %q", name, forged)
		}
	}

	if ValidTicket("other key", ticket, now) {
		t.Error("accepted a ticket signed with another key")
	}
	if ValidTicket("key", ticket, later) {
		t.Error("accepted an expired ticket")
	}
}

func TestTicketsDiffer(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	a, _ := IssueTicket("key", now)
	b, _ := IssueTicket("key", now)
	if a == b {
		t.Fatal("two tickets are identical")
	}
}

// With no key configured every signature would be computable by anyone.
func TestTicketNeedsAKey(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	if _, err := IssueTicket("", now); err == nil {
		t.Fatal("IssueTicket with no key: want an error")
	}
	body := "9999999999.00"
	if ValidTicket("", body+"."+signTicket("", body), now) {
		t.Fatal("ValidTicket with no key: want false")
	}
}
