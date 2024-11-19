package minio_storage

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	storage_interfaces "github.com/EvolutionAPI/evolution-go/pkg/storage/interfaces"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioMediaStorage struct {
	client     *minio.Client
	bucketName string
	baseURL    string
}

func NewMinioMediaStorage(
	endpoint,
	accessKeyID,
	secretAccessKey,
	bucketName,
	region string,
	useSSL bool,
) (storage_interfaces.MediaStorage, error) {
	// Initialize MinIO client
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// Create bucket if it doesn't exist
	exists, err := client.BucketExists(context.Background(), bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}

	if !exists {
		err = client.MakeBucket(context.Background(), bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	baseURL := fmt.Sprintf("https://%s/%s", endpoint, bucketName)
	if !useSSL {
		baseURL = fmt.Sprintf("http://%s/%s", endpoint, bucketName)
	}

	if region != "" && (strings.Contains(endpoint, "amazonaws.com") || strings.Contains(endpoint, "googlecloud.com")) {
		baseURL = fmt.Sprintf("https://%s.%s/%s", bucketName, endpoint, region)
		if !useSSL {
			baseURL = fmt.Sprintf("http://%s.%s/%s", bucketName, endpoint, region)
		}
	}

	return &MinioMediaStorage{
		client:     client,
		bucketName: bucketName,
		baseURL:    baseURL,
	}, nil
}

func (m *MinioMediaStorage) Store(ctx context.Context, data []byte, fileName string, contentType string) (string, error) {
	reader := bytes.NewReader(data)
	_, err := m.client.PutObject(ctx, m.bucketName, fileName, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("failed to store object: %w", err)
	}

	return fmt.Sprintf("%s/%s", m.baseURL, fileName), nil
}

func (m *MinioMediaStorage) Delete(ctx context.Context, fileName string) error {
	err := m.client.RemoveObject(ctx, m.bucketName, fileName, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

func (m *MinioMediaStorage) GetURL(ctx context.Context, fileName string) (string, error) {
	// Check if object exists
	_, err := m.client.StatObject(ctx, m.bucketName, fileName, minio.StatObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get object stats: %w", err)
	}

	return fmt.Sprintf("%s/%s", m.baseURL, fileName), nil
}
