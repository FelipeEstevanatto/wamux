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

// s3Repo is a minimal InstanceRepository for the S3 config path.
type s3Repo struct {
	instance_repository.InstanceRepository
	instance *instance_model.Instance
	updates  map[string]interface{}
}

func (r *s3Repo) GetInstanceByID(string) (*instance_model.Instance, error) { return r.instance, nil }

func (r *s3Repo) UpdateS3Config(_ string, updates map[string]interface{}) error {
	r.updates = updates
	// Apply the subset the assertions read back through GetS3Config.
	if v, ok := updates["s3_enabled"].(bool); ok {
		r.instance.S3Enabled = v
	}
	if v, ok := updates["s3_endpoint"].(string); ok {
		r.instance.S3Endpoint = v
	}
	if v, ok := updates["s3_bucket"].(string); ok {
		r.instance.S3Bucket = v
	}
	if v, ok := updates["s3_media_delivery"].(string); ok {
		r.instance.S3MediaDelivery = v
	}
	if v, ok := updates["s3_secret_key"].(string); ok {
		r.instance.S3SecretKey = v
	}
	return nil
}

type s3Whatsmeow struct {
	whatsmeow_service.WhatsmeowService
	invalidated int
}

func (s *s3Whatsmeow) InvalidateMediaStorage(string) { s.invalidated++ }

func newS3Service(t *testing.T) (instances, *s3Repo, *s3Whatsmeow, []byte) {
	t.Helper()
	encKey, err := webhooksign.DeriveEncryptionKey("instance-service-s3-test-secret")
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	repo := &s3Repo{instance: &instance_model.Instance{Id: "11111111-1111-1111-1111-111111111111"}}
	stub := &s3Whatsmeow{}
	svc := instances{
		instanceRepository: repo,
		whatsmeowService:   stub,
		config:             &config.Config{DataEncryptionKey: encKey},
		loggerWrapper:      logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()}),
	}
	return svc, repo, stub, encKey
}

func TestSetS3ConfigEncryptsSecretAndInvalidates(t *testing.T) {
	svc, repo, stub, encKey := newS3Service(t)

	status, err := svc.SetS3Config(repo.instance.Id, &S3ConfigStruct{
		Enabled:       true,
		Endpoint:      "https://s3.example.com",
		Region:        "us-east-1",
		Bucket:        "media",
		AccessKey:     "AKIAEXAMPLE",
		SecretKey:     "super-secret",
		MediaDelivery: "both",
	})
	if err != nil {
		t.Fatalf("SetS3Config: %v", err)
	}

	stored, _ := repo.updates["s3_secret_key"].(string)
	if stored == "" || stored == "super-secret" {
		t.Fatalf("stored secret %q must be an encrypted form", stored)
	}
	decrypted, err := webhooksign.Decrypt(encKey, stored)
	if err != nil || decrypted != "super-secret" {
		t.Fatalf("stored secret decrypts to %q (%v)", decrypted, err)
	}

	if status.SecretKeySet != true || status.MediaDelivery != "both" || status.Endpoint != "https://s3.example.com" {
		t.Fatalf("status = %+v", status)
	}
	if stub.invalidated != 1 {
		t.Fatalf("media storage invalidations = %d, want 1", stub.invalidated)
	}
}

func TestSetS3ConfigValidation(t *testing.T) {
	svc, repo, _, _ := newS3Service(t)

	// Enabled without a secret and none stored.
	if _, err := svc.SetS3Config(repo.instance.Id, &S3ConfigStruct{Enabled: true, Endpoint: "e", Bucket: "b", AccessKey: "a"}); err == nil {
		t.Fatal("expected an error when secretKey is missing")
	}
	// Enabled without endpoint.
	if _, err := svc.SetS3Config(repo.instance.Id, &S3ConfigStruct{Enabled: true, Bucket: "b", AccessKey: "a", SecretKey: "s"}); err == nil {
		t.Fatal("expected an error when endpoint is missing")
	}
	// Invalid delivery mode.
	if _, err := svc.SetS3Config(repo.instance.Id, &S3ConfigStruct{MediaDelivery: "ftp"}); err == nil {
		t.Fatal("expected an error for an invalid mediaDelivery")
	}
}

func TestSetS3ConfigKeepsStoredSecretWhenOmitted(t *testing.T) {
	svc, repo, _, _ := newS3Service(t)

	if _, err := svc.SetS3Config(repo.instance.Id, &S3ConfigStruct{Enabled: true, Endpoint: "https://s3.example.com", Bucket: "b", AccessKey: "a", SecretKey: "first-secret"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	first := repo.instance.S3SecretKey
	repo.updates = nil

	// Re-save without a secret: must keep the stored one and not touch it.
	if _, err := svc.SetS3Config(repo.instance.Id, &S3ConfigStruct{Enabled: true, Endpoint: "https://s3.example.com", Bucket: "b2", AccessKey: "a"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, ok := repo.updates["s3_secret_key"]; ok {
		t.Fatal("an omitted secret must not be rewritten")
	}
	if repo.instance.S3SecretKey != first {
		t.Fatal("the stored secret changed")
	}
}

func TestDeleteS3ConfigClearsAndInvalidates(t *testing.T) {
	svc, repo, stub, _ := newS3Service(t)
	if _, err := svc.SetS3Config(repo.instance.Id, &S3ConfigStruct{Enabled: true, Endpoint: "e", Bucket: "b", AccessKey: "a", SecretKey: "s"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := svc.DeleteS3Config(repo.instance.Id); err != nil {
		t.Fatalf("DeleteS3Config: %v", err)
	}
	if repo.updates["s3_enabled"] != false || repo.updates["s3_bucket"] != "" || repo.updates["s3_secret_key"] != "" {
		t.Fatalf("clear updates = %+v", repo.updates)
	}
	if stub.invalidated != 2 {
		t.Fatalf("invalidations = %d, want 2 (set + clear)", stub.invalidated)
	}
}
