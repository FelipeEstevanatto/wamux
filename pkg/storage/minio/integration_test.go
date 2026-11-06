package minio_storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	storage_interfaces "github.com/felipeestevanatto/wamux/pkg/storage/interfaces"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// These run against a real S3 server: set WAMUX_TEST_S3_ENDPOINT, _ACCESS_KEY and
// _SECRET_KEY. Each test gets its own bucket.
func realOptions(t *testing.T) Options {
	t.Helper()
	endpoint := os.Getenv("WAMUX_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set WAMUX_TEST_S3_ENDPOINT to run S3 tests")
	}
	o := Options{
		Endpoint:  endpoint,
		AccessKey: os.Getenv("WAMUX_TEST_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("WAMUX_TEST_S3_SECRET_KEY"),
		Bucket:    fmt.Sprintf("wamux-test-%d", time.Now().UnixNano()),
		PathStyle: true,
	}
	admin, err := minio.New(o.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(o.AccessKey, o.SecretKey, "")})
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.MakeBucket(context.Background(), o.Bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	return o
}

func realStore(t *testing.T, o Options) storage_interfaces.MediaStorage {
	t.Helper()
	store, err := NewMinioMediaStorageWithOptions(o)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return store
}

func fetch(t *testing.T, url string) (int, string, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), body
}

func TestRealStoreReturnsAURLThatServesTheFile(t *testing.T) {
	store := realStore(t, realOptions(t))
	data := []byte("real media bytes")

	url, err := store.Store(context.Background(), data, "photo.jpg", "image/jpeg")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	status, kind, body := fetch(t, url)
	if status != http.StatusOK || !bytes.Equal(body, data) {
		t.Fatalf("status %d, body %q", status, body)
	}
	if kind != "image/jpeg" {
		t.Errorf("Content-Type = %q", kind)
	}
}

// Files above the embed limit stream from disk, with their real size.
func TestRealStoreReaderStreamsALargeFile(t *testing.T) {
	store := realStore(t, realOptions(t))
	data := bytes.Repeat([]byte("0123456789abcdef"), 5<<18) // 20 MiB: over one upload part

	url, err := store.StoreReader(context.Background(), bytes.NewReader(data), int64(len(data)), "video.mp4", "video/mp4")
	if err != nil {
		t.Fatalf("StoreReader: %v", err)
	}

	status, _, body := fetch(t, url)
	if status != http.StatusOK || !bytes.Equal(body, data) {
		t.Fatalf("status %d, %d of %d bytes", status, len(body), len(data))
	}
}

func TestRealGetURLFindsAStoredFile(t *testing.T) {
	store := realStore(t, realOptions(t))
	data := []byte("found again")
	if _, err := store.Store(context.Background(), data, "doc.pdf", "application/pdf"); err != nil {
		t.Fatal(err)
	}

	url, err := store.GetURL(context.Background(), "doc.pdf")
	if err != nil {
		t.Fatalf("GetURL: %v", err)
	}

	if status, _, body := fetch(t, url); status != http.StatusOK || !bytes.Equal(body, data) {
		t.Fatalf("status %d, body %q", status, body)
	}
}

func TestRealDeleteRemovesTheFile(t *testing.T) {
	store := realStore(t, realOptions(t))
	url, err := store.Store(context.Background(), []byte("gone soon"), "old.jpg", "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Delete(context.Background(), "old.jpg"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if status, _, _ := fetch(t, url); status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestRealPublicURLReplacesThePresignedOne(t *testing.T) {
	o := realOptions(t)
	o.PublicURL = "https://cdn.test/media"
	store := realStore(t, o)

	url, err := store.Store(context.Background(), []byte("x"), "a.jpg", "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(url, o.PublicURL) || strings.Contains(url, "X-Amz-Signature") {
		t.Fatalf("url = %q", url)
	}
}

func TestRealTestConnection(t *testing.T) {
	o := realOptions(t)
	if err := TestConnection(o); err != nil {
		t.Fatalf("good credentials: %v", err)
	}

	o.SecretKey = "wrong-secret"
	if err := TestConnection(o); err == nil {
		t.Fatal("wrong credentials: want an error")
	}
}
