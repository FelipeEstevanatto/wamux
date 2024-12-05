package minio_storage

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"time"

	storage_interfaces "github.com/EvolutionAPI/evolution-go/pkg/storage/interfaces"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioMediaStorage struct {
	client     *minio.Client
	bucketName string
	baseURL    string
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

func NewMinioMediaStorage(
	endpoint,
	accessKeyID,
	secretAccessKey,
	bucketName,
	region string,
	useSSL bool,
) (storage_interfaces.MediaStorage, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
		Region: region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// Set bucket policy to allow public access
	err = setBucketPolicy(client, bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to set bucket policy: %w", err)
	}

	baseURL := fmt.Sprintf("https://%s/%s", endpoint, bucketName)
	if !useSSL {
		baseURL = fmt.Sprintf("http://%s/%s", endpoint, bucketName)
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

	// Gerando URL assinada com validade de 100 anos
	reqParams := make(url.Values)
	presignedURL, err := m.client.PresignedGetObject(ctx, m.bucketName, fileName, time.Hour*24*7, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	fmt.Println(presignedURL.String())

	return presignedURL.String(), nil
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

	// Gerando URL assinada com validade de 100 anos
	reqParams := make(url.Values)
	presignedURL, err := m.client.PresignedGetObject(ctx, m.bucketName, fileName, time.Hour*24*7, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	fmt.Println(presignedURL.String())

	return presignedURL.String(), nil
}
