# Circuit Breaker Runbook

This runbook covers troubleshooting circuit breaker issues in the Tragge platform.

## Overview

Circuit breakers protect services from cascading failures by temporarily stopping requests to unhealthy dependencies. The platform uses circuit breakers for:

- External API calls (market data providers, payment gateways)
- Database connections
- Inter-service communication
- Redis connections

## Circuit Breaker States

| State | Description | Behavior |
|-------|-------------|----------|
| **Closed** | Normal operation | Requests flow through normally |
| **Open** | Failures exceeded threshold | Requests immediately fail (fast-fail) |
| **Half-Open** | Testing recovery | Limited requests allowed to test |

## Alert Types

- **CircuitBreakerOpen**: A circuit breaker has opened
- **CircuitBreakerHighFailureRate**: Failure rate approaching threshold
- **CircuitBreakerFlapping**: Circuit is rapidly opening/closing
- **BulkheadSaturation**: Bulkhead concurrent limit reached
- **BulkheadRejectionSpike**: High rate of bulkhead rejections
- **CircuitBreakerSlowRecovery**: Circuit not recovering after timeout
- **CircuitBreakerTimeouts**: High timeout rate on circuit
- **CircuitBreakerHalfOpen**: Circuit stuck in half-open state

## Common Issues and Resolutions

### Open Circuit

**Symptoms:**
- Circuit breaker in open state
- Requests failing fast without reaching dependency
- Error messages about circuit being open

**Investigation Steps:**

1. **Identify which circuit opened:**
   ```promql
   tragge_circuit_breaker_state == 2  # 2 = open
   ```

2. **Check failure rate history:**
   ```promql
   tragge_circuit_breaker_failure_rate{circuit="$circuit"}
   ```

3. **Check the protected dependency:**
   ```bash
   # For external APIs
   curl -s -o /dev/null -w "%{http_code}" https://api.example.com/health

   # For internal services
   kubectl exec -it deploy/trading-engine -n tragge -- curl localhost:8084/healthz
   ```

4. **Review recent errors:**
   ```bash
   kubectl logs -l app=trading-engine -n tragge --tail=200 | grep -i "circuit\|error"
   ```

**Resolution Steps:**

1. **If dependency is down:**
   - Check dependency status and fix root cause
   - Circuit will auto-recover once dependency is healthy

2. **If false positive:**
   - Review circuit breaker thresholds
   - Consider increasing failure threshold or window

3. **Force circuit reset (use with caution):**
   ```bash
   # Via metrics endpoint if available
   curl -X POST http://service:8080/admin/circuit-breaker/reset?circuit=market-data
   ```

### High Failure Rate

**Symptoms:**
- Failure rate above warning threshold but below trip threshold
- Degraded performance
- Intermittent errors

**Investigation Steps:**

1. **Check current failure rate:**
   ```promql
   tragge_circuit_breaker_failure_rate{circuit="$circuit"}
   ```

2. **Identify error types:**
   ```promql
   sum by (error_type) (
     rate(tragge_circuit_breaker_failures_total{circuit="$circuit"}[5m])
   )
   ```

3. **Check latency:**
   ```promql
   histogram_quantile(0.99,
     rate(tragge_circuit_breaker_request_duration_seconds_bucket{circuit="$circuit"}[5m])
   )
   ```

**Resolution Steps:**

1. **Address underlying issues before circuit trips:**
   - Fix timeout issues
   - Scale dependent service
   - Check network connectivity

2. **Adjust timeouts if too aggressive:**
   ```yaml
   circuit_breaker:
     timeout: 5s  # Increase if legitimate slow responses
   ```

### Flapping

**Symptoms:**
- Circuit rapidly alternating between states
- Metrics show frequent state changes
- Inconsistent service behavior

**Investigation Steps:**

1. **Check state change frequency:**
   ```promql
   changes(tragge_circuit_breaker_state{circuit="$circuit"}[10m])
   ```

2. **Check if dependency is partially healthy:**
   ```promql
   rate(tragge_circuit_breaker_successes_total{circuit="$circuit"}[1m])
   rate(tragge_circuit_breaker_failures_total{circuit="$circuit"}[1m])
   ```

**Resolution Steps:**

1. **Adjust circuit breaker configuration:**
   ```yaml
   circuit_breaker:
     # Increase to prevent flapping
     half_open_max_requests: 5
     recovery_timeout: 60s
     failure_threshold: 10
   ```

2. **Add jitter to recovery timeout:**
   - Prevents thundering herd on recovery

3. **Check for load balancer issues:**
   - Ensure requests are distributed evenly
   - Check if some instances are unhealthy

### Bulkhead Saturation

