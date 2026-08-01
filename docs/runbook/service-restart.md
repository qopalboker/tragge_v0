# Service Restart Runbook

This document outlines procedures for safely restarting services in the tragge Trading Tournament Platform.

## Table of Contents

1. [Service Dependencies](#service-dependencies)
2. [Pre-Restart Checklist](#pre-restart-checklist)
3. [Service Restart Procedures](#service-restart-procedures)
4. [Rolling Restart](#rolling-restart)
5. [Full Stack Restart](#full-stack-restart)
6. [Troubleshooting](#troubleshooting)

---

## Service Dependencies

### Dependency Graph

```
┌─────────────────────────────────────────────────────────────────┐
│                        Infrastructure                           │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────────┐│
│  │PostgreSQL│  │  Redis   │  │ Redpanda │  │ External APIs    ││
│  │          │  │          │  │ (Kafka)  │  │ (TwelveData,     ││
│  │          │  │          │  │          │  │  Massive)        ││
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────────┬─────────┘│
└───────┼─────────────┼─────────────┼─────────────────┼──────────┘
        │             │             │                 │
        ▼             ▼             ▼                 ▼
┌───────────────────────────────────────────────────────────────┐
│                     Core Services                              │
│  ┌────────────────┐  ┌────────────────┐  ┌─────────────────┐  │
│  │ market-ingestor│  │ trading-engine │  │leaderboard-     │  │
│  │                │──▶│                │──▶│worker           │  │
│  │ (ticks.v1)     │  │ (fills, pnl)   │  │                 │  │
│  └────────────────┘  └────────────────┘  └─────────────────┘  │
└───────────────────────────────────────────────────────────────┘
        │                     │
        ▼                     ▼
┌───────────────────────────────────────────────────────────────┐
│                    BFF Services                                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                     │
│  │ user-bff │  │ trade-bff│  │ admin-bff│                     │
│  │          │  │          │  │          │                     │
│  └──────────┘  └──────────┘  └──────────┘                     │
└───────────────────────────────────────────────────────────────┘
        │             │             │
        ▼             ▼             ▼
┌───────────────────────────────────────────────────────────────┐
│                      Gateway                                   │
│  ┌───────────────────────────────────────────────────────────┐│
│  │                      Nginx                                 ││
│  └───────────────────────────────────────────────────────────┘│
└───────────────────────────────────────────────────────────────┘
```

### Restart Order

**Start Order (Infrastructure → Services):**
1. PostgreSQL
2. Redis
3. Redpanda
4. market-ingestor
5. trading-engine
6. leaderboard-worker
7. user-bff, trade-bff, admin-bff
8. Gateway

**Stop Order (Reverse):**
1. Gateway
2. user-bff, trade-bff, admin-bff
3. leaderboard-worker
4. trading-engine
5. market-ingestor
6. Redpanda
7. Redis
8. PostgreSQL

---

## Pre-Restart Checklist

### Before Any Restart

- [ ] Check current system health
- [ ] Verify no active contests in critical state
- [ ] Notify team in #engineering channel
- [ ] Have rollback plan ready
- [ ] Check for pending deployments

### Health Check Commands

```bash
# Quick health check
for svc in user-bff trade-bff admin-bff trading-engine market-ingestor leaderboard-worker; do
    echo "=== $svc ==="
    kubectl get pods -l app=$svc
    curl -s "http://$svc:8080/healthz" 2>/dev/null | jq . || echo "Not accessible"
done

# Check active contests
psql -h postgres -U app -d app -c \
    "SELECT id, name, status FROM contests WHERE status IN ('running', 'registration_open');"

# Check WebSocket connections
kubectl logs -l app=trade-bff --tail=50 | grep -c "connection"
```

---

## Service Restart Procedures

### user-bff

**Impact:** User authentication, registration, profile access

**Pre-restart:**
```bash
# Check active sessions
redis-cli -h redis KEYS "session:*" | wc -l
```

**Restart:**
```bash
# Rolling restart (zero downtime)
kubectl rollout restart deployment/user-bff

# Monitor restart
kubectl rollout status deployment/user-bff

# Verify
kubectl get pods -l app=user-bff
curl http://user-bff:8081/healthz
```

**Post-restart:**
- Sessions remain valid (stored in Redis)
- Users may need to retry failed requests

---

### trade-bff

**Impact:** WebSocket trading connections, real-time updates

**Pre-restart:**
```bash
# Count active WebSocket connections
kubectl logs -l app=trade-bff --tail=100 | grep "connected" | wc -l

# Notify users if many connections
# Consider maintenance mode for frontend
```

**Restart:**
```bash
# For graceful WebSocket handling, scale down first
kubectl scale deployment/trade-bff --replicas=1

# Wait for connections to drain (30 seconds)
sleep 30

# Rolling restart
kubectl rollout restart deployment/trade-bff
kubectl rollout status deployment/trade-bff

# Scale back up
kubectl scale deployment/trade-bff --replicas=2

# Verify
curl http://trade-bff:8082/healthz
```

**Post-restart:**
- WebSocket clients will reconnect automatically
- Brief gap in real-time updates (< 30 seconds)

---

### admin-bff

**Impact:** Admin dashboard, contest management

**Pre-restart:**
```bash
# Check for ongoing admin operations
kubectl logs -l app=admin-bff --tail=50 | grep -E "(creating|updating|deleting)"
```

**Restart:**
```bash
kubectl rollout restart deployment/admin-bff
kubectl rollout status deployment/admin-bff

# Verify
curl http://admin-bff:8083/healthz
```

**Post-restart:**
- Minimal impact, admins can retry operations

---

### trading-engine

**Impact:** Order processing, position updates, fills

**Pre-restart (Critical):**
```bash
# Check for pending orders
psql -h postgres -U app -d app -c \
    "SELECT COUNT(*) FROM orders WHERE status IN ('pending', 'open');"

# Check Kafka consumer lag
rpk group describe trading-engine-group

# Consider pausing order acceptance
```

**Restart:**
```bash
# Scale down gracefully
kubectl scale deployment/trading-engine --replicas=0

# Wait for in-flight processing (10 seconds)
sleep 10

# Verify orders are processed
psql -h postgres -U app -d app -c \
    "SELECT status, COUNT(*) FROM orders GROUP BY status;"

# Restart
kubectl scale deployment/trading-engine --replicas=1
kubectl rollout status deployment/trading-engine

# Verify Kafka consumers
rpk group describe trading-engine-group
```

**Post-restart:**
- Pending orders will be reprocessed
- Short delay in fill notifications

---

### market-ingestor

**Impact:** Market data feed, price updates

**Pre-restart:**
```bash
# Check current provider status
curl http://market-ingestor:8085/healthz | jq .

# Note: Restart will cause brief gap in market data
```

**Restart:**
```bash
kubectl rollout restart deployment/market-ingestor
kubectl rollout status deployment/market-ingestor

# Verify ticks are flowing
rpk topic consume ticks.v1 --num 5

# Check provider reconnection
kubectl logs -l app=market-ingestor --tail=20
```

**Post-restart:**
- Brief gap in market data (10-30 seconds)
- Provider will automatically reconnect
- May switch to fallback provider temporarily

---

### leaderboard-worker

**Impact:** Leaderboard updates, contest finalization

**Pre-restart:**
```bash
# Check for active contest finalization
kubectl logs -l app=leaderboard-worker --tail=50 | grep -E "(finalizing|payout)"

# Check consumer lag
rpk group describe leaderboard-worker-group
```

**Restart:**
```bash
kubectl rollout restart deployment/leaderboard-worker
kubectl rollout status deployment/leaderboard-worker

# Verify
curl http://leaderboard-worker:8086/healthz
rpk group describe leaderboard-worker-group
```

**Post-restart:**
- Leaderboard updates may be delayed briefly
- Will catch up from Kafka offset

---

### Gateway (Nginx)

**Impact:** All external traffic

**Pre-restart:**
```bash
# Check active connections
kubectl exec -it gateway-xxx -- nginx -s status 2>/dev/null || true
```

**Restart:**
```bash
# Nginx supports graceful reload
kubectl exec -it $(kubectl get pods -l app=gateway -o jsonpath='{.items[0].metadata.name}') \
    -- nginx -s reload

# Or full restart
kubectl rollout restart deployment/gateway
kubectl rollout status deployment/gateway

# Verify
curl http://gateway:8080/health
```

**Post-restart:**
- Active connections are briefly interrupted
- Clients will reconnect immediately

---

## Rolling Restart

### All Application Services

Use this for routine maintenance or after config changes:

```bash
#!/bin/bash
# rolling-restart-apps.sh

set -e

SERVICES=(
    "market-ingestor"
    "trading-engine"
    "leaderboard-worker"
    "user-bff"
    "trade-bff"
    "admin-bff"
    "gateway"
)

echo "Starting rolling restart of application services..."

for svc in "${SERVICES[@]}"; do
    echo "Restarting $svc..."
    kubectl rollout restart deployment/$svc
    kubectl rollout status deployment/$svc --timeout=120s
    echo "$svc restarted successfully"

    # Wait between restarts
    sleep 10
done

echo "All services restarted successfully!"
```

### Quick Restart Script

```bash
# Restart single service with monitoring
restart_service() {
    local svc=$1
    echo "=== Restarting $svc ==="

    # Pre-restart health
    echo "Pre-restart status:"
    kubectl get pods -l app=$svc

    # Restart
    kubectl rollout restart deployment/$svc
    kubectl rollout status deployment/$svc --timeout=120s

    # Post-restart verification
    echo "Post-restart status:"
    kubectl get pods -l app=$svc

    # Health check
    local port=$(kubectl get svc $svc -o jsonpath='{.spec.ports[0].port}')
    sleep 5
    curl -s "http://$svc:$port/healthz" | jq . || echo "Health check pending..."
}

# Usage: restart_service user-bff
```

---

## Full Stack Restart

### Complete System Restart

Use this only for major issues or scheduled maintenance:

```bash
#!/bin/bash
# full-restart.sh

set -e

echo "=========================================="
echo "FULL STACK RESTART"
echo "=========================================="
echo "This will restart ALL services including infrastructure."
echo "Estimated downtime: 5-10 minutes"
read -p "Continue? (yes/no): " confirm
if [[ "$confirm" != "yes" ]]; then
    echo "Cancelled"
    exit 1
fi

# 1. Stop application services
echo "Stopping application services..."
kubectl scale deployment user-bff trade-bff admin-bff --replicas=0
kubectl scale deployment trading-engine market-ingestor leaderboard-worker --replicas=0
kubectl scale deployment gateway --replicas=0

# 2. Wait for graceful shutdown
echo "Waiting for graceful shutdown..."
sleep 30

# 3. Restart infrastructure (if needed)
echo "Restarting infrastructure..."
kubectl rollout restart statefulset/postgres
kubectl rollout status statefulset/postgres --timeout=300s

kubectl rollout restart statefulset/redis
kubectl rollout status statefulset/redis --timeout=120s

kubectl rollout restart statefulset/redpanda
kubectl rollout status statefulset/redpanda --timeout=300s

# 4. Verify infrastructure
echo "Verifying infrastructure..."
pg_isready -h postgres -U app
redis-cli -h redis PING
rpk cluster health

# 5. Start application services in order
echo "Starting application services..."

kubectl scale deployment market-ingestor --replicas=1
kubectl rollout status deployment/market-ingestor --timeout=120s

kubectl scale deployment trading-engine --replicas=1
kubectl rollout status deployment/trading-engine --timeout=120s

kubectl scale deployment leaderboard-worker --replicas=1
kubectl rollout status deployment/leaderboard-worker --timeout=120s

kubectl scale deployment user-bff --replicas=2
kubectl scale deployment trade-bff --replicas=2
kubectl scale deployment admin-bff --replicas=2
kubectl rollout status deployment/user-bff deployment/trade-bff deployment/admin-bff --timeout=120s

kubectl scale deployment gateway --replicas=2
kubectl rollout status deployment/gateway --timeout=60s

# 6. Final verification
echo "=========================================="
echo "RESTART COMPLETE"
echo "=========================================="
kubectl get pods
echo ""
echo "Run health checks to verify:"
echo "  ./scripts/health-check.sh"
```

---

## Troubleshooting

### Pod Stuck in Terminating

```bash
# Force delete stuck pod
kubectl delete pod <pod-name> --grace-period=0 --force

# Check for finalizers
kubectl get pod <pod-name> -o yaml | grep finalizers
```

### Pod CrashLoopBackOff

```bash
# Check logs
kubectl logs <pod-name> --previous

# Check events
kubectl describe pod <pod-name>

# Common fixes:
# - Check environment variables
# - Verify secrets exist
# - Check resource limits
# - Verify dependencies are running
```

### Service Not Ready After Restart

```bash
# Check readiness probe
kubectl describe pod <pod-name> | grep -A5 "Readiness"

# Check logs for startup errors
kubectl logs <pod-name> --tail=100

# Verify dependencies
# PostgreSQL
pg_isready -h postgres

# Redis
redis-cli -h redis PING

# Kafka
rpk cluster health
```

### Restart Rollback

If restart causes issues:

```bash
# Rollback deployment
kubectl rollout undo deployment/<service-name>

# Or rollback to specific revision
kubectl rollout undo deployment/<service-name> --to-revision=<revision>

# Check rollout history
kubectl rollout history deployment/<service-name>
```

---

## Quick Reference

### Common Commands

```bash
# Restart single service
kubectl rollout restart deployment/<service>

# Watch restart progress
kubectl rollout status deployment/<service>

# Rollback if needed
kubectl rollout undo deployment/<service>

# Check all pods
kubectl get pods -o wide

# Check pod logs
kubectl logs -l app=<service> --tail=100

# Describe pod for events
kubectl describe pod <pod-name>
```

### Service Ports

| Service | Internal Port | Health Endpoint |
|---------|---------------|-----------------|
| user-bff | 8081 | /healthz |
| trade-bff | 8082 | /healthz |
| admin-bff | 8083 | /healthz |
| trading-engine | 8084 | /healthz |
| market-ingestor | 8085 | /healthz |
| leaderboard-worker | 8086 | /healthz |
| gateway | 8080 | /health |

---

*Last Updated: January 2026*
