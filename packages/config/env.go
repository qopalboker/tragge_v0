package config

import (
	"os"
	"strconv"
	"strings"
)

// GetEnv returns the value of the environment variable named by key,
// or defaultValue if the variable is empty or unset.
func GetEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// GetEnvString is an alias for GetEnv. Some services use this name.
func GetEnvString(key, defaultValue string) string {
	return GetEnv(key, defaultValue)
}

// GetEnvInt returns the value of the environment variable parsed as an int,
// or defaultValue if the variable is empty, unset, or cannot be parsed.
func GetEnvInt(key string, defaultValue int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultValue
	}
	return n
}

// GetEnvInt64 returns the value of the environment variable parsed as an int64,
// or defaultValue if the variable is empty, unset, or cannot be parsed.
func GetEnvInt64(key string, defaultValue int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return defaultValue
	}
	return n
}

// GetEnvBool returns the value of the environment variable parsed as a bool,
// or defaultValue if the variable is empty, unset, or cannot be parsed.
// Accepts "true", "1" (case-insensitive) as true values.
func GetEnvBool(key string, defaultValue bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "true" || v == "1" {
		return true
	}
	if v == "false" || v == "0" {
		return false
	}
	return defaultValue
}

// SetDefault sets the environment variable named by key to val
// only if the variable is currently empty or unset.
func SetDefault(key, val string) {
	if os.Getenv(key) == "" {
		os.Setenv(key, val)
	}
}
