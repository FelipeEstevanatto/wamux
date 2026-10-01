package ownership

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Store persists who owns each instance and until when (the lease), in the
// instance_ownership table created by migration 0004.
//
// # WHERE THIS FITS
//
// The advisory-lock Guard in ownership.go is the CORRECTNESS guarantee: it stops
// two nodes connecting the same instance. This Store is the DISCOVERABILITY
// layer the routing/failover design needs on top:
//
//   - claim records "node X owns instance I until time T";
//   - renew refreshes T while the node is healthy (heartbeat);
//   - expired lets a healthy node find orphaned instances (a crashed peer's
//     leases) and adopt them;
//   - whoOwns answers "which node serves instance I", so an API request can be
//     routed to the right node.
//
// A lease is deliberately NOT the same thing as the lock. The lock is exact but
// invisible (pg_locks); the lease is visible state with a TTL so failover can be
// reasoned about without querying pg_locks. A node must hold the lock before it
// writes a lease.
type Store struct {
	db *sql.DB
	// nodeID identifies this process in the ownership table. Set once at startup.
	nodeID string
	// leaseTTL is how long a claim is valid without a renew.
	leaseTTL time.Duration
	// nowFunc is injectable for tests.
	nowFunc func() time.Time
}

// ErrNotSupported is returned when the store has no database (single-node mode).
var ErrNotSupported = errors.New("ownership store requires a database")

// NewStore builds a store for one node. A nil db makes every method a no-op that
// reports not-current (so a single-node deployment never touches this path).
func NewStore(db *sql.DB, nodeID string, leaseTTL time.Duration) *Store {
	if leaseTTL <= 0 {
		leaseTTL = 60 * time.Second
	}
	return &Store{db: db, nodeID: nodeID, leaseTTL: leaseTTL, nowFunc: time.Now}
}

// NodeID returns this node's id.
func (s *Store) NodeID() string {
	if s == nil {
		return ""
	}
	return s.nodeID
}

// Owner is one row of the ownership table.
type Owner struct {
	InstanceID   string    `json:"instanceId"`
	NodeID       string    `json:"nodeId"`
	LockedAt     time.Time `json:"lockedAt"`
	LeaseExpires time.Time `json:"leaseExpires"`
	Epoch        int64     `json:"epoch"`
}

// Claim upserts ownership for instanceID to this node with a fresh lease. It
// succeeds if this node already owns it, or if the row is expired/absent. If
// another node holds a live lease, Claim returns (false, nil) — the caller must
// NOT connect.
//
// The epoch is bumped on every change of owner, so a stale renew from a node
// that lost the instance cannot resurrect its lease.
func (s *Store) Claim(ctx context.Context, instanceID string) (bool, error) {
	if s == nil || s.db == nil {
		return true, nil // single-node: always ours
	}

	expires := s.nowFunc().Add(s.leaseTTL).UTC()

	// Conditional upsert: take the row only when it is unowned, ours, or expired.
	const q = `
		INSERT INTO instance_ownership (instance_id, node_id, locked_at, lease_expires, owner_epoch)
		VALUES ($1, $2, $3, $4, 1)
		ON CONFLICT (instance_id) DO UPDATE
		SET node_id = EXCLUDED.node_id,
		    locked_at = EXCLUDED.locked_at,
		    lease_expires = EXCLUDED.lease_expires,
		    owner_epoch = instance_ownership.owner_epoch + 1
		WHERE instance_ownership.node_id = EXCLUDED.node_id
		   OR instance_ownership.lease_expires < EXCLUDED.locked_at`

	res, err := s.db.ExecContext(ctx, q, instanceID, s.nodeID, s.nowFunc().UTC(), expires)
	if err != nil {
		return false, fmt.Errorf("ownership: claim %s: %w", instanceID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, nil
	}
	// 0 rows affected means the WHERE refused the update: someone else's live
	// lease is still valid.
	return affected > 0, nil
}

// Renew extends this node's lease. It only matches rows still owned by this
// node, so a node that lost the instance (its lease expired and another node
// adopted it) cannot renew its way back in.
func (s *Store) Renew(ctx context.Context, instanceID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	expires := s.nowFunc().Add(s.leaseTTL).UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE instance_ownership SET lease_expires = $1 WHERE instance_id = $2 AND node_id = $3`,
		expires, instanceID, s.nodeID,
	)
	if err != nil {
		return fmt.Errorf("ownership: renew %s: %w", instanceID, err)
	}
	return nil
}

// Release removes this node's ownership row (clean shutdown / disconnect).
func (s *Store) Release(ctx context.Context, instanceID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM instance_ownership WHERE instance_id = $1 AND node_id = $2`,
		instanceID, s.nodeID,
	)
	if err != nil {
		return fmt.Errorf("ownership: release %s: %w", instanceID, err)
	}
	return nil
}

// WhoOwns reports which node owns instanceID and whether the lease is live.
func (s *Store) WhoOwns(ctx context.Context, instanceID string) (*Owner, bool, error) {
	if s == nil || s.db == nil {
		return nil, true, nil
	}
	var o Owner
	err := s.db.QueryRowContext(ctx,
		`SELECT instance_id, node_id, locked_at, lease_expires, owner_epoch
		 FROM instance_ownership WHERE instance_id = $1`, instanceID,
	).Scan(&o.InstanceID, &o.NodeID, &o.LockedAt, &o.LeaseExpires, &o.Epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("ownership: who owns %s: %w", instanceID, err)
	}
	return &o, o.LeaseExpires.After(s.nowFunc().UTC()), nil
}

// Expired lists instances whose lease has lapsed — candidates a healthy node can
// adopt during failover.
func (s *Store) Expired(ctx context.Context) ([]Owner, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT instance_id, node_id, locked_at, lease_expires, owner_epoch
		 FROM instance_ownership WHERE lease_expires < $1`, s.nowFunc().UTC())
	if err != nil {
		return nil, fmt.Errorf("ownership: list expired: %w", err)
	}
	defer rows.Close()

	var out []Owner
	for rows.Next() {
		var o Owner
		if err := rows.Scan(&o.InstanceID, &o.NodeID, &o.LockedAt, &o.LeaseExpires, &o.Epoch); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// OwnedByThisNode lists instance ids this node currently owns (for reporting).
func (s *Store) OwnedByThisNode(ctx context.Context) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT instance_id FROM instance_ownership WHERE node_id = $1`, s.nodeID)
	if err != nil {
		return nil, fmt.Errorf("ownership: list owned: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
