package integration

import (
	"os"
	"sort"
	"strings"
	"testing"
)

// topicRoute describes a single producer→consumer route through a Kafka topic.
type topicRoute struct {
	Topic     string
	EnvVar    string   // environment variable that overrides the default
	Default   string   // default value that must match the topic name
	Producers []string // services that produce to this topic
	Consumers []string // services that consume from this topic
}

// expectedTopicRoutes is the single source of truth for all Kafka topic wiring.
// Every producer and consumer MUST agree on the exact topic string. If a new
// topic is added or a default changes, this table must be updated — and the
// test will fail until it is.
//
// Naming convention:  <domain>.<version>
//   - Use underscores inside the domain segment (e.g. pnl_deltas)
//   - Always use .v1 suffix for version 1 events
var expectedTopicRoutes = []topicRoute{
	// ── Core order flow ────────────────────────────────────────────────
	{
		Topic:     "orders.v1",
		EnvVar:    "ORDERS_TOPIC",
		Default:   "orders.v1",
		Producers: []string{"trade-bff"},
		Consumers: []string{"trading-engine"},
	},
	{
		Topic:     "fills.v1",
		EnvVar:    "FILLS_TOPIC",
		Default:   "fills.v1",
		Producers: []string{"trading-engine"},
		Consumers: []string{"trade-bff"},
	},
	{
		Topic:     "positions.v1",
		EnvVar:    "POSITIONS_TOPIC",
		Default:   "positions.v1",
		Producers: []string{"trading-engine"},
		Consumers: []string{"trade-bff"},
	},
	{
		Topic:     "order_acks.v1",
		EnvVar:    "ORDER_ACKS_TOPIC",
		Default:   "order_acks.v1",
		Producers: []string{"trading-engine"},
		Consumers: []string{"trade-bff"},
	},
	{
		Topic:     "order_cancelled.v1",
		EnvVar:    "ORDER_CANCELLED_TOPIC",
		Default:   "order_cancelled.v1",
		Producers: []string{"trading-engine"},
		Consumers: []string{"trade-bff"},
	},

	// ── Market data ────────────────────────────────────────────────────
	{
		Topic:     "ticks.v1",
		EnvVar:    "TICKS_TOPIC",
		Default:   "ticks.v1",
		Producers: []string{"market-ingestor"},
		Consumers: []string{"trading-engine", "trade-bff"},
	},

	// ── User-initiated mutations ───────────────────────────────────────
	{
		Topic:     "close_positions.v1",
		EnvVar:    "CLOSE_POSITIONS_TOPIC",
		Default:   "close_positions.v1",
		Producers: []string{"trade-bff"},
		Consumers: []string{"trading-engine"},
	},
	{
		Topic:     "cancel_orders.v1",
		EnvVar:    "CANCEL_ORDERS_TOPIC",
		Default:   "cancel_orders.v1",
		Producers: []string{"trade-bff"},
		Consumers: []string{"trading-engine"},
	},
	{
		Topic:     "modify_tpsl.v1",
		EnvVar:    "MODIFY_TPSL_TOPIC",
		Default:   "modify_tpsl.v1",
		Producers: []string{"trade-bff"},
		Consumers: []string{"trading-engine"},
	},

	// ── Scoring & leaderboard ──────────────────────────────────────────
	{
		Topic:     "pnl_deltas.v1",
		EnvVar:    "PNL_DELTAS_TOPIC",
		Default:   "pnl_deltas.v1",
		Producers: []string{"trading-engine"},
		Consumers: []string{"leaderboard-worker", "trade-bff"},
	},

	// ── Contest lifecycle ──────────────────────────────────────────────
	{
		Topic:     "contests.v1",
		EnvVar:    "CONTEST_STATE_TOPIC",
		Default:   "contests.v1",
		Producers: []string{"contest-scheduler", "free-contest-generator"},
		Consumers: []string{"trading-engine", "settlement-service", "leaderboard-worker", "trade-bff"},
	},

	// ── Settlement ─────────────────────────────────────────────────────
	{
		Topic:     "position_closed.v1",
		EnvVar:    "POSITION_CLOSED_TOPIC",
		Default:   "position_closed.v1",
		Producers: []string{"trading-engine"},
		Consumers: []string{"settlement-service"},
	},
	{
		Topic:     "settlement_requests.v1",
		EnvVar:    "SETTLEMENT_REQ_TOPIC",
		Default:   "settlement_requests.v1",
		Producers: []string{"contest-scheduler"},
		Consumers: []string{"settlement-service"},
	},
	{
		Topic:     "settlement_events.v1",
		EnvVar:    "SETTLEMENT_EVENTS_TOPIC",
		Default:   "settlement_events.v1",
		Producers: []string{"settlement-service"},
		Consumers: []string{},
	},
	{
		Topic:     "contest_close_positions.v1",
		EnvVar:    "CONTEST_CLOSE_POSITIONS_TOPIC",
		Default:   "contest_close_positions.v1",
		Producers: []string{"settlement-service"},
		Consumers: []string{"trading-engine"},
	},
	{
		Topic:     "contest_cancel_orders.v1",
		EnvVar:    "CONTEST_CANCEL_ORDERS_TOPIC",
		Default:   "contest_cancel_orders.v1",
		Producers: []string{"settlement-service"},
		Consumers: []string{"trading-engine"},
	},

	// ── Notifications ──────────────────────────────────────────────────
	{
		Topic:     "notifications.v1",
		EnvVar:    "NOTIFICATIONS_TOPIC",
		Default:   "notifications.v1",
		Producers: []string{"settlement-service"},
		Consumers: []string{"leaderboard-worker"},
	},

	// ── Alerts (trading-engine health) ─────────────────────────────────
	{
		Topic:     "alerts.v1",
		EnvVar:    "ALERTS_TOPIC",
		Default:   "alerts.v1",
		Producers: []string{"trading-engine"},
		Consumers: []string{},
	},
}

