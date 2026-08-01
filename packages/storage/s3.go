package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Store implements ObjectStore using S3/MinIO.
type S3Store struct {
	client *minio.Client
	config Config
}

// NewS3Store creates a new S3-compatible object store client.
// It ensures the configured bucket exists, creating it if necessary.
func NewS3Store(ctx context.Context, cfg Config) (*S3Store, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: failed to create S3 client: %w", err)
	}

	// Ensure bucket exists
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("storage: failed to check bucket %s: %w", cfg.Bucket, err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{Region: cfg.Region}); err != nil {
			return nil, fmt.Errorf("storage: failed to create bucket %s: %w", cfg.Bucket, err)
		}
		if !cfg.Private {
			// Set public read policy so avatars are accessible without authentication
			policy := fmt.Sprintf(`{
				"Version": "2012-10-17",
				"Statement": [{
					"Effect": "Allow",
					"Principal": {"AWS": ["*"]},
					"Action": ["s3:GetObject"],
					"Resource": ["arn:aws:s3:::%s/*"]
				}]
			}`, cfg.Bucket)
			if err := client.SetBucketPolicy(ctx, cfg.Bucket, policy); err != nil {
				return nil, fmt.Errorf("storage: failed to set bucket policy: %w", err)
			}
		}
	}

	return &S3Store{client: client, config: cfg}, nil
}

// Upload stores an object and returns its public URL.
func (s *S3Store) Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) (string, error) {
	opts := minio.PutObjectOptions{
		ContentType: contentType,
	}
	_, err := s.client.PutObject(ctx, bucket, key, reader, size, opts)
	if err != nil {
		return "", fmt.Errorf("storage: upload failed for %s/%s: %w", bucket, key, err)
	}
	return s.URL(bucket, key), nil
}

// Delete removes an object from storage.
func (s *S3Store) Delete(ctx context.Context, bucket, key string) error {
	err := s.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("storage: delete failed for %s/%s: %w", bucket, key, err)
	}
	return nil
}

// Download retrieves an object from storage and returns its content reader,
// content type, and size. The caller must close the returned reader.
func (s *S3Store) Download(ctx context.Context, bucket, key string) (io.ReadCloser, string, int64, error) {
	obj, err := s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", 0, fmt.Errorf("storage: download failed for %s/%s: %w", bucket, key, err)
	}
	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, "", 0, fmt.Errorf("storage: stat failed for %s/%s: %w", bucket, key, err)
	}
	return obj, info.ContentType, info.Size, nil
}

// URL returns the public URL for an object.
func (s *S3Store) URL(bucket, key string) string {
	if s.config.PublicURL != "" {
		return fmt.Sprintf("%s/%s", strings.TrimRight(s.config.PublicURL, "/"), key)
	}
	scheme := "http"
	if s.config.UseSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/%s/%s", scheme, s.config.Endpoint, bucket, key)
}
