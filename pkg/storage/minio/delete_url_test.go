package minio_storage

import (
	"context"
	"strings"
	"testing"
)

func TestDeleteURLRefusesObjectsOutsideTheMediaFolder(t *testing.T) {
	m := &MinioMediaStorage{bucketName: "media"}
	if err := m.DeleteURL(context.Background(), "https://example.com/secrets/config.json"); err == nil {
		t.Fatal("DeleteURL accepted a path outside wamux-medias/")
	}
	if err := m.DeleteURL(context.Background(), "not a url"); err == nil || !strings.Contains(err.Error(), "invalid media URL") {
		t.Fatalf("invalid URL error = %v", err)
	}
}
