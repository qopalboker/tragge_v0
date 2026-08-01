package server

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsRetryableDBError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"context canceled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, false},
		{
			"deadlock detected (40P01)",
			&pgconn.PgError{Code: "40P01", Message: "deadlock detected"},
			true,
		},
		{
			"serialization failure (40001)",
			&pgconn.PgError{Code: "40001", Message: "could not serialize access"},
			true,
		},
		{
			"wrapped deadlock",
			fmt.Errorf("failed to credit: %w", &pgconn.PgError{Code: "40P01", Message: "deadlock detected"}),
			true,
		},
		{
			"unique violation (23505) - not retryable",
			&pgconn.PgError{Code: "23505", Message: "duplicate key value"},
			false,
		},
		{
			"generic error",
			errors.New("connection refused"),
			false,
		},
		{
			"wrapped context canceled",
			fmt.Errorf("operation failed: %w", context.Canceled),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableDBError(tt.err)
			if result != tt.expected {
				t.Errorf("isRetryableDBError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}
