package instance_service

import (
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/evolution-foundation/evolution-go/pkg/webhooksign"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
)

// hmacRepo is a minimal InstanceRepository that only implements the three
// methods the HMAC path uses.
type hmacRepo struct {
	instance_repository.InstanceRepository
	instance *instance_model.Instance
	stored   string
}

func (r *hmacRepo) GetInstanceByID(string) (*instance_model.Instance, error) {
	return r.instance, nil
}

func (r *hmacRepo) UpdateHmacKey(_ string, key string) error {
	r.stored = key
	r.instance.HmacKey = key
	return nil
}

// hmacWhatsmeow records the plaintext key pushed to the running client.
type hmacWhatsmeow struct {
	whatsmeow_service.WhatsmeowService
	key   string
	calls int
}

func (h *hmacWhatsmeow) SetWebhookHmacKey(_ string, key string) {
	h.key = key
	h.calls++
}

func newHmacService(t *testing.T, globalFallback bool) (instances, *hmacRepo, *hmacWhatsmeow) {
	t.Helper()

	encKey, err := webhooksign.DeriveEncryptionKey("instance-service-hmac-test-secret")
	if err != nil {
		t.Fatalf("derive encryption key: %v", err)
	}
	cfg := &config.Config{WebhookHmacEncryptionKey: encKey}
	if globalFallback {
		cfg.WebhookHmacGlobalKey = "the-global-fallback-key"
	}

	repo := &hmacRepo{instance: &instance_model.Instance{Id: "11111111-1111-1111-1111-111111111111", Token: "tok"}}
	stub := &hmacWhatsmeow{}

	logs := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	// The logger's writer goroutine is asynchronous; drain it before t.TempDir
	// removes the directory, otherwise a late write races the cleanup.
	t.Cleanup(func() { logs.Flush(repo.instance.Id) })

	return instances{
		instanceRepository: repo,
		whatsmeowService:   stub,
		config:             cfg,
		loggerWrapper:      logs,
	}, repo, stub
}

func TestSetWebhookHmacKeyStoresEncryptedAndPushesPlaintext(t *testing.T) {
	svc, repo, stub := newHmacService(t, false)
	key := "a-strong-per-instance-hmac-key-123456"

	status, err := svc.SetWebhookHmacKey(repo.instance.Id, key)
	if err != nil {
		t.Fatalf("SetWebhookHmacKey: %v", err)
	}
	if !status.Configured {
		t.Fatal("status should report the key as configured")
	}

	if repo.stored == "" || repo.stored == key {
		t.Fatalf("stored value %q must be an encrypted form of the key", repo.stored)
	}
	decrypted, err := webhooksign.Decrypt(svc.config.WebhookHmacEncryptionKey, repo.stored)
	if err != nil {
		t.Fatalf("stored value is not decryptable: %v", err)
	}
	if decrypted != key {
		t.Fatalf("stored key decrypts to %q, want %q", decrypted, key)
	}

	if stub.key != key || stub.calls != 1 {
		t.Fatalf("running client got key %q (%d calls), want %q once", stub.key, stub.calls, key)
	}
}

func TestSetWebhookHmacKeyRejectsShortKey(t *testing.T) {
	svc, repo, stub := newHmacService(t, false)

	if _, err := svc.SetWebhookHmacKey(repo.instance.Id, "too-short"); err == nil {
		t.Fatal("expected an error for a key under the minimum length")
	}
	if repo.stored != "" {
		t.Fatal("a rejected key must not be stored")
	}
	if stub.calls != 0 {
		t.Fatal("a rejected key must not be pushed to the client")
	}
}

func TestClearWebhookHmacKeyRemovesStoredKey(t *testing.T) {
	svc, repo, stub := newHmacService(t, false)
	if _, err := svc.SetWebhookHmacKey(repo.instance.Id, "a-strong-per-instance-hmac-key-123456"); err != nil {
		t.Fatalf("seed key: %v", err)
	}

	if err := svc.ClearWebhookHmacKey(repo.instance.Id); err != nil {
		t.Fatalf("ClearWebhookHmacKey: %v", err)
	}
	if repo.stored != "" {
		t.Fatalf("stored key = %q, want empty", repo.stored)
	}
	if stub.key != "" {
		t.Fatalf("client key = %q, want empty", stub.key)
	}

	status, err := svc.WebhookHmacStatus(repo.instance.Id)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Configured {
		t.Fatal("status should report the key as not configured")
	}
}

func TestWebhookHmacStatusReportsGlobalFallback(t *testing.T) {
	svc, repo, _ := newHmacService(t, true)

	status, err := svc.WebhookHmacStatus(repo.instance.Id)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Configured {
		t.Fatal("a fresh instance should have no per-instance key")
	}
	if !status.GlobalFallback {
		t.Fatal("global fallback should be reported when configured")
	}
}
