package storage

import (
	"context"
	"os"
	"strings"
)

// New creates the appropriate ObjectStore based on environment.
// In development/test: uses local filesystem (no MinIO needed).
// In production/staging: uses S3/MinIO.
func New(ctx context.Context, cfg Config) (ObjectStore, error) {
	env := strings.ToLower(os.Getenv("ENVIRONMENT"))

	// Use local storage when explicitly configured or in development/test
	if os.Getenv("STORAGE_BACKEND") == "local" || (cfg.Endpoint == "" && (env == "development" || env == "test" || env == "")) {
		basePath := os.Getenv("STORAGE_LOCAL_PATH")
		if basePath == "" {
			basePath = "./data/uploads"
		}
		publicURL := cfg.PublicURL
		if publicURL == "" {
			publicURL = "http://localhost:8080/uploads"
		}
		return NewLocalStore(basePath, publicURL)
	}

	return NewS3Store(ctx, cfg)
}

// BackendName returns "local" or "s3" for an initialized ObjectStore. Useful
// for log lines that want to record which backend the factory selected.
func BackendName(s ObjectStore) string {
	if _, ok := s.(*LocalStore); ok {
		return "local"
	}
	return "s3"
}