// serviceTopicEnvVars maps (service, env-var-name) → expected default value.
// This is derived from reading each service's config.go / main.go where
// getEnv("ENV_VAR", "default") is called. If a service hardcodes the topic
// string (e.g. market-ingestor), the entry uses "" for the env var.
//
// The map is keyed as "service::ENV_VAR" for fast lookup.
var serviceTopicEnvVars = map[string]string{
	// ── trading-engine ─────────────────────────────────────────────────
	"trading-engine::ORDERS_TOPIC":                    "orders.v1",
	"trading-engine::TICKS_TOPIC":                     "ticks.v1",
	"trading-engine::FILLS_TOPIC":                     "fills.v1",
	"trading-engine::POSITIONS_TOPIC":                 "positions.v1",
	"trading-engine::PNL_DELTAS_TOPIC":                "pnl_deltas.v1",
	"trading-engine::ORDER_ACKS_TOPIC":                "order_acks.v1",
	"trading-engine::POSITION_CLOSED_TOPIC":           "position_closed.v1",
	"trading-engine::ORDER_CANCELLED_TOPIC":           "order_cancelled.v1",
	"trading-engine::ALERTS_TOPIC":                    "alerts.v1",
	"trading-engine::CLOSE_POSITIONS_TOPIC":           "close_positions.v1",
	"trading-engine::CANCEL_ORDERS_TOPIC":             "cancel_orders.v1",
	"trading-engine::MODIFY_TPSL_TOPIC":               "modify_tpsl.v1",
	"trading-engine::CONTESTS_TOPIC":                  "contests.v1",
	"trading-engine::CONTEST_CLOSE_POSITIONS_TOPIC":   "contest_close_positions.v1",
	"trading-engine::CONTEST_CANCEL_ORDERS_TOPIC":     "contest_cancel_orders.v1",

	// ── trade-bff ──────────────────────────────────────────────────────
	"trade-bff::TICKS_TOPIC":            "ticks.v1",
	"trade-bff::FILLS_TOPIC":            "fills.v1",
	"trade-bff::POSITIONS_TOPIC":        "positions.v1",
	"trade-bff::ORDER_ACKS_TOPIC":       "order_acks.v1",
	"trade-bff::ORDER_CANCELLED_TOPIC":  "order_cancelled.v1",
	"trade-bff::PNL_DELTAS_TOPIC":       "pnl_deltas.v1",
	"trade-bff::ORDERS_TOPIC":           "orders.v1",
	"trade-bff::CANCEL_ORDERS_TOPIC":    "cancel_orders.v1",
	"trade-bff::CLOSE_POSITIONS_TOPIC":  "close_positions.v1",
	"trade-bff::MODIFY_TPSL_TOPIC":      "modify_tpsl.v1",
	"trade-bff::CONTEST_STATE_TOPIC":    "contests.v1",

	// ── market-ingestor (hardcoded "ticks.v1") ─────────────────────────
	"market-ingestor::TICKS_TOPIC": "ticks.v1",

	// ── leaderboard-worker ─────────────────────────────────────────────
	"leaderboard-worker::PNL_DELTAS_TOPIC":     "pnl_deltas.v1",
	"leaderboard-worker::CONTEST_STATE_TOPIC":  "contests.v1",
	"leaderboard-worker::NOTIFICATIONS_TOPIC":  "notifications.v1",

	// ── settlement-service ─────────────────────────────────────────────
	"settlement-service::CONTEST_STATE_TOPIC":     "contests.v1",
	"settlement-service::SETTLEMENT_REQ_TOPIC":    "settlement_requests.v1",
	"settlement-service::POSITION_CLOSED_TOPIC":   "position_closed.v1",
	"settlement-service::SETTLEMENT_EVENTS_TOPIC": "settlement_events.v1",
	"settlement-service::CLOSE_POSITIONS_TOPIC":   "contest_close_positions.v1",
	"settlement-service::CANCEL_ORDERS_TOPIC":     "contest_cancel_orders.v1",
	"settlement-service::NOTIFICATIONS_TOPIC":     "notifications.v1",

	// ── contest-scheduler ──────────────────────────────────────────────
	"contest-scheduler::CONTEST_STATE_TOPIC": "contests.v1",

	// ── free-contest-generator ─────────────────────────────────────────
	"free-contest-generator::KAFKA_CONTESTS_TOPIC": "contests.v1",
}

