package whatsmeow_service

import (
	"sync"
	"time"
)

// ============================================================================
// Backoff and close policy for the reconnect loop.
//
// THE LOOP: an instance whose device was logged out from the phone spins
// forever. Disconnected -> ReconnectClient -> the instance comes up with no
// session -> emits a QR -> nobody scans -> max QR count -> forced logout ->
// Disconnected again. Measured in production: 110 reconnects and 1135 QR codes
// in 50 minutes from a single instance, and it only stopped when a human looked.
//
// THE SHAPE: the first reconnectFreeAttempts restarts inside the window go
// straight through — that is the good case, a healthy instance that lost its
// websocket and must come back within seconds. Past that the loop turns into a
// growing wait, and the instance keeps trying: this is a backoff, not a
// give-up. An instance with a valid session still heals on its own; what is
// lost is the hammering.
//
// STABLE RESET: the counter must only be cleared after the socket has stayed
// open for stableConnectionWindow. A 440 (replaced) loop briefly reaches
// "Connected" on every attempt; clearing on any Connected event reset the
// counter every turn and defeated the backoff entirely.
//
// NOTE: only one goroutine waits per instance (the scheduled flag). Without it
// every Disconnected arriving during the wait would stack another one, and the
// backoff would become the very loop it was written to stop.
// ============================================================================

const (
	reconnectWindow        = 15 * time.Minute // restarts inside this window count together
	reconnectFreeAttempts  = 5                // how many go through with no wait
	stableConnectionWindow = 60 * time.Second
)

// The ladder of waits. The last step repeats indefinitely.
var reconnectBackoff = []time.Duration{5 * time.Minute, 15 * time.Minute, 30 * time.Minute}

// reconnectAllowed results.
const (
	// reconnectBlockedResult is returned when the loop must stop and wait for a
	// manual connect/restart. Keep the session: a manual connect may succeed.
	reconnectBlockedResult = time.Duration(-2)
)

type reconnectState struct {
	restarts  int
	since     time.Time
	step      int
	scheduled bool
	// connectedAt is when the current socket reached Connected. It is cleared
	// when a disconnect is evaluated, so a drop after a stable period can reset
	// the counter while a quick 440 flap cannot.
	connectedAt time.Time
	// conflicts counts consecutive replaced (440) closes. Any other close clears
	// it; the session is kept when the cap is reached.
	conflicts int
	// blocked stops automatic reconnects until a manual connect/restart. The
	// reason is persisted separately as the instance's disconnect reason.
	blocked       bool
	blockedReason string
}

var (
	reconnectMu    sync.Mutex
	reconnectTrack = map[string]*reconnectState{}
)

// reconnectAllowed reports whether this instance may restart right now.
//
//	(true, 0)   — go ahead, normal path
//	(false, d)  — wait d and then try; this goroutine owns the wait
//	(false, -1) — another goroutine is already waiting for this instance; give up
//	(false, -2) — automatic reconnect is blocked; wait for a manual connect
func reconnectAllowed(instanceID string) (bool, time.Duration) {
	return reconnectAllowedAt(instanceID, time.Now())
}

// reconnectAllowedAt is reconnectAllowed with an injectable clock for tests.
func reconnectAllowedAt(instanceID string, now time.Time) (bool, time.Duration) {
	reconnectMu.Lock()
	defer reconnectMu.Unlock()

	st := reconnectTrack[instanceID]
	if st == nil {
		reconnectTrack[instanceID] = &reconnectState{restarts: 1, since: now}
		return true, 0
	}
	if st.blocked {
		return false, reconnectBlockedResult
	}

	// A socket that stayed open long enough ended the previous episode. Start a
	// fresh budget for this drop; a brief open in a 440 loop does not qualify.
	if !st.connectedAt.IsZero() {
		stable := now.Sub(st.connectedAt) >= stableConnectionWindow
		st.connectedAt = time.Time{}
		if stable {
			reconnectTrack[instanceID] = &reconnectState{restarts: 1, since: now}
			return true, 0
		}
	}

	if now.Sub(st.since) > reconnectWindow {
		reconnectTrack[instanceID] = &reconnectState{restarts: 1, since: now}
		return true, 0
	}
	if st.scheduled {
		return false, -1
	}
	st.restarts++
	if st.restarts <= reconnectFreeAttempts {
		return true, 0
	}

	d := reconnectBackoff[len(reconnectBackoff)-1]
	if st.step < len(reconnectBackoff) {
		d = reconnectBackoff[st.step]
		st.step++
	}
	st.scheduled = true
	return false, d
}

