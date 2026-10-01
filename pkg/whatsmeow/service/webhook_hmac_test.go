package whatsmeow_service

import (
	"testing"

	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	"github.com/felipeestevanatto/wamux/pkg/safemap"
	"github.com/felipeestevanatto/wamux/pkg/webhooksign"
)

// newHmacTestService builds the smallest service that can resolve a signing key.
func newHmacTestService(t *testing.T, globalKey string) (*whatsmeowService, []byte) {
	t.Helper()

	encKey, err := webhooksign.DeriveEncryptionKey("webhook-hmac-test-encryption-secret")
	if err != nil {
		t.Fatalf("derive encryption key: %v", err)
	}

	logs := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	// Drain the asynchronous writer before t.TempDir cleanup removes the folder.
	t.Cleanup(func() { logs.Flush("66666666-6666-6666-6666-666666666666") })

	return &whatsmeowService{
		config: &config.Config{
			WebhookHmacEncryptionKey: encKey,
			WebhookHmacGlobalKey:     globalKey,
			LogDirectory:             t.TempDir(),
		},
		loggerWrapper:   logs,
		webhookHmacKeys: safemap.New[string](),
	}, encKey
}

func encryptForTest(t *testing.T, encKey []byte, plaintext string) string {
	t.Helper()
	encrypted, err := webhooksign.Encrypt(encKey, plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return encrypted
}

func TestWebhookSigningKeyUsesStoredPerInstanceKey(t *testing.T) {
	svc, encKey := newHmacTestService(t, "global-fallback-key-global-fallback")
	instance := &instance_model.Instance{
		Id:      "11111111-1111-1111-1111-111111111111",
		HmacKey: encryptForTest(t, encKey, "per-instance-key-per-instance-key"),
	}

	if got := string(svc.webhookSigningKey(instance)); got != "per-instance-key-per-instance-key" {
		t.Fatalf("key = %q, want the per-instance key", got)
	}
}

func TestWebhookSigningKeyPrefersRuntimeOverride(t *testing.T) {
	svc, encKey := newHmacTestService(t, "global-fallback-key-global-fallback")
	instance := &instance_model.Instance{
		Id:      "22222222-2222-2222-2222-222222222222",
		HmacKey: encryptForTest(t, encKey, "old-key-old-key-old-key-old-key-old"),
	}

	svc.SetWebhookHmacKey(instance.Id, "new-key-new-key-new-key-new-key-new")
	if got := string(svc.webhookSigningKey(instance)); got != "new-key-new-key-new-key-new-key-new" {
		t.Fatalf("key = %q, want the runtime override", got)
	}
}

func TestWebhookSigningKeyClearedOverrideDisablesSigning(t *testing.T) {
	svc, encKey := newHmacTestService(t, "global-fallback-key-global-fallback")
	instance := &instance_model.Instance{
		Id:      "33333333-3333-3333-3333-333333333333",
		HmacKey: encryptForTest(t, encKey, "stored-key-stored-key-stored-key-00"),
	}

	// Clearing the key must not fall back to the old stored value or the global
	// key: the operator asked for unsigned deliveries.
	svc.SetWebhookHmacKey(instance.Id, "")
	if got := svc.webhookSigningKey(instance); got != nil {
		t.Fatalf("key = %q, want nil after clearing", got)
	}
}

func TestWebhookSigningKeyFallsBackToGlobal(t *testing.T) {
	svc, _ := newHmacTestService(t, "global-fallback-key-global-fallback")
	instance := &instance_model.Instance{Id: "44444444-4444-4444-4444-444444444444"}

	if got := string(svc.webhookSigningKey(instance)); got != "global-fallback-key-global-fallback" {
		t.Fatalf("key = %q, want the global key", got)
	}
}

func TestWebhookSigningKeyNilWhenNothingConfigured(t *testing.T) {
	svc, _ := newHmacTestService(t, "")
	instance := &instance_model.Instance{Id: "55555555-5555-5555-5555-555555555555"}

	if got := svc.webhookSigningKey(instance); got != nil {
		t.Fatalf("key = %q, want nil when nothing is configured", got)
	}
	if got := svc.webhookSigningKey(nil); got != nil {
		t.Fatalf("key = %q, want nil for a nil instance", got)
	}
}

func TestWebhookSigningKeyUndecryptableStoredKeyFallsBack(t *testing.T) {
	svc, _ := newHmacTestService(t, "global-fallback-key-global-fallback")
	instance := &instance_model.Instance{
		Id:      "66666666-6666-6666-6666-666666666666",
		HmacKey: "this-is-not-valid-ciphertext",
	}

	if got := string(svc.webhookSigningKey(instance)); got != "global-fallback-key-global-fallback" {
		t.Fatalf("key = %q, want the global fallback", got)
	}
}
