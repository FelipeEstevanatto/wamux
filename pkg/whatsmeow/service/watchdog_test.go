package whatsmeow_service

import (
	"testing"
	"time"
)

func TestWatchdogReportNeedsConfirmationAndReportsOnce(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	p := newWatchdogPolicy("report", 5, func() time.Time { return now })

	if d := p.decide("i", "open", false, []string{"ws_closed"}); d.Action != "none" {
		t.Fatalf("first check = %+v", d)
	}
	now = now.Add(watchdogConfirm - time.Second)
	if d := p.decide("i", "open", false, []string{"ws_closed"}); d.Action != "none" {
		t.Fatalf("before confirmation = %+v", d)
	}
	now = now.Add(time.Second)
	if d := p.decide("i", "open", false, []string{"ws_closed"}); d.Action != "report" || d.Reason != "ws_closed" {
		t.Fatalf("confirmed = %+v", d)
	}
	if d := p.decide("i", "open", false, []string{"ws_closed"}); d.Action != "none" {
		t.Fatalf("report repeated = %+v", d)
	}
}

func TestWatchdogIgnoresNonSocketReasons(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	p := newWatchdogPolicy("on", 5, func() time.Time { return now })
	for i := 0; i < 5; i++ {
		now = now.Add(watchdogConfirm)
		if d := p.decide("i", "open", false, []string{"sends_unacked", "event_stuck"}); d.Action != "none" {
			t.Fatalf("non-trigger caused %+v", d)
		}
	}
}

func TestWatchdogRebuildsCapsAndRecovers(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	p := newWatchdogPolicy("on", 2, func() time.Time { return now })

	rebuild := func() bool {
		now = now.Add(watchdogConfirm)
		return p.decide("i", "open", false, []string{"ws_closed"}).Action == "rebuild"
	}
	start := func() { p.decide("i", "open", false, []string{"ws_closed"}) }

	start()
	if !rebuild() {
		t.Fatal("first rebuild did not happen")
	}
	// The backoff starts at 5s; advance past it before the next confirmation.
	now = now.Add(watchdogBackoff[0] + time.Second)
	start()
	if !rebuild() {
		t.Fatal("second rebuild did not happen")
	}
	now = now.Add(watchdogBackoff[1] + time.Second)
	start()
	now = now.Add(watchdogConfirm)
	if d := p.decide("i", "open", false, []string{"ws_closed"}); d.Action != "give-up" {
		t.Fatalf("cap decision = %+v, want give-up", d)
	}
	if d := p.decide("i", "open", false, []string{"ws_closed"}); d.Action != "none" {
		t.Fatalf("gave-up repeated = %+v", d)
	}

	// A healthy socket for 60s clears the cap, and a new episode may rebuild.
	now = now.Add(time.Second)
	if d := p.decide("i", "open", false, nil); d.Action != "none" {
		t.Fatalf("healthy = %+v", d)
	}
	now = now.Add(watchdogHealthy)
	p.decide("i", "open", false, nil)
	// Start a new trigger window, then hold it through the confirmation.
	p.decide("i", "open", false, []string{"ws_closed"})
	now = now.Add(watchdogConfirm)
	if d := p.decide("i", "open", false, []string{"ws_closed"}); d.Action != "rebuild" {
		t.Fatalf("after recovery = %+v", d)
	}
}

func TestWatchdogDoesNotActWhileBusyOrClosed(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	p := newWatchdogPolicy("on", 5, func() time.Time { return now })

	now = now.Add(watchdogConfirm)
	if d := p.decide("i", "open", true, []string{"ws_closed"}); d.Action != "none" {
		t.Fatalf("busy = %+v", d)
	}
	now = now.Add(watchdogConfirm)
	if d := p.decide("i", "close", false, []string{"ws_closed"}); d.Action != "none" {
		t.Fatalf("closed = %+v", d)
	}
}

func TestWatchdogOffDoesNothing(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	p := newWatchdogPolicy("off", 5, func() time.Time { return now })
	now = now.Add(time.Hour)
	if d := p.decide("i", "open", false, []string{"ws_closed"}); d.Action != "none" {
		t.Fatalf("off = %+v", d)
	}
}
