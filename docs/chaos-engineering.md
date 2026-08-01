# Chaos Engineering Framework

This document describes the chaos engineering framework for validating system resilience under failure conditions in the tragge platform.

## Overview

The chaos engineering framework allows you to simulate various failure scenarios to verify that:

1. **Pod failures** don't cause data loss
2. **Network partitions** are handled gracefully
3. **Database failover** is seamless
4. **The system recovers automatically** within acceptable time limits

## Quick Start

### Prerequisites

- Go 1.22+
- kubectl configured with cluster access
- Access to the target Kubernetes namespace

### Build the Tool

```bash
cd tools/chaos-test
go build -o chaos-test .
```

### List Available Scenarios

```bash
./chaos-test -list
```

### Run a Scenario

```bash
# Basic pod kill test
./chaos-test -scenario=pod-kill -namespace=tragge

# Run with background load
./chaos-test -scenario=pod-kill-trading -with-load \
  -base-url=http://localhost:8080 \
  -email=test@example.com \
  -password=password123 \
  -contest-id=abc123

# Run all scenarios
./chaos-test -scenario=all -output=json > results.json
```

## Available Scenarios

| Scenario | Description | Expected Recovery |
|----------|-------------|-------------------|
| `pod-kill` | Kills a random platform pod | < 2 minutes |
| `pod-kill-trading` | Kills a trading engine pod during active trading | < 3 minutes |
| `pod-kill-bff` | Kills a BFF pod to test client reconnection | < 2 minutes |
| `network-partition` | Creates network partition using NetworkPolicy | < 2 minutes after removal |
| `db-failover` | Kills PostgreSQL primary | < 5 minutes |
| `redis-failover` | Kills Redis master | < 3 minutes |
| `high-cpu` | Injects CPU stress on nodes | Services remain stable |
| `memory-pressure` | Injects memory pressure | Services remain stable |
| `kafka-partition` | Partitions Kafka/Redpanda brokers | < 2 minutes after removal |
| `dns-failure` | Blocks DNS resolution | < 2 minutes after removal |

## Command Line Options

| Flag | Default | Description |
|------|---------|-------------|
| `-scenario` | (required) | Scenario to run or "all" |
| `-namespace` | `tragge` | Kubernetes namespace |
| `-kubeconfig` | `~/.kube/config` | Path to kubeconfig |
| `-timeout` | `10m` | Overall test timeout |
| `-output` | `text` | Output format: `text` or `json` |
| `-dry-run` | `false` | Print what would be done |
| `-list` | `false` | List available scenarios |
| `-with-load` | `false` | Run with background load |
| `-load-users` | `50` | Number of virtual users |
| `-load-duration` | `5m` | Duration of load test |
| `-base-url` | `http://localhost:8080` | Base URL for API |
| `-email` | | Email for authentication |
| `-password` | | Password for authentication |
| `-contest-id` | | Contest ID for trading tests |

## Test Phases

Each chaos test runs through these phases:

### 1. Setup Phase
- Initializes Kubernetes client
- Validates target namespace exists
- Prepares scenario-specific resources

### 2. Chaos Phase
- Injects the failure (pod kill, network policy, etc.)
- Monitors system behavior during chaos
- Collects metrics

### 3. Verify Phase
- Waits for system recovery
- Verifies services are healthy
- Checks for data integrity
- Measures recovery time

### 4. Cleanup Phase
- Removes any injected chaos (NetworkPolicies, stress pods)
- Restores system to normal state

## Success Criteria

A scenario passes if:

1. **Recovery Time**: System recovers within the expected time limit
2. **Data Integrity**: No data loss detected
3. **Error Rate**: Error rate returns to baseline after recovery
4. **P99 Latency**: Returns to baseline within 5 minutes

## Running with Load

The `-with-load` flag enables background load generation during chaos tests. This provides more realistic conditions and validates:

- Circuit breaker behavior under load
- Connection retry mechanisms
- Order processing during failures
- WebSocket reconnection handling

