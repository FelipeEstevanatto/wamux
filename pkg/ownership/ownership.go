// Package ownership makes "one instance is connected on exactly one node" a
// guarantee instead of a hope.
//
// # THE PROBLEM
//
// Each connected instance is one long-lived WhatsApp socket held in process
// memory (clientPointer). Nothing stopped two replicas of the API from both
// connecting the SAME instance: WhatsApp would see two sessions for one device,
// replies would be duplicated, and the device could be logged out. On a single
// node that never happened; the moment you scale horizontally it does.
//
// # THE FIX, AND WHY POSTGRES ADVISORY LOCKS
//
// A Postgres session advisory lock (pg_try_advisory_lock) is a cluster-wide
// mutex held for the life of a database connection. We take one per instance
// before connecting and hold it until the instance is torn down. If another node
// already holds it, the connect is refused with a clear reason instead of
// silently racing.
//
// An advisory lock is the right primitive here because:
//   - it needs no extra infrastructure (etcd/Consul) — Postgres is already a
//     hard dependency;
//   - it is released automatically when the connection dies, so a crashed node's
//     locks free themselves (no stale lease to expire by hand);
//   - it is exactly scoped: one lock per instance id.
//
// The lock must live on a DEDICATED connection, not one from a pool: a pool may
// hand the connection back or reuse it, and a session lock is released when its
// session ends. Holder keeps its own *sql.Conn.
//
// This package is also the single-writer half of the routing story: the day a
// scheduler assigns instances to nodes, it just needs to take the same lock.
package ownership

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"time"
)

// Held reports the outcome of a claim. Held=false with Err=nil means another
// node owns the instance (not an error — a normal "someone else has it").
type Held struct {
	Held bool
	// LockKey is the 64-bit advisory-lock key derived from the instance id, so
	// an operator can find the holder with pg_locks.
	LockKey int64
}

// Guard takes and releases per-instance advisory locks.
type Guard struct {
	db *sql.DB
	// dbKind is "postgres", "sqlite" or "". Only Postgres implements advisory
	// locks; on anything else the guard is a no-op (single node) and says so.
	dbKind string
}

// ErrUnsupported is returned by ClaimOnly on a backend without advisory locks.
var ErrUnsupported = errors.New("ownership guard requires postgres")

// NewGuard builds a guard. dbKind must be "postgres" for locking to engage; any
// other value (or a nil db) makes every claim succeed, which preserves the
// single-node behaviour.
func NewGuard(db *sql.DB, dbKind string) *Guard {
	return &Guard{db: db, dbKind: dbKind}
}

// Supported reports whether locking is actually active.
func (g *Guard) Supported() bool {
	return g != nil && g.db != nil && g.dbKind == "postgres"
}

// lockKey derives a stable 64-bit key from the instance id. Postgres advisory
// locks are keyed by int64; hashing the uuid keeps a fixed, collision-cheap key
// and makes the value reproducible across nodes (the only requirement).
func lockKey(instanceID string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("evo:instance:" + instanceID))
	return int64(h.Sum64())
}

// Holder is a held lock. Release it when the instance is torn down.
type Holder struct {
	conn    *sql.Conn
	key     int64
	guarded bool
}

// Claim tries to take the instance's lock on a dedicated connection.
//
//   - Held=true  -> this node owns the instance; call Release when done.
//   - Held=false -> another node owns it; Err is nil.
//   - Err != nil -> a real failure (couldn't reach Postgres, etc.).
//
// When the guard is unsupported (sqlite / nil db) it always returns Held=true
// with a no-op Holder, so a single-node deployment behaves exactly as before.
func (g *Guard) Claim(ctx context.Context, instanceID string) (Held, *Holder, error) {
	if !g.Supported() {
		return Held{Held: true, LockKey: lockKey(instanceID)}, &Holder{guarded: false}, nil
	}

	key := lockKey(instanceID)

	conn, err := g.db.Conn(ctx)
	if err != nil {
		return Held{}, nil, fmt.Errorf("ownership: acquire connection: %w", err)
	}

	var acquired bool
	// pg_try_advisory_lock never blocks: it returns false immediately when the
	// lock is held elsewhere, which is exactly the "another node owns it" case.
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&acquired); err != nil {
		_ = conn.Close()
		return Held{}, nil, fmt.Errorf("ownership: try_advisory_lock: %w", err)
	}

	if !acquired {
		// Important: close the connection so Postgres does not keep a session
		// (and a pool slot) open for a lock we did not get.
		_ = conn.Close()
		return Held{Held: false, LockKey: key}, nil, nil
	}

	return Held{Held: true, LockKey: key}, &Holder{conn: conn, key: key, guarded: true}, nil
}

// Release drops the lock. Safe to call on a nil or no-op holder.
func (h *Holder) Release(ctx context.Context) error {
	if h == nil {
		return nil
	}
	if !h.guarded || h.conn == nil {
		return nil
	}

	// Use a short timeout: release runs on teardown and must not hang shutdown.
	releaseCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, _ = h.conn.ExecContext(releaseCtx, `SELECT pg_advisory_unlock($1)`, h.key)
	// Closing the connection also releases the lock even if the explicit unlock
	// failed, so this is belt-and-braces.
	err := h.conn.Close()
	h.conn = nil
	if err != nil {
		return fmt.Errorf("ownership: close lock connection: %w", err)
	}
	return nil
}
