package whatsmeow_service

import (
	"testing"
	"time"
)

func TestHealthVerdicts(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	t.Run("ok", func(t *testing.T) {
		h := newInstanceHealthTracker(now)
		h.markOpened(now)
		got := h.snapshot("i", now.Add(time.Second), true, false, false, "")
		if got.Verdict != "ok" || len(got.Reasons) != 0 {
			t.Fatalf("health = %+v", got)
		}
		if got.ReconnectOpens != 1 || got.OpenedAt == "" {
			t.Fatalf("health = %+v", got)
		}
	})

	t.Run("reconnecting", func(t *testing.T) {
		h := newInstanceHealthTracker(now)
		h.markOpened(now)
		h.markDisconnected(now.Add(time.Second), "network")
		got := h.snapshot("i", now.Add(2*time.Second), false, true, false, "")
		if got.Verdict != "reconnecting" {
			t.Fatalf("verdict = %q, want reconnecting", got.Verdict)
		}
		if got.LastDisconnectReason != "network" {
			t.Fatalf("reason = %q", got.LastDisconnectReason)
		}
	})

	t.Run("looping", func(t *testing.T) {
		h := newInstanceHealthTracker(now)
		for i := 0; i < healthLoopDrops; i++ {
			at := now.Add(time.Duration(i) * time.Minute)
			h.markOpened(at)
			h.markDisconnected(at.Add(time.Second), "440")
		}
		got := h.snapshot("i", now.Add(3*time.Minute), false, true, false, "")
		if got.Verdict != "looping" {
			t.Fatalf("verdict = %q, want looping (%+v)", got.Verdict, got)
		}
	})

	t.Run("stalled websocket", func(t *testing.T) {
		h := newInstanceHealthTracker(now)
		h.markOpened(now)
		got := h.snapshot("i", now.Add(time.Minute), false, false, false, "")
		if got.Verdict != "stalled" {
			t.Fatalf("verdict = %q, want stalled (%+v)", got.Verdict, got)
		}
		found := false
		for _, reason := range got.Reasons {
			if reason == "ws_closed" {
				found = true
			}
		}
		if !found {
			t.Fatalf("reasons = %v, want ws_closed", got.Reasons)
		}
	})

	t.Run("stuck event", func(t *testing.T) {
		h := newInstanceHealthTracker(now)
		h.markOpened(now)
		h.markEventStart(now, "*events.Message")
		got := h.snapshot("i", now.Add(healthEventStall+time.Second), true, false, false, "")
		if got.Verdict != "stalled" || got.EventRunningAgeMs == 0 {
			t.Fatalf("health = %+v", got)
		}
	})

	t.Run("blocked is closed", func(t *testing.T) {
		h := newInstanceHealthTracker(now)
		got := h.snapshot("i", now, false, true, true, "session replaced")
		if got.Verdict != "closed" || got.BlockedReason != "session replaced" {
			t.Fatalf("health = %+v", got)
		}
	})

	t.Run("closed", func(t *testing.T) {
		h := newInstanceHealthTracker(now)
		got := h.snapshot("i", now, false, false, false, "")
		if got.Verdict != "closed" {
			t.Fatalf("verdict = %q", got.Verdict)
		}
	})

	t.Run("missing tracker", func(t *testing.T) {
		var h *instanceHealthTracker
		got := h.snapshot("i", now, false, false, false, "")
		if got.Verdict != "closed" || len(got.Reasons) != 1 || got.Reasons[0] != "no_health_data" {
			t.Fatalf("health = %+v", got)
		}
	})
}

func TestHealthWindowPrunesOldCounts(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	h := newInstanceHealthTracker(now)
	h.markDisconnected(now.Add(-healthWindow-time.Minute), "old")
	for i := 0; i < healthLoopDrops; i++ {
		h.markDisconnected(now.Add(-time.Minute), "recent")
	}
	got := h.snapshot("i", now, false, false, false, "")
	if got.ReconnectDrops != healthLoopDrops {
		t.Fatalf("drops = %d, want %d", got.ReconnectDrops, healthLoopDrops)
	}
}

// BenchmarkHealthSnapshot measures the read path used by /instance/status,
// /instance/inventory and every /metrics scrape.
func BenchmarkHealthSnapshot(b *testing.B) {
	now := time.Unix(1_700_000_000, 0)
	h := newInstanceHealthTracker(now)
	for i := 0; i < 100; i++ {
		h.markOpened(now)
		h.markDisconnected(now, "440")
	}
	h.markInbound(now)
	h.markAck(now)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.snapshot("bench", now, false, true, false, "")
	}
}
