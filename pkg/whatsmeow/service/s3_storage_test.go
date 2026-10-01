package whatsmeow_service

import (
	"context"
	"testing"

	"github.com/felipeestevanatto/wamux/pkg/config"
	instance_model "github.com/felipeestevanatto/wamux/pkg/instance/model"
	logger_wrapper "github.com/felipeestevanatto/wamux/pkg/logger"
	"github.com/felipeestevanatto/wamux/pkg/safemap"
	"github.com/felipeestevanatto/wamux/pkg/webhooksign"
)

type stubStorage struct{}

func (stubStorage) Store(context.Context, []byte, string, string) (string, error) { return "", nil }
func (stubStorage) Delete(context.Context, string) error                          { return nil }
func (stubStorage) GetURL(context.Context, string) (string, error)                { return "", nil }

func TestInstanceMediaDelivery(t *testing.T) {
	cases := map[string]string{
		"":        "base64",
		"base64":  "base64",
		"s3":      "s3",
		"both":    "both",
		"BOTH":    "both",
		"garbage": "base64",
	}
	for in, want := range cases {
		if got := instanceMediaDelivery(in); got != want {
			t.Errorf("instanceMediaDelivery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMediaStorageForFallbacks(t *testing.T) {
	logs := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	t.Cleanup(func() { logs.Flush("i") })

	// No global MinIO, no instance S3 -> no storage, base64.
	svc := &whatsmeowService{
		config:            &config.Config{},
		loggerWrapper:     logs,
		instanceS3Storage: safemap.New[instanceS3Entry](),
	}
	if s, d := svc.MediaStorageFor(nil); s != nil || d != "base64" {
		t.Fatalf("nil instance -> (%v, %q), want (nil, base64)", s, d)
	}
	if s, d := svc.MediaStorageFor(&instance_model.Instance{Id: "i"}); s != nil || d != "base64" {
		t.Fatalf("disabled s3 -> (%v, %q), want (nil, base64)", s, d)
	}

	// Global MinIO enabled -> its storage with s3 delivery (historical behaviour).
	global := &whatsmeowService{
		config:            &config.Config{MinioEnabled: true},
		loggerWrapper:     logs,
		mediaStorage:      stubStorage{},
		instanceS3Storage: safemap.New[instanceS3Entry](),
	}
	if s, d := global.MediaStorageFor(&instance_model.Instance{Id: "i"}); s == nil || d != "s3" {
		t.Fatalf("global minio -> (%v, %q), want (storage, s3)", s, d)
	}
}

func TestMediaStorageForInstanceS3BuildsAndCaches(t *testing.T) {
	encKey, err := webhooksign.DeriveEncryptionKey("whatsmeow-s3-test-secret")
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	enc, err := webhooksign.Encrypt(encKey, "the-secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	logs := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	t.Cleanup(func() { logs.Flush("inst-1") })

	svc := &whatsmeowService{
		config:            &config.Config{DataEncryptionKey: encKey},
		loggerWrapper:     logs,
		instanceS3Storage: safemap.New[instanceS3Entry](),
	}

	// A public URL makes the client skip the (network) bucket-policy call, so
	// this stays hermetic.
	inst := &instance_model.Instance{
		Id:              "inst-1",
		S3Enabled:       true,
		S3Endpoint:      "localhost:9000",
		S3Bucket:        "media",
		S3AccessKey:     "ak",
		S3SecretKey:     enc,
		S3MediaDelivery: "both",
		S3PublicURL:     "https://cdn.example.com",
	}

	first, delivery := svc.MediaStorageFor(inst)
	if first == nil || delivery != "both" {
		t.Fatalf("instance s3 -> (%v, %q), want (storage, both)", first, delivery)
	}

	// Same signature -> cached instance returned.
	second, _ := svc.MediaStorageFor(inst)
	if second != first {
		t.Fatal("expected the built storage to be cached")
	}

	// Config change (new encrypted secret) -> rebuilt.
	newEnc, _ := webhooksign.Encrypt(encKey, "rotated")
	inst.S3SecretKey = newEnc
	third, _ := svc.MediaStorageFor(inst)
	if third == first {
		t.Fatal("expected a rebuild after the secret changed")
	}

	svc.InvalidateMediaStorage(inst.Id)
	if _, ok := svc.instanceS3Storage.Lookup(inst.Id); ok {
		t.Fatal("InvalidateMediaStorage did not drop the cache entry")
	}
}