### Load Test Configuration

```bash
./chaos-test -scenario=pod-kill-trading \
  -with-load \
  -load-users=100 \
  -load-duration=10m \
  -base-url=https://staging.tragge.com \
  -email=loadtest@example.com \
  -password=testpassword123 \
  -contest-id=contest-abc123
```

### Load Test Phases

1. **Baseline** (2 minutes): Collect metrics under normal conditions
2. **Chaos** (5 minutes): Inject chaos while maintaining load
3. **Recovery** (2 minutes): Verify return to normal operation

## Interpreting Results

### JSON Output

```json
{
  "scenario": "pod-kill-trading",
  "success": true,
  "duration": "4m32s",
  "metrics": {
    "errors_before_chaos": 0,
    "errors_during_chaos": 15,
    "errors_after_recovery": 0,
    "recovery_time": "45s",
    "data_loss": false,
    "p99_latency": "120ms"
  },
  "phases": [
    {"name": "setup", "success": true, "duration": "2s"},
    {"name": "chaos", "success": true, "duration": "30s"},
    {"name": "verify", "success": true, "duration": "45s"},
    {"name": "cleanup", "success": true, "duration": "1s"}
  ]
}
```

### Key Metrics

| Metric | Description | Target |
|--------|-------------|--------|
| `recovery_time` | Time from chaos injection to full recovery | < 2 minutes |
| `errors_during_chaos` | Number of failed requests during chaos | Depends on scenario |
| `errors_after_recovery` | Errors after recovery period | 0 |
| `data_loss` | Whether any data was lost | `false` |
| `p99_latency` | 99th percentile latency after recovery | Back to baseline |

## CI/CD Integration

### GitHub Actions

The chaos tests run automatically:

- **Weekly**: All scenarios run on Monday at 3am UTC
- **On-demand**: Trigger via workflow_dispatch

See `.github/workflows/chaos-test.yml` for configuration.

### Running in CI

```yaml
- name: Run chaos tests
  run: |
    ./tools/chaos-test/chaos-test \
      -scenario=pod-kill \
      -namespace=tragge-staging \
      -output=json > results.json
```

### Required Secrets

| Secret | Description |
|--------|-------------|
| `KUBECONFIG` | Base64-encoded kubeconfig for cluster access |
| `BASE_URL` | Base URL for load testing |
| `TEST_EMAIL` | Email for test authentication |
| `TEST_PASSWORD` | Password for test authentication |
| `TEST_CONTEST_ID` | Contest ID for trading tests |
| `SLACK_WEBHOOK_URL` | Slack webhook for notifications |

## Scenario Details

### Pod Kill Scenarios

These scenarios test Kubernetes pod recovery:

```bash
# Kill any platform pod
./chaos-test -scenario=pod-kill

# Kill trading engine specifically
./chaos-test -scenario=pod-kill-trading

# Kill BFF to test client reconnection
./chaos-test -scenario=pod-kill-bff
```

**What's tested:**
- Kubernetes automatically reschedules the pod
- Service remains available (via other replicas)
- No requests are lost during failover
- Clients reconnect successfully

### Network Partition Scenario

Simulates network failures using Kubernetes NetworkPolicy:

```bash
./chaos-test -scenario=network-partition
```

**What's tested:**
- Circuit breakers activate properly
- Services degrade gracefully
- Recovery is automatic when network is restored
- No data corruption occurs

### Database Failover Scenario

Tests PostgreSQL high availability:

```bash
./chaos-test -scenario=db-failover
```

**What's tested:**
- Replica promotion to primary
- Application reconnection to new primary
- No data loss during failover
- Write operations resume automatically

### Redis Failover Scenario

Tests Redis Sentinel or Cluster failover:

```bash
./chaos-test -scenario=redis-failover
```

**What's tested:**
- Sentinel/Cluster detects master failure
- New master is elected
- Clients reconnect automatically
- Session data is preserved

### Resource Pressure Scenarios

