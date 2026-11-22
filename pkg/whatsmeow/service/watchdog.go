package whatsmeow_service

import (
	"context"
	"sync"
	"time"
)

// Watchdog thresholds. The watchdog only acts while the stored state reads
// "open"; a socket that is connecting or already being restarted is left alone.
const (
	WatchdogCheckInterval = 10 * time.Second
	watchdogConfirm       = 30 * time.Second
	watchdogHealthy       = 60 * time.Second
	watchdogKeepAliveAge  = 3 * time.Minute
)

var watchdogTriggers = []string{"ws_closed", "keepalive_silent"}

// Watchdog rebuilds back off faster than the disconnect loop: the socket was
// already "open", so the first retry can be quick.
var watchdogBackoff = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute, 5 * time.Minute}

type watchdogDecision struct {
	Action string // none, report, rebuild, give-up
	Reason string
	Count  int
}

type watchdogState struct {
	since        map[string]time.Time
	healthySince time.Time
	consecutive  int
	rebuilds     int
	lastRebuild  time.Time
	lastReason   string
	gaveUp       bool
	reported     bool
}

// watchdogPolicy is the testable half of the watchdog. It decides; the service
// performs the rebuild. It has no dependency on whatsmeow.
type watchdogPolicy struct {
	mode        string
	maxRebuilds int
	now         func() time.Time
	mu          sync.Mutex
	states      map[string]*watchdogState
}

func newWatchdogPolicy(mode string, maxRebuilds int, now func() time.Time) *watchdogPolicy {
	if now == nil {
		now = time.Now
	}
	if maxRebuilds < 1 {
		maxRebuilds = 5
	}
	return &watchdogPolicy{mode: mode, maxRebuilds: maxRebuilds, now: now, states: map[string]*watchdogState{}}
}

func (p *watchdogPolicy) decide(instanceID string, state string, busy bool, reasons []string) watchdogDecision {
	if p == nil || p.mode == "off" {
		return watchdogDecision{Action: "none"}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	st := p.states[instanceID]
	if st == nil {
		st = &watchdogState{since: map[string]time.Time{}}
		p.states[instanceID] = st
	}

	if busy || state != "open" {
		st.since = map[string]time.Time{}
		st.healthySince = time.Time{}
		return watchdogDecision{Action: "none"}
	}

	active := make(map[string]bool, len(reasons))
	for _, reason := range reasons {
		active[reason] = true
	}
	for _, trigger := range watchdogTriggers {
		if active[trigger] {
			if _, ok := st.since[trigger]; !ok {
				st.since[trigger] = now
			}
		} else {
			delete(st.since, trigger)
		}
	}
	if len(st.since) == 0 {
		st.reported = false
		if st.healthySince.IsZero() {
			st.healthySince = now
		}
		if now.Sub(st.healthySince) >= watchdogHealthy {
			st.consecutive = 0
			st.gaveUp = false
		}
		return watchdogDecision{Action: "none"}
	}
	st.healthySince = time.Time{}

	reason := ""
	for _, trigger := range watchdogTriggers {
		if since, ok := st.since[trigger]; ok && now.Sub(since) >= watchdogConfirm {
			reason = trigger
			break
		}
	}
	if reason == "" {
		return watchdogDecision{Action: "none"}
	}

	if p.mode == "report" {
		if st.reported {
			return watchdogDecision{Action: "none"}
		}
		st.reported = true
		st.lastReason = reason
		return watchdogDecision{Action: "report", Reason: reason, Count: st.consecutive}
	}

	if st.gaveUp {
		return watchdogDecision{Action: "none"}
	}
	if st.consecutive >= p.maxRebuilds {
		st.gaveUp = true
		return watchdogDecision{Action: "give-up", Reason: reason, Count: st.consecutive}
	}
	if !st.lastRebuild.IsZero() && now.Sub(st.lastRebuild) < watchdogBackoff[min(st.consecutive, len(watchdogBackoff)-1)] {
		return watchdogDecision{Action: "none"}
	}

	st.consecutive++
	st.rebuilds++
	st.lastRebuild = now
	st.lastReason = reason
	st.since = map[string]time.Time{}
	return watchdogDecision{Action: "rebuild", Reason: reason, Count: st.consecutive}
}

func (p *watchdogPolicy) rebuildFinished() {}

// StartWatchdog runs the check loop until ctx is cancelled. Report mode logs
// what it would rebuild; on mode calls ReconnectClient for that one instance.
// A stuck event queue is never a trigger: the queue belongs to the instance, not
// the socket, and a rebuild would not reach it.
func (w whatsmeowService) StartWatchdog(ctx context.Context) {
	if w.config == nil || w.config.WatchdogMode == "" || w.config.WatchdogMode == "off" {
		return
	}
	policy := newWatchdogPolicy(w.config.WatchdogMode, w.config.WatchdogMaxRebuilds, time.Now)
	ticker := time.NewTicker(WatchdogCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for id, mycli := range w.myClientPointer.Snapshot() {
				if mycli == nil || mycli.WAClient == nil {
					continue
				}
				state := "closed"
				if mycli.Instance != nil && mycli.Instance.Connected {
					state = "open"
				}
				health := w.InstanceHealth(id)
				reasons := []string{}
				if health != nil {
					reasons = health.Reasons
				}
				decision := policy.decide(id, state, reconnectPending(id) || reconnectBlockedReason(id) != "", reasons)
				switch decision.Action {
				case "report":
					w.loggerWrapper.GetLogger(id).LogWarn("[%s] watchdog would rebuild the socket (%s); WATCHDOG_MODE=report, no action", id, decision.Reason)
				case "rebuild":
					w.loggerWrapper.GetLogger(id).LogWarn("[%s] watchdog rebuilding the socket (%s, attempt %d/%d)", id, decision.Reason, decision.Count, w.config.WatchdogMaxRebuilds)
					go func(instanceID string) {
						defer policy.rebuildFinished()
						if err := w.ReconnectClient(instanceID); err != nil {
							w.loggerWrapper.GetLogger(instanceID).LogError("[%s] watchdog rebuild failed: %v", instanceID, err)
						}
					}(id)
				case "give-up":
					w.loggerWrapper.GetLogger(id).LogError("[%s] watchdog gave up after %d rebuilds without a healthy socket (%s)", id, decision.Count, decision.Reason)
				}
			}
		}
	}
}
