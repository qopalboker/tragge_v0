package storage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestS3Store_URL(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		bucket   string
		key      string
		expected string
	}{
		{
			name:     "with public URL",
			config:   Config{PublicURL: "https://cdn.tragge.com"},
			bucket:   "avatars",
			key:      "avatars/user123_1234.jpg",
			expected: "https://cdn.tragge.com/avatars/user123_1234.jpg",
		},
		{
			name:     "with public URL trailing slash",
			config:   Config{PublicURL: "https://cdn.tragge.com/"},
			bucket:   "avatars",
			key:      "avatars/user123_1234.jpg",
			expected: "https://cdn.tragge.com/avatars/user123_1234.jpg",
		},
		{
			name:     "without public URL with SSL",
			config:   Config{Endpoint: "s3.amazonaws.com", UseSSL: true},
			bucket:   "tragge-avatars",
			key:      "avatars/user123_1234.jpg",
			expected: "https://s3.amazonaws.com/tragge-avatars/avatars/user123_1234.jpg",
		},
		{
			name:     "without public URL no SSL (MinIO)",
			config:   Config{Endpoint: "localhost:9000"},
			bucket:   "tragge-avatars",
			key:      "avatars/user123_1234.jpg",
			expected: "http://localhost:9000/tragge-avatars/avatars/user123_1234.jpg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &S3Store{config: tt.config}
			got := s.URL(tt.bucket, tt.key)
			if got != tt.expected {
				t.Errorf("URL() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestLocalStore_PathTraversal(t *testing.T) {
	basePath := t.TempDir()
	store, err := NewLocalStore(basePath, "http://localhost:8080/uploads")
	if err != nil {
		t.Fatalf("NewLocalStore failed: %v", err)
	}

	ctx := context.Background()

	t.Run("key with path traversal is rejected", func(t *testing.T) {
		_, err := store.Upload(ctx, "bucket", "../../etc/passwd", bytes.NewReader([]byte("data")), 4, "text/plain")
		if err == nil {
			t.Error("expected error for path traversal key, got nil")
		}
	})

	t.Run("key with deep path traversal is rejected", func(t *testing.T) {
		_, err := store.Upload(ctx, "bucket", "../../../root/.ssh/id_rsa", bytes.NewReader([]byte("data")), 4, "text/plain")
		if err == nil {
			t.Error("expected error for deep path traversal key, got nil")
		}
	})

	t.Run("bucket with path traversal is rejected", func(t *testing.T) {
		_, err := store.Upload(ctx, "../../tmp", "file.txt", bytes.NewReader([]byte("data")), 4, "text/plain")
		if err == nil {
			t.Error("expected error for path traversal bucket, got nil")
		}
	})

	t.Run("download with path traversal is rejected", func(t *testing.T) {
		_, _, _, err := store.Download(ctx, "bucket", "../../etc/passwd")
		if err == nil {
			t.Error("expected error for path traversal download, got nil")
		}
	})

	t.Run("delete with path traversal is rejected", func(t *testing.T) {
		err := store.Delete(ctx, "bucket", "../../etc/passwd")
		if err == nil {
			t.Error("expected error for path traversal delete, got nil")
		}
	})

	t.Run("normal key works", func(t *testing.T) {
		url, err := store.Upload(ctx, "avatars", "user123/photo.jpg", bytes.NewReader([]byte("imgdata")), 7, "image/jpeg")
		if err != nil {
			t.Fatalf("expected no error for normal key, got: %v", err)
		}
		if url != "http://localhost:8080/uploads/avatars/user123/photo.jpg" {
			t.Errorf("unexpected URL: %s", url)
		}

		// Verify file was actually written
		path := filepath.Join(basePath, "avatars", "user123", "photo.jpg")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("file not written: %v", err)
		}
		if string(data) != "imgdata" {
			t.Errorf("unexpected file content: %q", string(data))
		}
	})
}
