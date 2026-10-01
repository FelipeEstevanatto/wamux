package ownership

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newStoreDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "own.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`CREATE TABLE instance_ownership (
		instance_id   TEXT PRIMARY KEY,
		node_id       TEXT NOT NULL,
		locked_at     TIMESTAMP NOT NULL,
		lease_expires TIMESTAMP NOT NULL,
		owner_epoch   INTEGER NOT NULL DEFAULT 1
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func TestClaimWinsWhenUnowned(t *testing.T) {
	db := newStoreDB(t)
	s := NewStore(db, "node-1", time.Minute)

	ok, err := s.Claim(context.Background(), "inst-1")
	if err != nil || !ok {
		t.Fatalf("claim = %v, %v; want true", ok, err)
	}
	owner, live, err := s.WhoOwns(context.Background(), "inst-1")
	if err != nil || !live || owner.NodeID != "node-1" {
		t.Fatalf("who owns = %+v live=%v err=%v", owner, live, err)
	}
}

func TestClaimRefusedWhenAnotherNodeHoldsLiveLease(t *testing.T) {
	db := newStoreDB(t)
	a := NewStore(db, "node-a", time.Minute)
	b := NewStore(db, "node-b", time.Minute)

	if ok, _ := a.Claim(context.Background(), "inst-1"); !ok {
		t.Fatalf("node-a should claim")
	}
	// node-b must NOT be able to take a live lease.
	ok, err := b.Claim(context.Background(), "inst-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if ok {
		t.Fatalf("node-b should be refused while node-a's lease is live")
	}
}

func TestClaimSucceedsAfterLeaseExpires(t *testing.T) {
	db := newStoreDB(t)
	a := NewStore(db, "node-a", time.Minute)
	b := NewStore(db, "node-b", time.Minute)

	base := time.Now()
	a.nowFunc = func() time.Time { return base }
	if ok, _ := a.Claim(context.Background(), "inst-1"); !ok {
		t.Fatalf("node-a should claim")
	}

	// Move time past node-a's lease and let node-b adopt it (failover).
	b.nowFunc = func() time.Time { return base.Add(2 * time.Minute) }
	ok, err := b.Claim(context.Background(), "inst-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if !ok {
		t.Fatalf("node-b should adopt an expired lease")
	}
	owner, _, _ := b.WhoOwns(context.Background(), "inst-1")
	if owner.NodeID != "node-b" {
		t.Fatalf("owner = %s, want node-b", owner.NodeID)
	}
}

func TestRenewOnlyMatchesOwner(t *testing.T) {
	db := newStoreDB(t)
	a := NewStore(db, "node-a", time.Minute)
	b := NewStore(db, "node-b", time.Minute)

	if ok, _ := a.Claim(context.Background(), "inst-1"); !ok {
		t.Fatalf("node-a should claim")
	}
	// node-b's renew must not resurrect/extend node-a's row.
	if err := b.Renew(context.Background(), "inst-1"); err != nil {
		t.Fatalf("renew: %v", err)
	}
	owner, _, _ := b.WhoOwns(context.Background(), "inst-1")
	if owner.NodeID != "node-a" {
		t.Fatalf("renew by non-owner changed the owner to %s", owner.NodeID)
	}
}

func TestReleaseRemovesRow(t *testing.T) {
	db := newStoreDB(t)
	s := NewStore(db, "node-1", time.Minute)
	_, _ = s.Claim(context.Background(), "inst-1")
	if err := s.Release(context.Background(), "inst-1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, live, _ := s.WhoOwns(context.Background(), "inst-1"); live {
		t.Fatalf("released instance should not be owned")
	}
}

func TestExpiredListsLapsedLeases(t *testing.T) {
	db := newStoreDB(t)
	a := NewStore(db, "node-a", time.Minute)
	base := time.Now()
	a.nowFunc = func() time.Time { return base }
	_, _ = a.Claim(context.Background(), "inst-1")

	a.nowFunc = func() time.Time { return base.Add(10 * time.Minute) }
	expired, err := a.Expired(context.Background())
	if err != nil {
		t.Fatalf("expired: %v", err)
	}
	if len(expired) != 1 || expired[0].InstanceID != "inst-1" {
		t.Fatalf("expired = %+v, want inst-1", expired)
	}
}

func TestNilStoreIsSingleNode(t *testing.T) {
	var s *Store
	ok, err := s.Claim(context.Background(), "x")
	if err != nil || !ok {
		t.Fatalf("nil store should always claim: %v %v", ok, err)
	}
}
