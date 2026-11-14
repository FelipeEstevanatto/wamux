package avatarcache

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestSetGetAndInvalidate(t *testing.T) {
	instance := "test-instance"
	jid := types.NewJID("5511999999999", types.DefaultUserServer)
	pic := &types.ProfilePictureInfo{URL: "https://example/pic.jpg"}

	Set(instance, jid, false, pic)
	if got, hit, missing := Get(instance, jid, false); !hit || missing || got.URL != pic.URL {
		t.Fatalf("get = %+v hit=%v missing=%v", got, hit, missing)
	}
	if _, hit, _ := Get(instance, jid, true); hit {
		t.Fatal("preview and full forms must not share an entry")
	}

	Invalidate(instance, jid)
	if _, hit, _ := Get(instance, jid, false); hit {
		t.Fatal("invalidate did not drop the full form")
	}
}

func TestMissIsCachedSeparately(t *testing.T) {
	instance := "test-miss"
	jid := types.NewJID("5511888888888", types.DefaultUserServer)
	SetMiss(instance, jid, false)

	if pic, hit, missing := Get(instance, jid, false); !hit || !missing || pic != nil {
		t.Fatalf("miss = %+v hit=%v missing=%v", pic, hit, missing)
	}
}

func TestRateLimitBurstThenRefill(t *testing.T) {
	Configure(60, 2)
	defer Configure(defaultRatePerMinute, defaultRateBurst)
	instance := "test-rate"
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < 2; i++ {
		if !Allow(instance, now) {
			t.Fatalf("request %d denied inside burst", i+1)
		}
	}
	if Allow(instance, now) {
		t.Fatal("burst was not exhausted")
	}
	if !Allow(instance, now.Add(time.Minute)) {
		t.Fatal("a minute did not refill the bucket")
	}
}

func TestRateLimitCanBeDisabled(t *testing.T) {
	Configure(0, 0)
	defer Configure(defaultRatePerMinute, defaultRateBurst)
	instance := "test-rate-disabled"
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < 20; i++ {
		if !Allow(instance, now) {
			t.Fatal("disabled limiter denied a request")
		}
	}
}
