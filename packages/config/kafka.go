package config

import "os"

// KafkaSecurityConfig holds optional Kafka SASL/TLS settings loaded
// from environment variables. When Mechanism is empty, no SASL
// authentication is configured (suitable for development).
//
// Environment variables:
//   - KAFKA_SASL_MECHANISM: "PLAIN", "SCRAM-SHA-256", or "SCRAM-SHA-512"
//   - KAFKA_SASL_USERNAME: SASL username
//   - KAFKA_SASL_PASSWORD: SASL password (use Docker secrets in production)
//   - KAFKA_TLS_ENABLED:   "true" to enable TLS
type KafkaSecurityConfig struct {
	Mechanism string // SASL mechanism: PLAIN, SCRAM-SHA-256, SCRAM-SHA-512
	Username  string // SASL username
	Password  string // SASL password
	TLS       bool   // Whether to use TLS
}

// LoadKafkaSecurityConfig reads Kafka security settings from environment
// variables. Returns a zero-value config when no SASL mechanism is set.
func LoadKafkaSecurityConfig() KafkaSecurityConfig {
	return KafkaSecurityConfig{
		Mechanism: GetEnv("KAFKA_SASL_MECHANISM", ""),
		Username:  GetEnv("KAFKA_SASL_USERNAME", ""),
		Password:  os.Getenv("KAFKA_SASL_PASSWORD"),
		TLS:       GetEnv("KAFKA_TLS_ENABLED", "false") == "true",
	}
}

// Enabled returns true if SASL authentication is configured.
func (c KafkaSecurityConfig) Enabled() bool {
	return c.Mechanism != ""
}
