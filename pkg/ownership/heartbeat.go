package ownership

import (
	"context"
	"time"
)

// Heartbeat periodically renews this node's leases so they do not expire while
// the node is healthy. Run it for the process lifetime; Stop it on shutdown.
//
// A node whose leases stop being renewed will have them expire, and another
// healthy node will adopt those instances (see Expired + Claim). That is the
// failover mechanism: no manual intervention, no stale "connected" flags.
type Heartbeat struct {
	store    *Store
	interval time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
}

// StartHeartbeat renews every lease this node owns, every interval. A nil store
// (single node) returns a no-op heartbeat.
func StartHeartbeat(store *Store, interval time.Duration) *Heartbeat {
	if interval <= 0 {
		interval = 20 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &Heartbeat{store: store, interval: interval, cancel: cancel, done: make(chan struct{})}

	if store == nil || store.db == nil {
		close(h.done)
		return h
	}

	go func() {
		defer close(h.done)
		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				h.renewAll(ctx)
			}
		}
	}()
	return h
}

func (h *Heartbeat) renewAll(ctx context.Context) {
	if h.store == nil {
		return
	}
	ids, err := h.store.OwnedByThisNode(ctx)
	if err != nil {
		return
	}
	for _, id := range ids {
		_ = h.store.Renew(ctx, id)
	}
}

// Stop halts the heartbeat and waits for the goroutine to exit.
func (h *Heartbeat) Stop() {
	if h == nil {
		return
	}
	h.cancel()
	<-h.done
}
