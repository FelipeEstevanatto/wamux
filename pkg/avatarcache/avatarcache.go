// Package avatarcache holds profile-picture answers for the API and lets the
// WhatsApp event loop invalidate them when a picture changes.
package avatarcache

import (
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"go.mau.fi/whatsmeow/types"
)

const (
	// A found picture is reused for this long. The URL itself can outlive it;
	// the TTL mainly bounds staleness after a picture change that we did not see.
	hitTTL = 10 * time.Minute
	// "No picture" is cached longer than a failure, but not forever: a contact
	// can add a picture later.
	missTTL = 15 * time.Minute
)

type miss struct{}

var entries = cache.New(hitTTL, 2*time.Minute)

func key(instanceID string, jid types.JID, preview bool) string {
	// Boolean is spelled out so the two forms cannot collide.
	if preview {
		return instanceID + "|" + jid.ToNonAD().String() + "|1"
	}
	return instanceID + "|" + jid.ToNonAD().String() + "|0"
}

// Get returns (picture, true, false) on a hit, (nil, true, true) for a cached
// "no picture", and (nil, false, false) on a miss.
func Get(instanceID string, jid types.JID, preview bool) (*types.ProfilePictureInfo, bool, bool) {
	v, ok := entries.Get(key(instanceID, jid, preview))
	if !ok {
		return nil, false, false
	}
	if _, missing := v.(miss); missing {
		return nil, true, true
	}
	pic, ok := v.(*types.ProfilePictureInfo)
	if !ok {
		return nil, false, false
	}
	return pic, true, false
}

func Set(instanceID string, jid types.JID, preview bool, pic *types.ProfilePictureInfo) {
	if pic == nil {
		return
	}
	entries.Set(key(instanceID, jid, preview), pic, hitTTL)
}

func SetMiss(instanceID string, jid types.JID, preview bool) {
	entries.Set(key(instanceID, jid, preview), miss{}, missTTL)
}

// Invalidate drops both preview forms for one contact after a Picture or
// Contact event says the picture changed.
func Invalidate(instanceID string, jid types.JID) {
	if jid.IsEmpty() {
		return
	}
	entries.Delete(key(instanceID, jid, false))
	entries.Delete(key(instanceID, jid, true))
}

// Rate limiting. WhatsApp answers rate-overlimit when profile pictures are
// queried in a burst; a small per-instance bucket keeps a bulk contact list
// from turning into one.
const (
	ratePerMinute = 30.0
	rateBurst     = 5.0
)

type bucket struct {
	tokens float64
	last   time.Time
}

var (
	limitMu  sync.Mutex
	limiters = map[string]*bucket{}
)

// Allow consumes one token for instanceID. It is false when the instance has
// exhausted its burst and refill budget.
func Allow(instanceID string, now time.Time) bool {
	limitMu.Lock()
	defer limitMu.Unlock()

	b := limiters[instanceID]
	if b == nil {
		b = &bucket{tokens: rateBurst, last: now}
		limiters[instanceID] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * (ratePerMinute / 60.0)
		if b.tokens > rateBurst {
			b.tokens = rateBurst
		}
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
