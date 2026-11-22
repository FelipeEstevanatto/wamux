package whatsmeow_service

import (
	"testing"
	"time"
)

func resetReconnectTestState(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		reconnectSucceeded(id)
	}
	t.Cleanup(func() {
		for _, id := range ids {
			reconnectSucceeded(id)
		}
	})
}

// A quick drop (shorter than the stable window) must grow the restart count:
// a 440 loop opens briefly every time and must not reset the budget.
func TestReconnectQuickDropsAccumulate(t *testing.T) {
	const id = "reconnect-quick-drops"
	resetReconnectTestState(t, id)
	now := time.Unix(1_700_000_000, 0)

	allowed, wait := reconnectAllowedAt(id, now)
	if !allowed || wait != 0 {
		t.Fatalf("first reconnect = (%v, %v), want (true, 0)", allowed, wait)
	}

	// Five free attempts, then the ladder starts. Each connection lasts 1s.
	wantWaits := []time.Duration{reconnectBackoff[0], reconnectBackoff[1], reconnectBackoff[2]}
	for i := 0; i < reconnectFreeAttempts+2; i++ {
		reconnectConnectedAt(id, now)
		now = now.Add(time.Second)
		allowed, wait = reconnectAllowedAt(id, now)
		reconnectWaitDone(id)
		if i < reconnectFreeAttempts-1 {
			if !allowed || wait != 0 {
				t.Fatalf("attempt %d = (%v, %v), want a free attempt", i+2, allowed, wait)
			}
			continue
		}
		step := i - (reconnectFreeAttempts - 1)
		if step >= len(wantWaits) {
			step = len(wantWaits) - 1
		}
		if allowed || wait != wantWaits[step] {
			t.Fatalf("attempt %d = (%v, %v), want wait %v", i+2, allowed, wait, wantWaits[step])
		}
	}
}

// After the socket stays open for the stable window, the next drop gets a fresh
// budget. This is the normal healthy-instance case.
func TestReconnectStableConnectionResetsBudget(t *testing.T) {
	const id = "reconnect-stable-reset"
	resetReconnectTestState(t, id)
	now := time.Unix(1_700_000_000, 0)

	// Burn the free attempts and enter the backoff.
	for i := 0; i < reconnectFreeAttempts+1; i++ {
		reconnectConnectedAt(id, now)
		now = now.Add(time.Second)
		allowed, wait := reconnectAllowedAt(id, now)
		reconnectWaitDone(id)
		if i == reconnectFreeAttempts && (allowed || wait == 0) {
			t.Fatalf("attempt %d = (%v, %v), want a backoff", i+1, allowed, wait)
		}
	}

	// The instance reconnects and stays up well past the stable window.
	reconnectConnectedAt(id, now)
	now = now.Add(stableConnectionWindow + time.Second)
	allowed, wait := reconnectAllowedAt(id, now)
	if !allowed || wait != 0 {
		t.Fatalf("stable drop = (%v, %v), want a fresh budget", allowed, wait)
	}
}

// A manual reconnect (or API connect) clears the block and the history.
func TestReconnectManualClearResetsBlock(t *testing.T) {
	const id = "reconnect-manual-clear"
	resetReconnectTestState(t, id)

	reconnectBlock(id, "session replaced")
	if allowed, wait := reconnectAllowedAt(id, time.Now()); allowed || wait != reconnectBlockedResult {
		t.Fatalf("blocked = (%v, %v), want (false, %v)", allowed, wait, reconnectBlockedResult)
	}
	if got := reconnectBlockedReason(id); got != "session replaced" {
		t.Fatalf("reason = %q", got)
	}

	ClearReconnectBackoff(id)
	if allowed, wait := reconnectAllowedAt(id, time.Now()); !allowed || wait != 0 {
		t.Fatalf("after clear = (%v, %v), want (true, 0)", allowed, wait)
	}
}

// The watchdog and the normal backoff must not race: a scheduled wait is
// visible as pending.
func TestReconnectPendingWhileScheduled(t *testing.T) {
	const id = "reconnect-pending"
	resetReconnectTestState(t, id)
	now := time.Unix(1_700_000_000, 0)

	for i := 0; i < reconnectFreeAttempts; i++ {
		reconnectAllowedAt(id, now)
	}
	allowed, wait := reconnectAllowedAt(id, now)
	if allowed || wait == 0 || wait == reconnectBlockedResult {
		t.Fatalf("expected a scheduled backoff, got (%v, %v)", allowed, wait)
	}
	if !reconnectPending(id) {
		t.Fatal("a scheduled wait must read as pending")
	}
	reconnectWaitDone(id)
	if reconnectPending(id) {
		t.Fatal("a completed wait must not read as pending")
	}
}

func TestReconnectConflictCap(t *testing.T) {
	const id = "reconnect-conflicts"
	resetReconnectTestState(t, id)

	for i := 1; i <= 10; i++ {
		count, capped := reconnectRecordConflict(id, 10)
		if count != i {
			t.Fatalf("conflict count = %d, want %d", count, i)
		}
		if capped != (i >= 10) {
			t.Fatalf("capped at %d = %v", i, capped)
		}
	}

	// 0 means unlimited: never capped.
	if _, capped := reconnectRecordConflict(id, 0); capped {
		t.Fatal("limit 0 must not cap")
	}

	// A non-440 disconnect ends the streak.
	reconnectDisconnected(id)
	if _, capped := reconnectRecordConflict(id, 10); capped {
		t.Fatal("a non-440 close must reset the consecutive count")
	}
}

func TestNonRetryableCloseCode(t *testing.T) {
	nonRetryable := []int{401, 402, 403, 406}
	retryable := []int{400, 405, 409, 413, 414, 415, 418, 500, 501, 503, 515}
	for _, code := range nonRetryable {
		if !nonRetryableCloseCode(code) {
			t.Errorf("%d should be non-retryable", code)
		}
	}
	for _, code := range retryable {
		if nonRetryableCloseCode(code) {
			t.Errorf("%d should be retryable", code)
		}
	}
}
