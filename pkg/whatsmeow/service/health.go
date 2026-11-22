package whatsmeow_service

import (
	"sync"
	"time"
)

// Health windows and thresholds. The 10-minute window is what the Reforged
// comparison used and is long enough to catch a 440 loop without reporting every
// ordinary network blip.
const (
	healthWindow        = 10 * time.Minute
	healthLoopDrops     = 3
	healthLoopConflicts = 2
	// A single event handler running longer than this is a stall, not a slow
	// message: media downloads run on their own pool and are bounded separately.
	healthEventStall = 2 * time.Minute
)

// instanceHealthTracker is the in-memory facts behind one instance's health. It
// never reads the database or Redis, and its snapshot carries times/counts only:
// no JIDs, message ids or content.
type instanceHealthTracker struct {
	mu            sync.Mutex
	observedSince time.Time

	openedAt      time.Time
	connected     bool
	lastFrame     time.Time
	lastEvent     time.Time
	lastEventType string
	eventStart    time.Time
	lastInbound   time.Time
	lastAck       time.Time

	lastDisconnectAt      time.Time
	lastDisconnectReason  string
	lastKeepAliveTimeout  time.Time
	lastKeepAliveRestored time.Time

	// Timestamps in the current window, pruned on read.
	opens     []time.Time
	drops     []time.Time
	conflicts []time.Time
}

func newInstanceHealthTracker(now time.Time) *instanceHealthTracker {
	return &instanceHealthTracker{observedSince: now}
}

func (h *instanceHealthTracker) markOpened(now time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.openedAt = now
	h.connected = true
	h.opens = append(h.opens, now)
	h.mu.Unlock()
}

func (h *instanceHealthTracker) markDisconnected(now time.Time, reason string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.connected = false
	h.lastDisconnectAt = now
	if reason != "" {
		h.lastDisconnectReason = reason
	}
	h.drops = append(h.drops, now)
	h.mu.Unlock()
}

func (h *instanceHealthTracker) markConflict(now time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.conflicts = append(h.conflicts, now)
	h.mu.Unlock()
}

func (h *instanceHealthTracker) markEventStart(now time.Time, name string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.eventStart = now
	h.lastEvent = now
	h.lastFrame = now
	h.lastEventType = name
	h.mu.Unlock()
}

func (h *instanceHealthTracker) markEventEnd() {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.eventStart = time.Time{}
	h.mu.Unlock()
}

func (h *instanceHealthTracker) markInbound(now time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.lastInbound = now
	h.mu.Unlock()
}

func (h *instanceHealthTracker) markAck(now time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.lastAck = now
	h.mu.Unlock()
}

func (h *instanceHealthTracker) markKeepAliveTimeout(now time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.lastKeepAliveTimeout = now
	h.mu.Unlock()
}

func (h *instanceHealthTracker) markKeepAliveRestored(now time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.lastKeepAliveRestored = now
	h.mu.Unlock()
}

func inWindow(times []time.Time, now time.Time) []time.Time {
	cutoff := now.Add(-healthWindow)
	out := times[:0]
	for _, t := range times {
		if t.After(cutoff) {
			out = append(out, t)
		}
	}
	return out
}

// snapshot builds the public view. liveConnected is the socket's own answer;
// storedConnected is what the tracker last observed. A disagreement is a stall.
func (h *instanceHealthTracker) snapshot(instanceID string, now time.Time, liveConnected, reconnecting, blocked bool, blockedReason string) InstanceHealth {
	out := InstanceHealth{
		InstanceID:    instanceID,
		Connected:     liveConnected,
		Reconnecting:  reconnecting,
		ObservedSince: h.observedSince.Format(time.RFC3339),
	}
	if h == nil {
		out.Verdict = "closed"
		out.Reasons = []string{"no_health_data"}
		return out
	}

	h.mu.Lock()
	opens := inWindow(append([]time.Time(nil), h.opens...), now)
	drops := inWindow(append([]time.Time(nil), h.drops...), now)
	conflicts := inWindow(append([]time.Time(nil), h.conflicts...), now)
	out.OpenedAt = formatTime(h.openedAt)
	out.LastFrameAt = formatTime(h.lastFrame)
	out.LastEventAt = formatTime(h.lastEvent)
	out.LastInboundAt = formatTime(h.lastInbound)
	out.LastAckAt = formatTime(h.lastAck)
	out.LastDisconnectAt = formatTime(h.lastDisconnectAt)
	out.LastDisconnectReason = h.lastDisconnectReason
	out.LastKeepAliveTimeoutAt = formatTime(h.lastKeepAliveTimeout)
	out.LastKeepAliveRestoredAt = formatTime(h.lastKeepAliveRestored)
	out.EventType = h.lastEventType
	if !h.eventStart.IsZero() {
		out.EventRunningAgeMs = now.Sub(h.eventStart).Milliseconds()
	}
	out.ReconnectOpens = len(opens)
	out.ReconnectDrops = len(drops)
	out.Conflicts = len(conflicts)
	storedConnected := h.connected
	eventStart := h.eventStart
	h.mu.Unlock()

	if storedConnected && !liveConnected {
		out.Reasons = append(out.Reasons, "ws_closed")
		if out.Verdict == "" {
			out.Verdict = "stalled"
		}
	}

	if blocked {
		out.BlockedReason = blockedReason
		out.Verdict = "closed"
		out.Reasons = append(out.Reasons, "reconnect_suppressed")
	}
	if len(drops) >= healthLoopDrops {
		out.Reasons = append(out.Reasons, "reconnect_loop")
	}
	if len(conflicts) >= healthLoopConflicts {
		out.Reasons = append(out.Reasons, "session_conflict")
	}
	if !liveConnected && out.Verdict == "" {
		if reconnecting {
			out.Verdict = "reconnecting"
		}
	}
	if liveConnected && !eventStart.IsZero() && now.Sub(eventStart) >= healthEventStall {
		out.Reasons = append(out.Reasons, "event_stuck")
	}
	if out.Verdict == "" {
		switch {
		case len(drops) >= healthLoopDrops || len(conflicts) >= healthLoopConflicts:
			out.Verdict = "looping"
		case liveConnected && len(out.Reasons) > 0:
			out.Verdict = "stalled"
		case liveConnected:
			out.Verdict = "ok"
		case blocked:
			out.Verdict = "closed"
		default:
			out.Verdict = "closed"
		}
	}
	// A reconnect loop is the more useful diagnosis than "reconnecting".
	if !blocked && (len(drops) >= healthLoopDrops || len(conflicts) >= healthLoopConflicts) {
		out.Verdict = "looping"
	}
	if len(out.Reasons) == 0 {
		out.Reasons = []string{}
	}
	return out
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
