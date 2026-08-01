# Kafka Topic Routing

This document is the canonical reference for all Kafka topics in the tragge platform.
It is kept in sync with the regression test in `tests/integration/kafka_topics_test.go`.

## Topic Routing Table

| Topic | Env Override | Producer(s) | Consumer(s) |
|-------|-------------|-------------|-------------|
| `orders.v1` | `ORDERS_TOPIC` | trade-bff | trading-engine |
| `fills.v1` | `FILLS_TOPIC` | trading-engine | trade-bff |
| `positions.v1` | `POSITIONS_TOPIC` | trading-engine | trade-bff |
| `order_acks.v1` | `ORDER_ACKS_TOPIC` | trading-engine | trade-bff |
| `order_cancelled.v1` | `ORDER_CANCELLED_TOPIC` | trading-engine | trade-bff |
| `ticks.v1` | `TICKS_TOPIC` | market-ingestor | trading-engine, trade-bff |
| `close_positions.v1` | `CLOSE_POSITIONS_TOPIC` | trade-bff | trading-engine |
| `cancel_orders.v1` | `CANCEL_ORDERS_TOPIC` | trade-bff | trading-engine |
| `modify_tpsl.v1` | `MODIFY_TPSL_TOPIC` | trade-bff | trading-engine |
| `pnl_deltas.v1` | `PNL_DELTAS_TOPIC` | trading-engine | leaderboard-worker, trade-bff |
| `contests.v1` | `CONTEST_STATE_TOPIC` | contest-scheduler, free-contest-generator | trading-engine, settlement-service, leaderboard-worker, trade-bff |
| `position_closed.v1` | `POSITION_CLOSED_TOPIC` | trading-engine | settlement-service |
| `settlement_requests.v1` | `SETTLEMENT_REQ_TOPIC` | contest-scheduler | settlement-service |
| `settlement_events.v1` | `SETTLEMENT_EVENTS_TOPIC` | settlement-service | *(DB only)* |
| `contest_close_positions.v1` | `CONTEST_CLOSE_POSITIONS_TOPIC` | settlement-service | trading-engine |
| `contest_cancel_orders.v1` | `CONTEST_CANCEL_ORDERS_TOPIC` | settlement-service | trading-engine |
| `notifications.v1` | `NOTIFICATIONS_TOPIC` | settlement-service | leaderboard-worker |
| `alerts.v1` | `ALERTS_TOPIC` | trading-engine | *(fire-and-forget)* |

## Flow Diagrams

### Core Trading Flow

```
                         orders.v1
  trade-bff  ─────────────────────────────────►  trading-engine
      ▲                                               │
      │                                               │
      │  fills.v1                                     │  fills.v1
      ◄───────────────────────────────────────────────┤
      │                                               │
      │  positions.v1                                 │  positions.v1
      ◄───────────────────────────────────────────────┤
      │                                               │
      │  order_acks.v1                                │  order_acks.v1
      ◄───────────────────────────────────────────────┤
      │                                               │
      │  order_cancelled.v1                           │  order_cancelled.v1
      ◄───────────────────────────────────────────────┘
```

### Market Data Flow

```
  market-ingestor ───── ticks.v1 ─────►  trading-engine
                            │
                            └──────────►  trade-bff
```

### User-Initiated Mutations

```
                    close_positions.v1
  trade-bff  ──────────────────────────►  trading-engine
                    cancel_orders.v1
             ──────────────────────────►  trading-engine
                    modify_tpsl.v1
             ──────────────────────────►  trading-engine
```

### Scoring & Leaderboard

```
  trading-engine ─── pnl_deltas.v1 ──►  leaderboard-worker
                          │
                          └───────────►  trade-bff
```

### Contest Lifecycle

```
  contest-scheduler  ─┐
                      ├── contests.v1 ──►  trading-engine
  free-contest-gen   ─┘        │
                               ├───────►  settlement-service
                               ├───────►  leaderboard-worker
                               └───────►  trade-bff
```

### Settlement Flow

```
                              settlement_requests.v1
  contest-scheduler  ─────────────────────────────────►  settlement-service
                                                               │
                                                               │ contest_close_positions.v1
  trading-engine  ◄────────────────────────────────────────────┤
                                                               │ contest_cancel_orders.v1
  trading-engine  ◄────────────────────────────────────────────┤
                                                               │
  trading-engine ──── position_closed.v1 ──────────────────────►  settlement-service
                                                               │
                                                               │ settlement_events.v1
                                                               │ (persisted to DB)
                                                               │
                                                               │ notifications.v1
  leaderboard-worker ◄─────────────────────────────────────────┘
```

### Full System Overview

```
┌──────────────────┐                              ┌────────────────────┐
│  market-ingestor │── ticks.v1 ─────────────────►│                    │
└──────────────────┘                              │                    │
                                                  │   trading-engine   │
┌──────────────────┐   orders.v1                  │                    │
│                  │─────────────────────────────►│                    │
│                  │   close_positions.v1          │                    │
│                  │─────────────────────────────►│                    │
│    trade-bff     │   cancel_orders.v1           │                    │
│                  │─────────────────────────────►│                    │
│                  │   modify_tpsl.v1             │                    │
│                  │─────────────────────────────►│                    │
│                  │                              │                    │
│                  │◄─── fills.v1 ────────────────│                    │
│                  │◄─── positions.v1 ────────────│                    │
│                  │◄─── order_acks.v1 ───────────│                    │
│                  │◄─── order_cancelled.v1 ──────│                    │
│                  │◄─── pnl_deltas.v1 ───────────│                    │
│                  │◄─── ticks.v1 ────────────────│                    │
│                  │◄─── contests.v1 ─────────────│                    │
└──────────────────┘                              └───────┬────────────┘
                                                          │
                                     pnl_deltas.v1        │ position_closed.v1
                                          │               │
                                          ▼               ▼
                                   ┌──────────────┐ ┌──────────────────┐
                                   │ leaderboard- │ │  settlement-     │
                                   │   worker     │ │    service       │
                                   └──────┬───────┘ └──────┬───────────┘
                                          │                │
                                          │                │ contest_close_positions.v1
                                          │                │ contest_cancel_orders.v1
                                          │                │     │
                                          │                │     └──► trading-engine
                                          │                │
                                          │◄─── notifications.v1
                                          │◄─── contests.v1
                                          │
┌──────────────────┐   contests.v1        │
│contest-scheduler │──────────┬───────────┘
│                  │          │
│free-contest-gen  │          ├──► trading-engine
└──────────────────┘          ├──► settlement-service
                              ├──► leaderboard-worker
                              └──► trade-bff
```

