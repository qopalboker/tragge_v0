package infra

import (
	"crypto/tls"
	"log"

	"github.com/Parsaeffatravesh/tragge/packages/config"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
)

// KafkaSecurityOpts returns kgo.Opt values for SASL and TLS based on
// environment variables. Returns nil when no SASL mechanism is configured.
// Call once at startup and append the result to every kgo.NewClient opts slice.
func KafkaSecurityOpts() []kgo.Opt {
	cfg := config.LoadKafkaSecurityConfig()
	var opts []kgo.Opt

	if cfg.Enabled() {
		switch cfg.Mechanism {
		case "PLAIN":
			opts = append(opts, kgo.SASL(
				plain.Auth{User: cfg.Username, Pass: cfg.Password}.AsMechanism(),
			))
		case "SCRAM-SHA-256":
			opts = append(opts, kgo.SASL(
				scram.Auth{User: cfg.Username, Pass: cfg.Password}.AsSha256Mechanism(),
			))
		case "SCRAM-SHA-512":
			opts = append(opts, kgo.SASL(
				scram.Auth{User: cfg.Username, Pass: cfg.Password}.AsSha512Mechanism(),
			))
		default:
			log.Printf("WARNING: unknown KAFKA_SASL_MECHANISM %q, skipping SASL", cfg.Mechanism)
		}
	}

	if cfg.TLS {
		opts = append(opts, kgo.DialTLSConfig(&tls.Config{}))
	}

	return opts
}

// NewKafkaOpts is a convenience that prepends SeedBrokers and appends security opts.
// Use this for simple clients; for clients with custom opts, append KafkaSecurityOpts() manually.
func NewKafkaOpts(brokers []string, extraOpts ...kgo.Opt) []kgo.Opt {
	opts := []kgo.Opt{kgo.SeedBrokers(brokers...)}
	opts = append(opts, KafkaSecurityOpts()...)
	opts = append(opts, extraOpts...)
	return opts
}
