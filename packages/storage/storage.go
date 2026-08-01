package storage

import (
	"context"
	"io"
)

// ObjectStore defines the interface for object storage operations.
// Implementations include S3Store (for S3/MinIO).
type ObjectStore interface {
	// Upload stores an object and returns its public URL.
	Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) (string, error)

	// Download retrieves an object from storage and returns its content reader,
	// content type, and size. The caller must close the returned reader.
	Download(ctx context.Context, bucket, key string) (io.ReadCloser, string, int64, error)

	// Delete removes an object from storage.
	Delete(ctx context.Context, bucket, key string) error

	// URL returns the public URL for an object.
	URL(bucket, key string) string
}

// Config holds S3/MinIO connection configuration.
type Config struct {
	Endpoint       string // e.g., "minio:9000" (dev) or "s3.amazonaws.com" (prod)
	AccessKeyID    string
	SecretAccessKey string
	Region         string // e.g., "us-east-1"
	Bucket         string // e.g., "tragge-avatars"
	UseSSL         bool   // false for local MinIO, true for AWS S3
	PublicURL      string // Optional CDN/public URL prefix, e.g., "https://cdn.tragge.com"
	Private        bool   // If true, bucket will NOT get a public read policy (use for sensitive data like KYC)
}