## Consumer Groups

| Service | Consumer Group | Topics |
|---------|---------------|--------|
| trading-engine | `trading-engine` | orders.v1, ticks.v1, close_positions.v1, cancel_orders.v1, modify_tpsl.v1 |
| trading-engine | `trading-engine-contest-state` | contests.v1 |
| trading-engine | `trading-engine-contest-close` | contest_close_positions.v1 |
| trading-engine | `trading-engine-contest-cancel` | contest_cancel_orders.v1 |
| trade-bff | `trade-bff-*` | ticks.v1, fills.v1, positions.v1, order_acks.v1, order_cancelled.v1, pnl_deltas.v1, contests.v1 |
| leaderboard-worker | `leaderboard-worker` | pnl_deltas.v1 |
| leaderboard-worker | `leaderboard-worker-contest-state` | contests.v1 |
| leaderboard-worker | `leaderboard-worker-notifications` | notifications.v1 |
| settlement-service | `settlement-service` | contests.v1, settlement_requests.v1, position_closed.v1 |

## Env Var Quick Reference

Services may use different env var names for the same logical topic. This
table shows the env var each service reads for each topic.

| Topic | trading-engine | trade-bff | leaderboard-worker | settlement-service | contest-scheduler | free-contest-gen |
|-------|---------------|-----------|-------------------|-------------------|-------------------|-----------------|
| orders.v1 | `ORDERS_TOPIC` | `ORDERS_TOPIC` | — | — | — | — |
| ticks.v1 | `TICKS_TOPIC` | *(hardcoded)* | — | — | — | — |
| fills.v1 | `FILLS_TOPIC` | `FILLS_TOPIC` | — | — | — | — |
| positions.v1 | `POSITIONS_TOPIC` | `POSITIONS_TOPIC` | — | — | — | — |
| order_acks.v1 | `ORDER_ACKS_TOPIC` | `ORDER_ACKS_TOPIC` | — | — | — | — |
| order_cancelled.v1 | `ORDER_CANCELLED_TOPIC` | `ORDER_CANCELLED_TOPIC` | — | — | — | — |
| pnl_deltas.v1 | `PNL_DELTAS_TOPIC` | `PNL_DELTAS_TOPIC` | `PNL_DELTAS_TOPIC` | — | — | — |
| contests.v1 | `CONTESTS_TOPIC` | `CONTEST_STATE_TOPIC` | `CONTEST_STATE_TOPIC` | `CONTEST_STATE_TOPIC` | `CONTEST_STATE_TOPIC` | `KAFKA_CONTESTS_TOPIC` |
| close_positions.v1 | `CLOSE_POSITIONS_TOPIC` | `CLOSE_POSITIONS_TOPIC` | — | — | — | — |
| cancel_orders.v1 | `CANCEL_ORDERS_TOPIC` | `CANCEL_ORDERS_TOPIC` | — | — | — | — |
| modify_tpsl.v1 | `MODIFY_TPSL_TOPIC` | `MODIFY_TPSL_TOPIC` | — | — | — | — |
| position_closed.v1 | `POSITION_CLOSED_TOPIC` | — | — | `POSITION_CLOSED_TOPIC` | — | — |
| contest_close_positions.v1 | `CONTEST_CLOSE_POSITIONS_TOPIC` | — | — | `CLOSE_POSITIONS_TOPIC` | — | — |
| contest_cancel_orders.v1 | `CONTEST_CANCEL_ORDERS_TOPIC` | — | — | `CANCEL_ORDERS_TOPIC` | — | — |
| notifications.v1 | — | — | `NOTIFICATIONS_TOPIC` | `NOTIFICATIONS_TOPIC` | — | — |
| alerts.v1 | `ALERTS_TOPIC` | — | — | — | — | — |
| settlement_requests.v1 | — | — | — | `SETTLEMENT_REQ_TOPIC` | — | — |
| settlement_events.v1 | — | — | — | `SETTLEMENT_EVENTS_TOPIC` | — | — |

## Naming Conventions

- **Topic names**: `<domain_with_underscores>.<version>` (e.g. `pnl_deltas.v1`)
- **Env vars**: `<UPPER_SNAKE_CASE>_TOPIC` (e.g. `PNL_DELTAS_TOPIC`)
- **Version**: All topics currently use `.v1`

## Regression Testing

The `tests/integration/kafka_topics_test.go` file contains automated tests that
verify all topic routing is correct. Run with:

```bash
cd tests/integration && go test -v -run TestKafkaTopicAlignment ./...
```

The test validates:
1. Every producer/consumer default matches the expected topic name
2. No topics are orphaned (produced but never consumed)
3. Every env var in the service config map is referenced by at least one route
4. Topic names follow the naming convention
5. No duplicate topic definitions

When adding a new topic or changing a service's topic configuration, update
both this document and the test file.
