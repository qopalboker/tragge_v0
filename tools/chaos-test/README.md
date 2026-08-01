# Chaos Test Tool

A chaos engineering framework for validating system resilience under failure conditions.

## Quick Start

```bash
# Build the tool
go build -o chaos-test .

# List available scenarios
./chaos-test -list

# Run a basic scenario
./chaos-test -scenario=pod-kill -namespace=tragge

# Run with JSON output
./chaos-test -scenario=pod-kill -output=json
```

## Usage

```
chaos-test [options]

Options:
  -scenario string     Chaos scenario to run (use -list to see available)
  -namespace string    Kubernetes namespace (default "tragge")
  -kubeconfig string   Path to kubeconfig (uses in-cluster config if empty)
  -timeout duration    Overall test timeout (default 10m0s)
  -output string       Output format: text, json (default "text")
  -dry-run            Print what would be done without executing
  -list               List available scenarios
  -with-load          Run scenario with background load
  -load-users int     Number of virtual users for load test (default 50)
  -load-duration      Duration of load test (default 5m0s)
  -base-url string    Base URL for API calls (default "http://localhost:8080")
  -email string       Email for authentication (required for load tests)
  -password string    Password for authentication (required for load tests)
  -contest-id string  Contest ID for trading tests
```

## Available Scenarios

### Basic Pod Kill Scenarios

| Scenario | Description |
|----------|-------------|
| `pod-kill` | Kills a random pod and verifies automatic recovery |
| `pod-kill-trading` | Kills a trading engine pod and verifies no order loss |
| `pod-kill-bff` | Kills a BFF pod and verifies client reconnection |

### Database Scenarios

| Scenario | Description |
|----------|-------------|
| `db-failover` | Kills PostgreSQL primary and verifies replica promotion |
| `database-failure` | Kills PostgreSQL, verifies circuit breakers open and services degrade gracefully |

### Redis Scenarios

| Scenario | Description |
|----------|-------------|
| `redis-failover` | Kills Redis master and verifies Sentinel/Cluster failover |
| `redis-failure` | Kills Redis, verifies leaderboard falls back to cached data and trading continues |

### Kafka Scenarios

| Scenario | Description |
|----------|-------------|
| `kafka-partition` | Partitions Kafka brokers and verifies message delivery resumes |
| `kafka-broker-failure` | Kills one Kafka broker, verifies message processing continues with rebalancing |

### Network Scenarios

| Scenario | Description |
|----------|-------------|
| `network-partition` | Simulates network partition using NetworkPolicy |
| `network-partition-iptables` | Uses iptables to block traffic, verifies circuit detection and graceful degradation |
| `dns-failure` | Blocks DNS resolution and verifies graceful degradation |

### Resource Pressure Scenarios

| Scenario | Description |
|----------|-------------|
| `high-cpu` | Injects CPU stress on a node and verifies service stability |
| `memory-pressure` | Injects memory pressure and verifies OOM handling |

### Cascade Failure Scenarios

| Scenario | Description |
|----------|-------------|
| `cascade-failure` | Causes shard-router to fail, verifies trade-bff circuit opens and continues with fallback routing |

## Examples

### Run All Scenarios

```bash
./chaos-test -scenario=all -output=json > results.json
```

### Run with Background Load

```bash
./chaos-test -scenario=pod-kill-trading \
  -with-load \
  -load-users=100 \
  -base-url=https://staging.tragge.com \
  -email=test@example.com \
  -password=password123 \
  -contest-id=abc123
```

### Dry Run

```bash
./chaos-test -scenario=db-failover -dry-run
```

## Success Criteria

A scenario passes if:

- **Recovery Time**: < 2 minutes (5 minutes for database failover)
- **Data Loss**: None detected
- **Errors After Recovery**: 0
- **P99 Latency**: Returns to baseline within 5 minutes

## Documentation

See [docs/chaos-engineering.md](../../docs/chaos-engineering.md) for detailed documentation.

## Requirements

- Go 1.22+
- kubectl access to target Kubernetes cluster
- Appropriate RBAC permissions to:
  - List/delete pods
  - Create/delete NetworkPolicies
  - List services and endpoints
