package db

import (
	"database/sql"
)

// NewPoolFromDB creates a Pool wrapper around an existing sql.DB.
// This is intended for testing purposes only, allowing sqlmock to be used.
// The pool will use the provided db for both primary and replica operations.
//
// Example usage with sqlmock:
//
//	mockDB, mock, _ := sqlmock.New()
//	pool := db.NewPoolFromDB(mockDB)
//	// Use pool in tests...
func NewPoolFromDB(database *sql.DB) *Pool {
	pool := &Pool{
		config:  DefaultConfig(),
		primary: database,
		stopLag: make(chan struct{}),
	}
	return pool
}

// NewPoolFromDBWithConfig creates a Pool wrapper with custom config.
// This is intended for testing purposes only.
func NewPoolFromDBWithConfig(database *sql.DB, cfg Config) *Pool {
	pool := &Pool{
		config:  cfg,
		primary: database,
		stopLag: make(chan struct{}),
	}
	return pool
}