// reconnectWaitDone puts the instance back in line once its wait is over.
func reconnectWaitDone(instanceID string) {
	reconnectMu.Lock()
	if st := reconnectTrack[instanceID]; st != nil {
		st.scheduled = false
	}
	reconnectMu.Unlock()
}

// reconnectConnected records that the socket reached Connected. It deliberately
// does NOT clear the restart counter: a 440 loop reaches Connected on every
// attempt. reconnectAllowedAt clears it only once the connection has stayed
// open for stableConnectionWindow.
func reconnectConnected(instanceID string) {
	reconnectConnectedAt(instanceID, time.Now())
}

// reconnectConnectedAt is reconnectConnected with an injectable clock for tests.
func reconnectConnectedAt(instanceID string, now time.Time) {
	reconnectMu.Lock()
	st := reconnectTrack[instanceID]
	if st == nil {
		st = &reconnectState{since: now}
		reconnectTrack[instanceID] = st
	}
	st.connectedAt = now
	reconnectMu.Unlock()
}

// reconnectDisconnected clears the connected timestamp and ends a 440 streak.
// A non-440 close means the previous conflicts were not consecutive.
func reconnectDisconnected(instanceID string) {
	reconnectMu.Lock()
	if st := reconnectTrack[instanceID]; st != nil {
		st.connectedAt = time.Time{}
		st.conflicts = 0
	}
	reconnectMu.Unlock()
}

// reconnectRecordConflict counts one replaced (440) close and reports the new
// consecutive count and whether the configured cap has been reached. limit <= 0
// means unlimited.
func reconnectRecordConflict(instanceID string, limit int) (int, bool) {
	reconnectMu.Lock()
	defer reconnectMu.Unlock()

	st := reconnectTrack[instanceID]
	if st == nil {
		st = &reconnectState{restarts: 1, since: time.Now()}
		reconnectTrack[instanceID] = st
	}
	st.conflicts++
	st.connectedAt = time.Time{}
	return st.conflicts, limit > 0 && st.conflicts >= limit
}

// reconnectBlock stops automatic reconnects for an instance. It is used when
// retrying cannot succeed (logged out, banned, session replaced) and when the
// 440 cap is reached. The session is kept.
func reconnectBlock(instanceID, reason string) {
	reconnectMu.Lock()
	st := reconnectTrack[instanceID]
	if st == nil {
		st = &reconnectState{since: time.Now()}
		reconnectTrack[instanceID] = st
	}
	st.blocked = true
	st.blockedReason = reason
	st.scheduled = false
	st.connectedAt = time.Time{}
	reconnectMu.Unlock()
}

// reconnectBlockedReason returns the reason automatic reconnect is blocked.
func reconnectBlockedReason(instanceID string) string {
	reconnectMu.Lock()
	defer reconnectMu.Unlock()
	if st := reconnectTrack[instanceID]; st != nil {
		return st.blockedReason
	}
	return ""
}

// reconnectPending reports whether a reconnect is already scheduled/waiting for
// this instance. The watchdog uses it to avoid racing the normal backoff.
func reconnectPending(instanceID string) bool {
	reconnectMu.Lock()
	defer reconnectMu.Unlock()
	st := reconnectTrack[instanceID]
	return st != nil && (st.scheduled || st.blocked)
}

// reconnectSucceeded clears the state. It is the manual override used when a
// human explicitly asks for a reconnect; automatic recovery uses the stable
// window instead.
func reconnectSucceeded(instanceID string) {
	reconnectMu.Lock()
	delete(reconnectTrack, instanceID)
	reconnectMu.Unlock()
}

// ClearReconnectBackoff forgets an instance's reconnect history so the next
// reconnect gets a fresh, unthrottled budget. Used when a human explicitly asks
// for a reconnect (for example after changing the proxy or restoring a blocked
// session), which is exactly the signal that the automatic counter no longer
// describes the situation.
func ClearReconnectBackoff(instanceID string) {
	reconnectSucceeded(instanceID)
}
