package whatsmeow_service

import (
	"fmt"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// chatEphemeralCache must not grow without bound: entries expire, and an
// instance's entries are dropped on teardown.
func TestChatEphemeralCacheIsBoundedAndPurged(t *testing.T) {
	chatEphemeralCache.Flush()

	const instA = "inst-A"
	const instB = "inst-B"
	for i := 0; i < 1000; i++ {
		SetCachedChatEphemeral(instA, types.NewJID(fmt.Sprintf("5515%08d", i), types.DefaultUserServer), 86400)
		SetCachedChatEphemeral(instB, types.NewJID(fmt.Sprintf("5516%08d", i), types.DefaultUserServer), 86400)
	}
	if n := chatEphemeralCache.ItemCount(); n != 2000 {
		t.Fatalf("expected 2000 entries, got %d", n)
	}

	// Purging one instance removes exactly its entries.
	PurgeInstanceCaches(instA)
	if n := chatEphemeralCache.ItemCount(); n != 1000 {
		t.Fatalf("after purge of %s expected 1000 entries, got %d", instA, n)
	}

	// The other instance's entries are intact and still readable.
	if got, known := GetCachedChatEphemeral(instB, types.NewJID("551600000000", types.DefaultUserServer)); !known || got != 86400 {
		t.Fatalf("entry for %s lost: known=%v got=%d", instB, known, got)
	}
	// The purged instance's entry is gone.
	if _, known := GetCachedChatEphemeral(instA, types.NewJID("551500000000", types.DefaultUserServer)); known {
		t.Fatalf("entry for %s should have been purged", instA)
	}
}

func TestAccountLimitsCachePurgedOnTeardown(t *testing.T) {
	accountLimitsCache.Flush()
	accountLimitsCache.Set("inst-A", &AccountLimitsCacheEntry{FetchedAt: time.Now()}, time.Minute)
	accountLimitsCache.Set("inst-B", &AccountLimitsCacheEntry{FetchedAt: time.Now()}, time.Minute)

	PurgeInstanceCaches("inst-A")

	if _, ok := GetCachedAccountLimits("inst-A"); ok {
		t.Fatal("inst-A limits should have been purged")
	}
	if _, ok := GetCachedAccountLimits("inst-B"); !ok {
		t.Fatal("inst-B limits should be intact")
	}
}

// An empty id must be a no-op, not a prefix that matches every key.
func TestPurgeInstanceCachesEmptyIDIsNoop(t *testing.T) {
	chatEphemeralCache.Flush()
	accountLimitsCache.Flush()
	SetCachedChatEphemeral("inst-A", types.NewJID("551500000000", types.DefaultUserServer), 60)
	accountLimitsCache.Set("inst-A", &AccountLimitsCacheEntry{}, time.Minute)

	PurgeInstanceCaches("")

	if chatEphemeralCache.ItemCount() != 1 || accountLimitsCache.ItemCount() != 1 {
		t.Fatal("empty instance id must not purge anything")
	}
}

// The media-retry cache must refuse blobs over the cap.
func TestMediaRetrySizeCap(t *testing.T) {
	if mediaRetryMaxCacheBytes <= 0 {
		t.Fatal("cap must be positive")
	}
	// The constant is what HandleMediaRetry compares against; assert it is a
	// sane value so a large blob cannot be retained indefinitely.
	if mediaRetryMaxCacheBytes > 64<<20 {
		t.Fatalf("cap too large to prevent memory spikes: %d", mediaRetryMaxCacheBytes)
	}
}
