# Contest Scheduler

The contest-scheduler service manages automatic contest state transitions throughout the contest lifecycle.

## Purpose

This service is responsible for:

1. **Automatic State Transitions** - Moving contests through their lifecycle states:
   - `scheduled` -> `running` (when start time arrives)
   - `running` -> `completed` (when end time arrives)
   - Auto-cancellation if minimum participants not met

2. **Distributed Coordination** - Using Redis distributed locks to safely run multiple instances

3. **Event Publishing** - Publishing contest state change events to Kafka (`contests.v1` topic)

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     Contest Scheduler                            │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐   │
│  │   Scheduler  │  │ State Machine│  │  Distributed Lock    │   │
│  │    Loop      │──│  (transitions)│──│  (Redis)             │   │
│  └──────────────┘  └──────────────┘  └──────────────────────┘   │
│         │                  │                    │                │
│         ▼                  ▼                    ▼                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐   │
│  │  PostgreSQL  │  │    Kafka     │  │       Redis          │   │
│  │  (contests)  │  │ (contests.v1)│  │   (locks, cache)     │   │
│  └──────────────┘  └──────────────┘  └──────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `PORT` | `8088` | HTTP server port |
| `POSTGRES_DSN` | - | PostgreSQL connection string |
| `POSTGRES_REPLICA_DSN` | - | Read replica DSN (optional) |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `REDIS_MODE` | `standalone` | Redis mode: standalone, sentinel, cluster |
| `KAFKA_BROKERS` | `localhost:9092` | Kafka broker addresses |
| `CONTEST_STATE_TOPIC` | `contests.v1` | Kafka topic for state events |
| `CHECK_INTERVAL` | `30s` | How often to check for transitions |
| `MAX_CONCURRENT` | `10` | Max concurrent transitions |
| `MAX_RETRIES` | `3` | Max retries for failed transitions |
| `LOCK_TTL` | `60s` | Redis lock TTL |
| `INSTANCE_ID` | hostname | Unique instance identifier |

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/healthz` | Liveness probe |
| `GET` | `/readyz` | Readiness probe (checks DB, Redis, scheduler) |
| `GET` | `/health/scheduler` | Detailed scheduler health status |
| `GET` | `/metrics` | Prometheus metrics |

## Prometheus Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `contest_scheduler_checks_total` | Counter | Total scheduler check cycles |
| `contest_scheduler_transitions_total` | Counter | State transitions by type |
| `contest_scheduler_transitions_failed_total` | Counter | Failed transitions |
| `contest_scheduler_transition_duration_seconds` | Histogram | Transition duration |
| `contest_scheduler_locks_acquired_total` | Counter | Locks acquired |
| `contest_scheduler_errors_total` | Counter | Total errors |
| `contest_scheduler_candidates_found` | Gauge | Candidates found in last check |

## State Transitions

The scheduler handles these automatic transitions:

```
                    (min participants not met)
                              │
scheduled ─────┬──────────────▼──────────────► cancelled
               │
               │ (start time reached,
               │  min participants met)
               ▼
           running ────────────────────────► completed
                     (end time reached)
```

## Running Locally

```bash
# Start dependencies
make up

# Run the scheduler
make dev-contest-scheduler
```

## Deployment

The service is designed to run multiple replicas safely using distributed locking.
See `infra/k8s/base/contest-scheduler.yaml` for Kubernetes deployment configuration.

## Related Services

- **admin-bff** - Creates and manages contests via API
- **leaderboard-worker** - Processes contest finalization and payouts
- **trading-engine** - Processes trades during running contests

