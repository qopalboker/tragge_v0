// Package config provides environment variable validation utilities for
// production readiness. Services use these functions at startup to fail fast
// when critical configuration is missing in production or staging environments.
package config

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// IsProduction returns true unless ENVIRONMENT is explicitly set to a
// non-production value (development, local, test). An empty/unset
// ENVIRONMENT is treated as production for safety.
func IsProduction() bool {
	env := strings.ToLower(os.Getenv("ENVIRONMENT"))
	if env == "" {
		log.Println("WARNING: ENVIRONMENT not set, defaulting to production security mode")
		return true
	}
	return env != "development" && env != "local" && env != "test"
}

// ValidateRequired checks that all specified environment variables are set
// (non-empty). Returns a slice of missing variable names, or nil if all
// are present.
func ValidateRequired(keys ...string) []string {
	var missing []string
	for _, key := range keys {
		if os.Getenv(key) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

// ValidateAnyRequired checks that at least one of the specified environment
// variables is set. Returns true if at least one is set.
func ValidateAnyRequired(keys ...string) bool {
	for _, key := range keys {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}

// MustBeSet validates that all specified environment variables are set in
// production or staging environments. If any are missing, it panics with
// the missing variable names.
//
// In development (or when ENVIRONMENT is unset), this is a no-op to allow
// services to start with default values.
func MustBeSet(vars ...string) {
	if !IsProduction() {
		return
	}
	missing := ValidateRequired(vars...)
	if len(missing) == 0 {
		return
	}
	panic(fmt.Sprintf("FATAL: missing required environment variables: %s",
		strings.Join(missing, ", ")))
}

// MustBeSetAny validates that at least one of the specified environment
// variables is set in production or staging environments. The label parameter
// describes the configuration group in the error message.
//
// This is useful when a configuration value can come from multiple sources,
// e.g., POSTGRES_DSN or POSTGRES_HOST for database connection.
func MustBeSetAny(label string, keys ...string) {
	if !IsProduction() {
		return
	}
	if ValidateAnyRequired(keys...) {
		return
	}
	panic(fmt.Sprintf("FATAL: %s not configured (set one of: %s)",
		label, strings.Join(keys, ", ")))
}
