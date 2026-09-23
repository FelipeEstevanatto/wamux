package send_service

import (
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/patrickmn/go-cache"
)

// testSendService builds a service with a working logger and cache but a nil
// client pointer: a cache hit must resolve the recipient without ever reaching
// for a client, so a nil map would panic if the cache were bypassed.
func testSendService(t *testing.T) *sendService {
	t.Helper()
	logCfg := &config.Config{LogDirectory: t.TempDir()}
	return &sendService{
		config:          &config.Config{CheckUserExists: true, LogDirectory: t.TempDir()},
		loggerWrapper:   logger_wrapper.NewLoggerManager(logCfg),
		userExistsCache: cache.New(time.Minute, 2*time.Minute),
	}
}

// A cached positive resolution must short-circuit before any client/network use.
func TestValidateAndCheckUserExistsUsesCachedPositive(t *testing.T) {
	s := testSendService(t)
	s.userExistsCache.Set("5514991421911",
		userExistsResult{remoteJID: "5514991421911@s.whatsapp.net", found: true},
		cache.DefaultExpiration)

	jid, err := s.validateAndCheckUserExists("5514991421911", nil, nil, nil, &instance_model.Instance{Id: "inst-1"})
	if err != nil {
		t.Fatalf("cached positive lookup failed: %v", err)
	}
	if jid.User != "5514991421911" {
		t.Fatalf("unexpected JID: %+v", jid)
	}
}

// A cached negative must fail fast without a client round trip.
func TestValidateAndCheckUserExistsUsesCachedNegative(t *testing.T) {
	s := testSendService(t)
	s.userExistsCache.Set("5514000000000",
		userExistsResult{found: false},
		cache.DefaultExpiration)

	if _, err := s.validateAndCheckUserExists("5514000000000", nil, nil, nil, &instance_model.Instance{Id: "inst-1"}); err == nil {
		t.Fatal("expected an error for a cached negative result")
	}
}

// With the check disabled, group/broadcast/newsletter/lid recipients must not be
// looked up at all (no cache, no client).
func TestValidateAndCheckUserExistsSkipsGroupsAndLids(t *testing.T) {
	s := testSendService(t)
	for _, phone := range []string{"12345@g.us", "status@broadcast", "123@newsletter", "12345@lid"} {
		if _, err := s.validateAndCheckUserExists(phone, nil, nil, nil, &instance_model.Instance{Id: "inst-1"}); err != nil {
			t.Fatalf("group/lid %q should not need a client: %v", phone, err)
		}
	}
}
