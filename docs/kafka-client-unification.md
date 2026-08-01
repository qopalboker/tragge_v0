# Kafka Client Library Unification Plan

## Status: Tech Debt — Planned

This document tracks the dual Kafka client library usage across the tragge platform and outlines a phased migration plan to standardize on a single library.

## Current State

Two different Kafka client libraries are in use:

| Library | Version | Services |
|---------|---------|----------|
| `github.com/IBM/sarama` | v1.43.1 | admin-bff, contest-scheduler, free-contest-generator, statemachine package |
| `github.com/twmb/franz-go` | v1.17.0 | trading-engine, trade-bff, market-ingestor, leaderboard-worker, settlement-service |

### Per-Service Breakdown

| Service | Library | Role | Notes |
|---------|---------|------|-------|
| **trading-engine** | franz-go | Consumer + Producer | High-throughput; consumes `orders.v1`, `ticks.v1`; produces `fills.v1`, `positions.v1`, `pnl.v1`. Uses `kadm` for topic admin. |
| **trade-bff** | franz-go | Consumer + Producer | Publishes order requests to `orders.v1`; consumes fills/positions for WebSocket push. |
| **market-ingestor** | franz-go | Producer | Publishes tick snapshots to `ticks.v1`. High-throughput producer. |
| **leaderboard-worker** | franz-go | Consumer | Consumes `pnl.v1` and `contests.v1` for leaderboard updates. |
| **settlement-service** | franz-go | Consumer + Producer | Consumes events for post-contest settlement processing. |
| **admin-bff** | sarama | Producer | Publishes contest state events via statemachine package. Uses `SyncProducer`. |
| **contest-scheduler** | sarama | Producer | Publishes contest state transitions via statemachine package. Uses `SyncProducer`. |
| **free-contest-generator** | sarama | Producer | Publishes generated contest events. Uses `SyncProducer`. |

### Shared Package

The `packages/statemachine` package defines a `sarama.SyncProducer` interface dependency for publishing contest state events to the `contests.v1` topic. This is the root dependency that pulls sarama into admin-bff and contest-scheduler.

## Problems

1. **Serialization differences** — sarama and franz-go use different default serialization behaviors, which can cause subtle bugs when one library produces and another consumes.
2. **Error handling inconsistency** — Different error types, retry semantics, and failure modes between the two libraries.
3. **Dependency overhead** — Two complete Kafka client dependency trees increase binary sizes and complicate dependency management.
4. **Feature compatibility** — sarama has known issues with newer Kafka/Redpanda features (e.g., cooperative rebalancing, transactional APIs). franz-go has better Redpanda compatibility.
5. **Developer cognitive load** — Contributors need to understand two different APIs for the same underlying protocol.

## Recommendation: Standardize on franz-go

franz-go is the preferred library because:
- Already used by the majority of services (5 out of 8)
- Used by all high-throughput services (trading-engine, market-ingestor)
- Referenced in the [kafka-partitioning.md](./kafka-partitioning.md) documentation
- Better Redpanda compatibility
- More actively maintained with modern Go idioms
- Lower allocation overhead and better performance

## Migration Plan

### Phase A — Document (Now)

- [x] Document which services use which library (this file)
- [x] Map out the statemachine package dependency on sarama

### Phase B — Migrate sarama services to franz-go (Future)

Migrate one service at a time, starting with the simplest:

1. **free-contest-generator** — Standalone producer, no statemachine dependency. Replace `sarama.SyncProducer` with `kgo.Client` + synchronous `Produce`.
2. **admin-bff** — Depends on statemachine; migrate after Phase C or decouple from statemachine producer interface.
3. **contest-scheduler** — Same statemachine dependency as admin-bff.

For each service:
- Replace sarama imports with franz-go equivalents
- Update producer configuration (acks, retries, timeouts)
- Verify message key/value serialization matches existing consumers
- Test against Redpanda in dev environment
- Remove sarama from `go.mod`

### Phase C — Update statemachine package (Future)

The statemachine package is the root cause of sarama usage in admin-bff and contest-scheduler. Two approaches:

**Option 1: Replace sarama interface directly**
- Change `statemachine.Config.KafkaProducer` from `sarama.SyncProducer` to a `kgo.Client`
- Update all call sites in admin-bff and contest-scheduler
- This is a breaking change to the package API

**Option 2: Abstract behind an interface**
- Define a `MessagePublisher` interface in the statemachine package:
  ```go
  type MessagePublisher interface {
      Publish(ctx context.Context, topic string, key, value []byte) error
  }
  ```
- Implement with franz-go
- Removes direct Kafka library coupling from statemachine's public API
- Easier to test (mockable interface)

Option 2 is recommended as it provides a cleaner abstraction and simplifies testing.

### Phase D — Cleanup (Future)

- Remove all sarama references from `go.work` and `go.sum` files
- Verify no indirect sarama dependencies remain
- Update this document to mark migration as complete

## Risk Assessment

- **Low risk**: free-contest-generator migration (standalone, producer-only)
- **Medium risk**: statemachine refactor (shared package, multiple consumers)
- **Low urgency**: No production incidents attributed to the dual-library setup

This is tech debt, not a production issue. Schedule migration during low-activity periods.

## References

- [franz-go documentation](https://pkg.go.dev/github.com/twmb/franz-go/pkg/kgo)
- [sarama documentation](https://pkg.go.dev/github.com/IBM/sarama)
- [Kafka Partitioning Strategy](./kafka-partitioning.md) (uses franz-go examples)
