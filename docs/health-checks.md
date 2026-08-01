# Health Check Endpoints

This document describes the health check endpoints implemented across all Tragge services for Kubernetes liveness, readiness, and startup probes.

## Overview

All services implement three health check endpoints:

| Endpoint | Purpose | HTTP Status |
|----------|---------|-------------|
| `/healthz` | Liveness probe - is the process running? | 200 OK |
| `/readyz` | Readiness probe - can the service handle traffic? | 200 OK / 503 Service Unavailable |
| `/health/circuits` | Circuit breaker status | 200 OK / 503 Service Unavailable |
| `/metrics` | Prometheus metrics | 200 OK |

## Response Schema

### Readiness Response (`/readyz`)

All services return a consistent JSON response format:

```json
{
  "status": "ready|degraded|unavailable",
  "service": "service-name",
  "timestamp": "2025-01-08T12:00:00Z",
  "message": "optional error message",
  "database": "healthy|unavailable|not_configured",
  "redis": "healthy|unavailable|not_configured",
  "kafka": "healthy|unavailable" | { ... detailed status },
  "circuits": "healthy" | { ... circuit breaker status }
}
```

### Status Values

| Status | Description | HTTP Code |
|--------|-------------|-----------|
| `ready` | All critical dependencies are healthy | 200 |
| `degraded` | Some non-critical dependencies are unavailable, service can still handle traffic | 200 |
| `unavailable` | Critical dependencies are unavailable, service cannot handle traffic | 503 |

### Liveness Response (`/healthz`)

Simple response indicating the process is alive:

```json
{
  "status": "ok"
}
```

## Service-Specific Dependencies

### user-bff (:8081)

| Dependency | Critical | Description |
|------------|----------|-------------|
| Database | Yes | PostgreSQL primary and replicas |
| Redis | No | Session management (degrades gracefully) |
| Circuits | Yes | Circuit breakers for external calls |

### admin-bff (:8083)

| Dependency | Critical | Description |
|------------|----------|-------------|
| Database | Yes | PostgreSQL primary and replicas |
| Kafka | No | Admin queries only (degrades gracefully) |
| Circuits | Yes | Circuit breakers for external calls |

### trade-bff (:8082)

| Dependency | Critical | Description |
|------------|----------|-------------|
| Database | Yes | PostgreSQL primary and replicas |
| Redis | No | WebSocket session registry |
| Kafka | No | Order publishing and event consumption |
| Circuits | No | Circuit breakers (degrades gracefully) |

### trading-engine (:8085)

| Dependency | Critical | Description |
|------------|----------|-------------|
| Database | Yes | Order and position persistence |
| Redis | Yes | Price book and order state |
| Kafka | Yes | Order consumption and event publishing |
| Circuits | Yes | Circuit breakers for external calls |
| Shard | Yes* | Shard readiness (if sharding enabled) |

### market-ingestor (:8084)

| Dependency | Critical | Description |
|------------|----------|-------------|
| WebSocket Provider | Yes | TwelveData/Massive market data |
| Redis | No | Tick caching |
| Kafka | No | Tick publishing |

### leaderboard-worker (:8086)

| Dependency | Critical | Description |
|------------|----------|-------------|
| Database | Yes | Leaderboard persistence |
| Redis | Yes | Leaderboard sorted sets |
| Kafka | No | PnL delta consumption |
| Circuits | Yes | Circuit breakers |

### shard-router (:8090)

| Dependency | Critical | Description |
|------------|----------|-------------|
| Database | Yes | Shard metadata |
| Redis | Yes | Routing cache |
| Shards | Yes | At least one active shard required |
| Circuits | Yes | Circuit breakers |

## Kubernetes Probe Configuration

### Startup Probe

Used to determine when a container has started successfully. The startup probe runs first, and once it succeeds, liveness and readiness probes begin.

```yaml
startupProbe:
  httpGet:
    path: /healthz
    port: http
  initialDelaySeconds: 2-5
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 12-24  # Varies by service
  successThreshold: 1
```

Maximum startup times:
- **BFF services**: 62s (2 + 12*5)
- **Workers with Kafka**: 92s (2 + 18*5)
- **Trading engine / Market ingestor**: 125s (5 + 24*5)

### Liveness Probe

Checks if the process is alive. If it fails, Kubernetes restarts the container.

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: http
  periodSeconds: 10
  timeoutSeconds: 3
  failureThreshold: 3
  successThreshold: 1
```

### Readiness Probe

Checks if the service can handle traffic. If it fails, the pod is removed from service endpoints.

```yaml
readinessProbe:
  httpGet:
    path: /readyz
    port: http
  periodSeconds: 5
  timeoutSeconds: 3
  failureThreshold: 3
  successThreshold: 1
```

## Grafana Dashboard

A dedicated Grafana dashboard "Service Health Probes" provides:

1. **Service Health Status**: Real-time status of all services (healthy/degraded/unavailable)
2. **Health Check Latency**: p95 latency for database, Redis, Kafka, and circuit checks
3. **Dependency Status Heatmap**: Visual timeline of all dependencies
4. **Kubernetes Probe Metrics**: Pod restarts and readiness status

## Prometheus Metrics

The health package exposes the following metrics:

```
# Health check duration
health_check_duration_seconds{service, dependency}

# Health check status (1=ok, 0.5=degraded, 0=unavailable)
health_check_status{service, dependency}
```

## Best Practices

1. **Don't check external dependencies in liveness probes** - Only check if the process is running
2. **Use startup probes for slow-starting services** - Prevents premature restarts during initialization
3. **Set appropriate timeouts** - Health checks should complete within 2-3 seconds
4. **Mark non-critical failures as degraded, not unavailable** - Allows traffic to continue flowing
5. **Include detailed information in readiness responses** - Helps with debugging
6. **Monitor health check latency** - Slow checks can indicate infrastructure issues
