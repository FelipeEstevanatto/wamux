package instance_service

import (
	"errors"
	"testing"

	"github.com/patrickmn/go-cache"
	"go.mau.fi/whatsmeow"

	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	instance_repository "github.com/felipeestevanatto/wamux/pkg/instance/repository"
	"github.com/felipeestevanatto/wamux/pkg/killswitch"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	"github.com/felipeestevanatto/wamux/pkg/safemap"
)

type logoutRepo struct {
	instance_repository.InstanceRepository
	saved *instance_model.Instance
	err   error
}

func (r *logoutRepo) Update(instance *instance_model.Instance) error {
	copied := *instance
	r.saved = &copied
	return r.err
}

func logoutService(t *testing.T, repo *logoutRepo) (instances, *killswitch.Switch) {
	t.Helper()
	logs := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	t.Cleanup(func() { _ = logs.GetLogger("i").Close() })

	kill := killswitch.New()
	svc := instances{
		instanceRepository: repo,
		authCache:          cache.New(authCacheTTL, 2*authCacheTTL),
		killChannel:        safemap.New[*killswitch.Switch](),
		clientPointer:      safemap.New[*whatsmeow.Client](),
		loggerWrapper:      logs,
	}
	svc.killChannel.Set("i", kill)
	svc.clientPointer.Set("i", &whatsmeow.Client{})
	return svc, kill
}

func TestLogoutReleasesTheClientEvenIfSavingFails(t *testing.T) {
	for name, saveErr := range map[string]error{"saved": nil, "database down": errors.New("down")} {
		t.Run(name, func(t *testing.T) {
			repo := &logoutRepo{err: saveErr}
			svc, kill := logoutService(t, repo)
			instance := &instance_model.Instance{Id: "i", Jid: "1@s.whatsapp.net", Connected: true, AutoReconnect: true}

			svc.finishLogout(instance)

			if !kill.Fired() || kill.Mode() != killswitch.Stop {
				t.Errorf("kill fired=%v mode=%v", kill.Fired(), kill.Mode())
			}
			if svc.clientPointer.Get("i") != nil || svc.killChannel.Get("i") != nil {
				t.Error("the client was left behind")
			}
			if instance.Connected || instance.Jid != "" || instance.AutoReconnect {
				t.Errorf("instance %+v", instance)
			}
			if repo.saved == nil || repo.saved.Jid != "" {
				t.Errorf("saved %+v", repo.saved)
			}
		})
	}
}