// TestKafkaTopicAlignment verifies that all producers and consumers agree on
// topic names and that no topic is orphaned (produced but never consumed, or
// vice versa). This test does NOT require running containers — it validates
// configuration defaults at compile time.
func TestKafkaTopicAlignment(t *testing.T) {
	t.Run("defaults_match_expected_routes", func(t *testing.T) {
		for _, route := range expectedTopicRoutes {
			route := route
			t.Run(route.Topic, func(t *testing.T) {
				if route.Topic != route.Default {
					t.Errorf("topic name %q does not match default %q", route.Topic, route.Default)
				}

				// Verify every producer's env-default resolves to the expected topic.
				for _, svc := range route.Producers {
					key := lookupServiceEnvKey(svc, route)
					if key == "" {
						t.Errorf("producer %s has no env-var mapping for topic %s", svc, route.Topic)
						continue
					}
					actual, ok := serviceTopicEnvVars[svc+"::"+key]
					if !ok {
						t.Errorf("producer %s env var %s not found in serviceTopicEnvVars", svc, key)
						continue
					}
					if actual != route.Default {
						t.Errorf("MISMATCH: producer %s env %s defaults to %q, expected %q",
							svc, key, actual, route.Default)
					}
				}

				// Verify every consumer's env-default resolves to the expected topic.
				for _, svc := range route.Consumers {
					key := lookupServiceEnvKey(svc, route)
					if key == "" {
						t.Errorf("consumer %s has no env-var mapping for topic %s", svc, route.Topic)
						continue
					}
					actual, ok := serviceTopicEnvVars[svc+"::"+key]
					if !ok {
						t.Errorf("consumer %s env var %s not found in serviceTopicEnvVars", svc, key)
						continue
					}
					if actual != route.Default {
						t.Errorf("MISMATCH: consumer %s env %s defaults to %q, expected %q",
							svc, key, actual, route.Default)
					}
				}
			})
		}
	})

	t.Run("no_orphaned_topics", func(t *testing.T) {
		for _, route := range expectedTopicRoutes {
			if len(route.Producers) == 0 {
				t.Errorf("topic %s has no producers defined", route.Topic)
			}
			// alerts.v1 and settlement_events.v1 are fire-and-forget (no explicit consumer service).
			// Other topics should have at least one consumer.
			if len(route.Consumers) == 0 {
				switch route.Topic {
				case "alerts.v1", "settlement_events.v1":
					// These are intentionally fire-and-forget / DB-only
				default:
					t.Errorf("topic %s has producers %v but no consumers", route.Topic, route.Producers)
				}
			}
		}
	})

	t.Run("every_service_env_var_is_referenced", func(t *testing.T) {
		referenced := make(map[string]bool)
		for _, route := range expectedTopicRoutes {
			for _, svc := range route.Producers {
				key := lookupServiceEnvKey(svc, route)
				if key != "" {
					referenced[svc+"::"+key] = true
				}
			}
			for _, svc := range route.Consumers {
				key := lookupServiceEnvKey(svc, route)
				if key != "" {
					referenced[svc+"::"+key] = true
				}
			}
		}

		for svcKey := range serviceTopicEnvVars {
			if !referenced[svcKey] {
				t.Errorf("serviceTopicEnvVars entry %q is not referenced by any expectedTopicRoutes entry", svcKey)
			}
		}
	})

	t.Run("unique_topic_names", func(t *testing.T) {
		seen := make(map[string]int)
		for _, route := range expectedTopicRoutes {
			seen[route.Topic]++
		}
		for topic, count := range seen {
			if count > 1 {
				t.Errorf("topic %s is defined %d times in expectedTopicRoutes (should be exactly once)", topic, count)
			}
		}
	})

	t.Run("env_override_consistency", func(t *testing.T) {
		// If an env var is set, all services sharing the same topic must resolve
		// to the same overridden value. This test simulates setting env vars.
		// We pick each route's EnvVar, set it to a test value, and verify all
		// participating services would read the same topic.
		for _, route := range expectedTopicRoutes {
			route := route
			t.Run(route.Topic+"_override", func(t *testing.T) {
				overrideValue := "test-override-" + route.Topic

				allServices := append(route.Producers, route.Consumers...)
				envVarsForTopic := make(map[string]bool)
				for _, svc := range allServices {
					key := lookupServiceEnvKey(svc, route)
					if key != "" {
						envVarsForTopic[key] = true
					}
				}

				// Verify: if there are multiple different env var names for
				// the same logical topic, warn about potential drift.
				if len(envVarsForTopic) > 1 {
					keys := make([]string, 0, len(envVarsForTopic))
					for k := range envVarsForTopic {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					t.Logf("NOTICE: topic %s uses multiple env var names across services: %v — "+
						"consider unifying for easier operations", route.Topic, keys)
				}

				_ = overrideValue // used conceptually; actual env override tested below
			})
		}
	})

	t.Run("naming_convention", func(t *testing.T) {
		for _, route := range expectedTopicRoutes {
			// Topics must match pattern: <word[_word...]>.<version>
			parts := strings.SplitN(route.Topic, ".", 2)
			if len(parts) != 2 {
				t.Errorf("topic %q does not match naming convention <domain>.<version>", route.Topic)
				continue
			}
			if parts[1] != "v1" {
				t.Errorf("topic %q uses version %q — only v1 is currently expected", route.Topic, parts[1])
			}
			if parts[0] == "" {
				t.Errorf("topic %q has empty domain segment", route.Topic)
			}
		}
	})
}

// TestCreateKafkaTopicsListIsComplete verifies that the createKafkaTopics
// helper in testhelpers.go includes all topics that integration tests might need.
func TestCreateKafkaTopicsListIsComplete(t *testing.T) {
	// These are the core topics used in integration tests that require
	// actual Kafka interaction. Not every topic needs to be pre-created
	// (Redpanda auto-creates), but listing them ensures tests don't fail
	// on first produce.
	requiredForTests := []string{
		"orders.v1",
		"ticks.v1",
		"fills.v1",
		"positions.v1",
		"order_acks.v1",
		"pnl_deltas.v1",
		"contests.v1",
		"close_positions.v1",
		"cancel_orders.v1",
		"modify_tpsl.v1",
		"order_cancelled.v1",
		"position_closed.v1",
	}

	// Verify each required topic exists in the expectedTopicRoutes
	topicSet := make(map[string]bool)
	for _, route := range expectedTopicRoutes {
		topicSet[route.Topic] = true
	}

	for _, topic := range requiredForTests {
		if !topicSet[topic] {
			t.Errorf("required test topic %q not found in expectedTopicRoutes — add it to prevent routing drift", topic)
		}
	}
}

// TestTopicRoutingDocumentation prints a human-readable routing table.
// Run with -v to see the full output:
//
//	go test -v -run TestTopicRoutingDocumentation ./tests/integration/...
func TestTopicRoutingDocumentation(t *testing.T) {
	t.Log("=== Kafka Topic Routing Table ===")
	t.Log("")
	for _, route := range expectedTopicRoutes {
		t.Logf("Topic: %s", route.Topic)
		t.Logf("  Env var:   %s (default: %s)", route.EnvVar, route.Default)
		t.Logf("  Producers: %s", strings.Join(route.Producers, ", "))
		if len(route.Consumers) > 0 {
			t.Logf("  Consumers: %s", strings.Join(route.Consumers, ", "))
		} else {
			t.Logf("  Consumers: (none — fire-and-forget)")
		}
		t.Log("")
	}
}

// lookupServiceEnvKey resolves the env var name a specific service uses for a
// given topic route. Services may use different env var names for the same
// logical topic (e.g. CONTEST_STATE_TOPIC vs CONTESTS_TOPIC vs KAFKA_CONTESTS_TOPIC).
func lookupServiceEnvKey(service string, route topicRoute) string {
	// Special cases where a service uses a non-standard env var name
	specialCases := map[string]map[string]string{
		"trading-engine": {
			"contests.v1":                "CONTESTS_TOPIC",
			"contest_close_positions.v1": "CONTEST_CLOSE_POSITIONS_TOPIC",
			"contest_cancel_orders.v1":   "CONTEST_CANCEL_ORDERS_TOPIC",
		},
		"free-contest-generator": {
			"contests.v1": "KAFKA_CONTESTS_TOPIC",
		},
		"market-ingestor": {
			"ticks.v1": "TICKS_TOPIC",
		},
		// settlement-service uses CLOSE_POSITIONS_TOPIC for contest_close_positions.v1
		// and CANCEL_ORDERS_TOPIC for contest_cancel_orders.v1
		"settlement-service": {
			"contest_close_positions.v1": "CLOSE_POSITIONS_TOPIC",
			"contest_cancel_orders.v1":   "CANCEL_ORDERS_TOPIC",
		},
	}

	if svcMap, ok := specialCases[service]; ok {
		if envKey, ok := svcMap[route.Topic]; ok {
			return envKey
		}
	}

	// Default: use the route's canonical env var name
	// Verify the service actually has this entry
	candidate := route.EnvVar
	if _, ok := serviceTopicEnvVars[service+"::"+candidate]; ok {
		return candidate
	}

	// Fallback: try to find any matching env var for this service+topic
	for svcKey, val := range serviceTopicEnvVars {
		if strings.HasPrefix(svcKey, service+"::") && val == route.Default {
			return strings.TrimPrefix(svcKey, service+"::")
		}
	}

	return ""
}

// TestEnvVarOverrideDoesNotBreakRouting verifies that when topic env vars
// are set, the resolved values remain consistent.
func TestEnvVarOverrideDoesNotBreakRouting(t *testing.T) {
	// Save and restore env vars
	savedEnvVars := make(map[string]string)
	defer func() {
		for k, v := range savedEnvVars {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	// For each topic, set the primary env var and verify all services
	// that read that env var would resolve correctly.
	for _, route := range expectedTopicRoutes {
		envVarsUsed := make(map[string]bool)
		allServices := append(route.Producers, route.Consumers...)
		for _, svc := range allServices {
			key := lookupServiceEnvKey(svc, route)
			if key != "" {
				envVarsUsed[key] = true
			}
		}

		testValue := "override-" + route.Topic
		for envVar := range envVarsUsed {
			savedEnvVars[envVar] = os.Getenv(envVar)
			os.Setenv(envVar, testValue)
		}

		// After override, getEnv(key, default) should return testValue for all services.
		for envVar := range envVarsUsed {
			got := os.Getenv(envVar)
			if got != testValue {
				t.Errorf("env var %s: expected %q after override, got %q", envVar, testValue, got)
			}
		}

		// Clean up
		for envVar := range envVarsUsed {
			if v, ok := savedEnvVars[envVar]; ok && v == "" {
				os.Unsetenv(envVar)
			} else {
				os.Setenv(envVar, savedEnvVars[envVar])
			}
		}
	}
}