**Symptoms:**
- Bulkhead at maximum concurrent requests
- Requests being queued or rejected
- High latency for bulkhead-protected operations

**Investigation Steps:**

1. **Check current utilization:**
   ```promql
   tragge_bulkhead_in_use / tragge_bulkhead_max
   ```

2. **Check queue depth:**
   ```promql
   tragge_bulkhead_queue_size
   ```

**Resolution Steps:**

1. **Scale the service:**
   ```bash
   kubectl scale deploy/trading-engine --replicas=5 -n tragge
   ```

2. **Increase bulkhead limits:**
   ```yaml
   bulkhead:
     max_concurrent: 100  # Increase carefully
     max_wait_duration: 5s
   ```

3. **Optimize protected operations:**
   - Reduce operation duration
   - Add caching where appropriate

### Rejection Spike

**Symptoms:**
- High rate of bulkhead rejections
- Users experiencing errors
- Operations not being processed

**Investigation Steps:**

1. **Check rejection rate:**
   ```promql
   rate(tragge_bulkhead_rejections_total[5m])
   ```

2. **Check if traffic spike:**
   ```promql
   rate(tragge_http_requests_total[5m])
   ```

**Resolution Steps:**

1. **For traffic spike:**
   - Scale services
   - Enable auto-scaling
   - Consider rate limiting at gateway

2. **For sustained high load:**
   - Review capacity planning
   - Optimize operations
   - Add caching

### Slow Recovery

**Symptoms:**
- Circuit stays open longer than expected
- Half-open probes failing
- Service not recovering after dependency fix

**Investigation Steps:**

1. **Check half-open success rate:**
   ```promql
   rate(tragge_circuit_breaker_successes_total{circuit="$circuit", state="half_open"}[5m])
   ```

2. **Verify dependency is actually healthy:**
   ```bash
   # Direct health check
   curl http://dependency-service:8080/healthz
   ```

**Resolution Steps:**

1. **If dependency is healthy but probes fail:**
   - Check if half-open requests are different from probes
   - Ensure probe endpoint is representative

2. **Force recovery (use with caution):**
   ```bash
   # Restart service to reset circuit state
   kubectl rollout restart deploy/trading-engine -n tragge
   ```

### Timeouts

**Symptoms:**
- High timeout rate on circuit
- Slow responses from dependency
- Circuit may trip due to timeouts

**Investigation Steps:**

1. **Check timeout rate:**
   ```promql
   rate(tragge_circuit_breaker_timeouts_total{circuit="$circuit"}[5m])
   ```

2. **Check dependency latency:**
   ```promql
   histogram_quantile(0.99,
     rate(tragge_external_request_duration_seconds_bucket{dependency="$dep"}[5m])
   )
   ```

**Resolution Steps:**

1. **If dependency is slow:**
   - Investigate dependency performance
   - Add caching
   - Consider async processing

2. **If timeout too aggressive:**
   ```yaml
   circuit_breaker:
     timeout: 10s  # Increase timeout
   ```

### Half-Open State

**Symptoms:**
- Circuit stuck in half-open state
- Probes not succeeding or failing
- Limited throughput

**Investigation Steps:**

1. **Check state duration:**
   ```promql
   time() - tragge_circuit_breaker_state_change_timestamp{circuit="$circuit"}
   ```

2. **Check probe results:**
   ```promql
   rate(tragge_circuit_breaker_half_open_successes_total{circuit="$circuit"}[5m])
   rate(tragge_circuit_breaker_half_open_failures_total{circuit="$circuit"}[5m])
   ```

**Resolution Steps:**

1. **If probes are timing out:**
   - Increase half-open timeout
   - Check probe endpoint

2. **If success/failure threshold not being met:**
   - Adjust thresholds
   - Check for intermittent issues

## Configuration Reference

```yaml
circuit_breaker:
  # Number of failures to trip circuit
  failure_threshold: 5

  # Window for counting failures
  failure_window: 10s

  # Time to wait before entering half-open
  recovery_timeout: 30s

  # Requests to allow in half-open state
  half_open_max_requests: 3

  # Successes needed to close circuit
  success_threshold: 2

  # Request timeout
  timeout: 5s

bulkhead:
  # Maximum concurrent requests
  max_concurrent: 50

  # Maximum queue size
  max_queue_size: 100

  # Maximum wait time in queue
  max_wait_duration: 10s
```

## Monitoring Dashboard

View the Circuit Breaker Health dashboard in Grafana:
- Dashboard: `/d/circuit-breaker-health`

## Related Runbooks

- [Incident Response](incident-response.md)
- [Scaling Guide](scaling-guide.md)
- [Service Restart](service-restart.md)
