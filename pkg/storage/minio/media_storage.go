package minio_storage

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	storage_interfaces "github.com/felipeestevanatto/wamux/pkg/storage/interfaces"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioMediaStorage struct {
	client     *minio.Client
	bucketName string
	baseURL    string
	// publicURL, when set, is a CDN/public base URL returned instead of a
	// presigned URL (the bucket is expected to be publicly readable).
	publicURL string
}

// Options configures an S3-compatible media store.
type Options struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
	PathStyle bool
	PublicURL string
	// SkipBucketPolicy leaves the bucket policy untouched (useful for
	// providers that reject SetBucketPolicy, or when using presigned URLs).
	SkipBucketPolicy bool
}

func setBucketPolicy(client *minio.Client, bucketName string) error {
	policy := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": "*",
				"Action": ["s3:GetObject"],
				"Resource": ["arn:aws:s3:::` + bucketName + `/*"]
			}
		]
	}`

	return client.SetBucketPolicy(context.Background(), bucketName, policy)
}

// generateFilePath creates a simple media folder structure
// Format: wamux-medias/{filename}
func generateFilePath(fileName string) string {
	return fmt.Sprintf("wamux-medias/%s", fileName)
}

// resolveFilePath determines if the input is a full path or just a filename
// If it's just a filename, it assumes it's in the wamux-medias folder
// If it's a full path, it returns it as-is
func (m *MinioMediaStorage) resolveFilePath(_ context.Context, fileNameOrPath string) (string, error) {
	// If the input already contains path separators, assume it's a full path
	if strings.Contains(fileNameOrPath, "/") {
		return fileNameOrPath, nil
	}

	// If it's just a filename, assume it's in the wamux-medias folder
	return fmt.Sprintf("wamux-medias/%s", fileNameOrPath), nil
}

// NewMinioMediaStorageWithOptions builds a store from an explicit Options. It
// is the constructor the per-instance S3 path uses.
func NewMinioMediaStorageWithOptions(o Options) (storage_interfaces.MediaStorage, error) {
	if o.Endpoint == "" || o.Bucket == "" {
		return nil, fmt.Errorf("S3 endpoint and bucket are required")
	}

	client, err := minio.New(o.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(o.AccessKey, o.SecretKey, ""),
		Secure:       o.UseSSL,
		Region:       o.Region,
		BucketLookup: bucketLookup(o.PathStyle),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	if !o.SkipBucketPolicy {
		if err := setBucketPolicy(client, o.Bucket); err != nil {
			// Some providers (like Backblaze B2) don't support SetBucketPolicy.
			// Files can still be served via presigned or public URLs.
			fmt.Printf("Warning: Failed to set bucket policy (provider may not support it): %v\n", err)
		}
	}

	scheme := "https"
	if !o.UseSSL {
		scheme = "http"
	}
	baseURL := fmt.Sprintf("%s://%s/%s", scheme, o.Endpoint, o.Bucket)

	return &MinioMediaStorage{
		client:     client,
		bucketName: o.Bucket,
		baseURL:    baseURL,
		publicURL:  strings.TrimRight(o.PublicURL, "/"),
	}, nil
}

func bucketLookup(pathStyle bool) minio.BucketLookupType {
	if pathStyle {
		return minio.BucketLookupPath
	}
	return minio.BucketLookupAuto
}

// NormalizeEndpoint splits an S3 endpoint into the host:port minio-go expects
// and whether TLS should be used. A scheme (http:// / https://) wins; with no
// scheme, TLS is off (matching the global MinIO config's default).
func NormalizeEndpoint(raw string) (endpoint string, useSSL bool) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, "https://"):
		return strings.TrimRight(strings.TrimPrefix(raw, "https://"), "/"), true
	case strings.HasPrefix(raw, "http://"):
		return strings.TrimRight(strings.TrimPrefix(raw, "http://"), "/"), false
	default:
		return strings.TrimRight(raw, "/"), false
	}
}

func NewMinioMediaStorage(
	endpoint,
	accessKeyID,
	secretAccessKey,
	bucketName,
	region string,
	useSSL bool,
) (storage_interfaces.MediaStorage, error) {
	return NewMinioMediaStorageWithOptions(Options{
		Endpoint:  endpoint,
		AccessKey: accessKeyID,
		SecretKey: secretAccessKey,
		Bucket:    bucketName,
		Region:    region,
		UseSSL:    useSSL,
	})
}

// TestConnection verifies the endpoint and bucket are reachable with the given
// credentials. It performs no writes.
func TestConnection(o Options) error {
	if o.Endpoint == "" || o.Bucket == "" {
		return fmt.Errorf("S3 endpoint and bucket are required")
	}
	client, err := minio.New(o.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(o.AccessKey, o.SecretKey, ""),
		Secure:       o.UseSSL,
		Region:       o.Region,
		BucketLookup: bucketLookup(o.PathStyle),
	})
	if err != nil {
		return fmt.Errorf("failed to create MinIO client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	exists, err := client.BucketExists(ctx, o.Bucket)
	if err != nil {
		return fmt.Errorf("failed to reach bucket %q: %w", o.Bucket, err)
	}
	if !exists {
		return fmt.Errorf("bucket %q does not exist", o.Bucket)
	}
	return nil
}

func (m *MinioMediaStorage) Store(ctx context.Context, data []byte, fileName string, contentType string) (string, error) {
	// Generate organized file path
	filePath := generateFilePath(fileName)
	reader := bytes.NewReader(data)

	_, err := m.client.PutObject(ctx, m.bucketName, filePath, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("failed to store object: %w", err)
	}

	// A custom public URL (CDN) bypasses presigning.
	if m.publicURL != "" {
		return m.publicURL + "/" + filePath, nil
	}

	// Gerando URL assinada com validade de 7 dias
	reqParams := make(url.Values)
	presignedURL, err := m.client.PresignedGetObject(ctx, m.bucketName, filePath, time.Hour*24*7, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL.String(), nil
}

func (m *MinioMediaStorage) Delete(ctx context.Context, fileName string) error {
	// Resolve the full path for the file
	filePath, err := m.resolveFilePath(ctx, fileName)
	if err != nil {
		return fmt.Errorf("failed to resolve file path: %w", err)
	}

	err = m.client.RemoveObject(ctx, m.bucketName, filePath, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

func (m *MinioMediaStorage) GetURL(ctx context.Context, fileName string) (string, error) {
	// Resolve the full path for the file
	filePath, err := m.resolveFilePath(ctx, fileName)
	if err != nil {
		return "", fmt.Errorf("failed to resolve file path: %w", err)
	}

	// Check if object exists
	_, err = m.client.StatObject(ctx, m.bucketName, filePath, minio.StatObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get object stats: %w", err)
	}

	if m.publicURL != "" {
		return m.publicURL + "/" + filePath, nil
	}

	// Gerando URL assinada com validade de 7 dias
	reqParams := make(url.Values)
	presignedURL, err := m.client.PresignedGetObject(ctx, m.bucketName, filePath, time.Hour*24*7, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL.String(), nil
}