Tests system stability under resource constraints:

```bash
# CPU stress
./chaos-test -scenario=high-cpu

# Memory pressure
./chaos-test -scenario=memory-pressure
```

**What's tested:**
- Services don't crash under pressure
- Kubernetes resource limits are effective
- System recovers when pressure is removed

## Runbook: Interpreting Results

### Scenario Failed: Recovery Time Exceeded

**Symptoms:**
- `recovery_time` > 2 minutes
- Services show extended downtime

**Investigation:**
1. Check pod restart times: `kubectl get pods -o wide`
2. Review pod events: `kubectl describe pod <pod-name>`
3. Check image pull times
4. Review resource requests/limits

**Remediation:**
- Increase replica count for faster failover
- Ensure image is cached on nodes
- Adjust liveness/readiness probe timeouts

### Scenario Failed: Data Loss Detected

**Symptoms:**
- `data_loss: true` in results
- Order counts don't match

**Investigation:**
1. Check Kafka consumer lag
2. Review order database for gaps
3. Check for failed transactions

**Remediation:**
- Review Kafka producer acks configuration
- Ensure database transactions are committed
- Add order idempotency checks

### Scenario Failed: Errors After Recovery

**Symptoms:**
- `errors_after_recovery` > 0
- P99 latency elevated

**Investigation:**
1. Check connection pool exhaustion
2. Review circuit breaker states
3. Check for backpressure in Kafka

**Remediation:**
- Tune connection pool settings
- Adjust circuit breaker thresholds
- Scale Kafka consumers

## Best Practices

### Before Running in Production

1. Always test in staging first
2. Schedule tests during low-traffic periods
3. Have rollback procedures ready
4. Monitor all dependent services

### During Chaos Tests

1. Monitor dashboards actively
2. Be ready to abort if issues escalate
3. Document any unexpected behavior
4. Keep incident response team informed

### After Chaos Tests

1. Review all metrics and logs
2. Document findings
3. Create tickets for any issues found
4. Update runbooks as needed

## Extending the Framework

### Adding a New Scenario

1. Create a new struct implementing `ChaosScenario`:

```go
type MyNewScenario struct {
    BaseChaosScenario
    // scenario-specific fields
}

func (s *MyNewScenario) Name() string { return "my-new-scenario" }
func (s *MyNewScenario) Description() string { return "Description of what it tests" }
func (s *MyNewScenario) Run(ctx context.Context) error { /* inject chaos */ }
func (s *MyNewScenario) Verify(ctx context.Context) error { /* verify recovery */ }
func (s *MyNewScenario) Cleanup(ctx context.Context) error { /* cleanup resources */ }
```

2. Register in `main.go`:

```go
var scenarios = map[string]ChaosScenario{
    // ...existing scenarios...
    "my-new-scenario": &MyNewScenario{},
}
```

3. Add to GitHub Actions workflow options

### Adding Custom Metrics

Extend the `TestMetrics` struct and update scenario implementations to collect additional metrics.

## Related Documentation

- [Incident Response Runbook](runbook/incident-response.md)
- [Database Recovery](runbook/database-recovery.md)
- [Scaling Guide](runbook/scaling-guide.md)
- [Service Restart Procedures](runbook/service-restart.md)

## Troubleshooting

### "No pods found with selector"

Ensure the namespace and selector match your deployment:

```bash
kubectl get pods -n <namespace> -l app.kubernetes.io/part-of=tragge-platform
```

### "Failed to create Kubernetes client"

Check kubeconfig:

```bash
kubectl config current-context
kubectl cluster-info
```

### "Timeout waiting for recovery"

The scenario's expected recovery time may need adjustment, or there's an actual issue:

1. Check pod status: `kubectl get pods -n <namespace> -w`
2. Check events: `kubectl get events -n <namespace> --sort-by='.lastTimestamp'`
3. Review logs: `kubectl logs -n <namespace> <pod-name>`

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0.0 | 2026-01 | Initial release with 10 scenarios |
