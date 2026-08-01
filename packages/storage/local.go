package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// LocalStore implements ObjectStore using the local filesystem.
// Suitable for development and testing. For production, use S3Store.
type LocalStore struct {
	basePath  string
	publicURL string
}

// NewLocalStore creates a local filesystem-backed object store.
// basePath is the root directory for uploads (e.g., "./data/uploads").
// publicURL is the URL prefix for serving files (e.g., "http://localhost:8080/uploads").
func NewLocalStore(basePath, publicURL string) (*LocalStore, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("storage: failed to create base path %s: %w", basePath, err)
	}
	return &LocalStore{
		basePath:  basePath,
		publicURL: strings.TrimRight(publicURL, "/"),
	}, nil
}

func (s *LocalStore) safePath(bucket, key string) (string, error) {
	resolved := filepath.Clean(filepath.Join(s.basePath, bucket, key))
	if !strings.HasPrefix(resolved, filepath.Clean(s.basePath)+string(os.PathSeparator)) {
		return "", fmt.Errorf("storage: invalid path traversal detected")
	}
	return resolved, nil
}

func (s *LocalStore) Upload(_ context.Context, bucket, key string, reader io.Reader, _ int64, _ string) (string, error) {
	path, err := s.safePath(bucket, key)
	if err != nil {
		return "", err
	}

	// Ensure subdirectories exist (key may contain slashes)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("storage: mkdir failed for %s: %w", filepath.Dir(path), err)
	}

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("storage: create file failed for %s: %w", path, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, reader); err != nil {
		return "", fmt.Errorf("storage: write failed for %s: %w", path, err)
	}

	return s.URL(bucket, key), nil
}

func (s *LocalStore) Download(_ context.Context, bucket, key string) (io.ReadCloser, string, int64, error) {
	path, err := s.safePath(bucket, key)
	if err != nil {
		return nil, "", 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", 0, fmt.Errorf("storage: open failed for %s: %w", path, err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, "", 0, fmt.Errorf("storage: stat failed for %s: %w", path, err)
	}

	// Detect content type from first 512 bytes
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	contentType := http.DetectContentType(buf[:n])
	// Seek back to start
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, "", 0, fmt.Errorf("storage: seek failed for %s: %w", path, err)
	}

	return f, contentType, info.Size(), nil
}

func (s *LocalStore) Delete(_ context.Context, bucket, key string) error {
	path, err := s.safePath(bucket, key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: delete failed for %s: %w", path, err)
	}
	return nil
}

func (s *LocalStore) URL(bucket, key string) string {
	return fmt.Sprintf("%s/%s/%s", s.publicURL, bucket, key)
}
