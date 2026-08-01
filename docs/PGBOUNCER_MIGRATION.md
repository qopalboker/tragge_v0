# PgBouncer Migration Guide

This guide provides step-by-step instructions for deploying PgBouncer connection pooler and migrating all services to use it.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Pre-Deployment Steps](#pre-deployment-steps)
- [Deployment](#deployment)
- [Service Migration](#service-migration)
- [Testing](#testing)
- [Monitoring](#monitoring)
- [Rollback](#rollback)
- [Troubleshooting](#troubleshooting)

## Overview

### Why PgBouncer?

**Current State:**
- Services connect directly to PostgreSQL
- Total connection requirement: ~220 connections across 6 services
- PostgreSQL default max_connections: 100
- Expected load: 1000+ concurrent users

**Problem:**
Without connection pooling, each service maintains dedicated database connections, which can:
- Exhaust PostgreSQL connection limits
- Cause connection failures during high traffic
- Waste resources with idle connections
- Create performance bottlenecks

**Solution:**
PgBouncer acts as a connection pooler between applications and PostgreSQL:
- Reduces actual PostgreSQL connections from 220 → ~100
- Maintains connection pool for efficient reuse
- Handles 500+ client connections
- Improves performance and scalability

### Architecture

```
Before:
┌─────────────┐
│  user-bff   │────┐
└─────────────┘    │
┌─────────────┐    │
│  trade-bff  │────┤
└─────────────┘    │
┌─────────────┐    ├──► PostgreSQL (100 max connections) ❌
│  admin-bff  │────┤
└─────────────┘    │
┌─────────────┐    │
│trading-eng. │────┤
└─────────────┘    │
┌─────────────┐    │
│leaderboard- │────┘
│   worker    │
└─────────────┘

After:
┌─────────────┐
│  user-bff   │────┐
└─────────────┘    │
┌─────────────┐    │
│  trade-bff  │────┤
└─────────────┘    │    ┌──────────────┐
┌─────────────┐    ├───►│  PgBouncer   │──► PostgreSQL (100 max) ✅
│  admin-bff  │────┤    │  (pooler)    │
└─────────────┘    │    └──────────────┘
┌─────────────┐    │     • 500 client connections
│trading-eng. │────┤     • ~100 server connections
└─────────────┘    │     • Transaction pooling
┌─────────────┐    │
│leaderboard- │────┘
│   worker    │
└─────────────┘
```

## Prerequisites

1. **Kubernetes cluster** with namespace `tragge`
2. **PostgreSQL running** in the cluster (StatefulSet `postgres`)
3. **kubectl access** with appropriate permissions
4. **Backup** of current database (recommended)

## Pre-Deployment Steps

### 1. Generate Production Passwords

The default passwords in `pgbouncer-secrets.yaml` are placeholders. Generate secure passwords for production.

#### Option A: Generate MD5 Passwords (for auth_type = md5)

```bash
# Function to generate PgBouncer MD5 password
generate_pgbouncer_md5() {
    local username=$1
    local password=$2
    echo -n "${password}${username}" | md5sum | awk '{print "md5"$1}'
}

# Example: Generate password for 'app' user
APP_PASSWORD="YOUR_SECURE_PASSWORD_HERE"
generate_pgbouncer_md5 "app" "$APP_PASSWORD"
# Output: md5<hash>

# Example: Generate password for 'postgres' admin user
POSTGRES_PASSWORD="YOUR_POSTGRES_PASSWORD"
generate_pgbouncer_md5 "postgres" "$POSTGRES_PASSWORD"
# Output: md5<hash>
```

Update `infra/k8s/base/pgbouncer-secrets.yaml`:

```yaml
userlist.txt: |
  "app" "md5<your_generated_hash_for_app>"
  "postgres" "md5<your_generated_hash_for_postgres>"
```

#### Option B: Use SCRAM-SHA-256 (more secure, PostgreSQL 10+)

```bash
# Connect to PostgreSQL
kubectl exec -it -n tragge postgres-0 -- psql -U postgres -d app

# Get SCRAM hashes
SELECT rolname, rolpassword FROM pg_authid WHERE rolname IN ('app', 'postgres');
```

Update `infra/k8s/base/pgbouncer-config.yaml`:
```ini
auth_type = scram-sha-256
```

Update `infra/k8s/base/pgbouncer-secrets.yaml`:
```yaml
userlist.txt: |
  "app" "SCRAM-SHA-256$<iterations>:<salt>$<storedKey>:<serverKey>"
  "postgres" "SCRAM-SHA-256$<iterations>:<salt>$<storedKey>:<serverKey>"
```

### 2. Configure PostgreSQL

Increase PostgreSQL max_connections to accommodate PgBouncer pools:

```bash
# Edit PostgreSQL configuration
kubectl exec -it -n tragge postgres-0 -- psql -U postgres -c "ALTER SYSTEM SET max_connections = 110;"

# Restart PostgreSQL (requires downtime)
kubectl rollout restart -n tragge statefulset/postgres

# Verify
kubectl exec -it -n tragge postgres-0 -- psql -U postgres -c "SHOW max_connections;"
```

**Explanation:**
- PgBouncer max_db_connections: 100
- PostgreSQL needs: 100 + 10 (for admin/superuser reserved connections)
- Setting to 110 provides headroom

### 3. Verify Current Connection Usage

Before deploying, check current PostgreSQL connection count:

```bash
kubectl exec -it -n tragge postgres-0 -- psql -U postgres -d app -c "
  SELECT
    datname,
    COUNT(*) as connections,
    MAX(state) as max_state
  FROM pg_stat_activity
  WHERE datname = 'app'
  GROUP BY datname;
"
```

## Deployment

### Step 1: Apply PgBouncer Resources

```bash
# Navigate to repository root
cd /home/user/tragge

# Preview the deployment
kubectl kustomize infra/k8s/base | grep -A 50 "kind: Deployment" | grep -A 50 "pgbouncer"

# Apply PgBouncer resources
kubectl apply -k infra/k8s/base

# Or apply individually for more control
kubectl apply -f infra/k8s/base/pgbouncer-config.yaml
kubectl apply -f infra/k8s/base/pgbouncer-secrets.yaml
kubectl apply -f infra/k8s/base/pgbouncer.yaml
```

### Step 2: Verify Deployment

```bash
# Check PgBouncer pods are running
kubectl get pods -n tragge -l app.kubernetes.io/name=pgbouncer

# Expected output:
# NAME                         READY   STATUS    RESTARTS   AGE
# pgbouncer-xxxxxxxxxx-xxxxx   2/2     Running   0          1m
# pgbouncer-xxxxxxxxxx-xxxxx   2/2     Running   0          1m

# Check service is created
kubectl get svc -n tragge pgbouncer

# Expected output:
# NAME        TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)             AGE
# pgbouncer   ClusterIP   10.96.xxx.xxx   <none>        6432/TCP,9127/TCP   1m

# View PgBouncer logs
kubectl logs -n tragge -l app.kubernetes.io/name=pgbouncer -c pgbouncer --tail=50

# Should see:
# 2024-01-06 12:00:00.000 UTC [1] LOG C-0x0: pgbouncer/app@pgbouncer:6432 logged in
# 2024-01-06 12:00:00.000 UTC [1] LOG Stats: 0 xacts/s, 0 queries/s
```

### Step 3: Test PgBouncer Connectivity

```bash
# Port-forward to access PgBouncer locally
kubectl port-forward -n tragge svc/pgbouncer 6432:6432 &

# Test connection to app database
PGPASSWORD="YOUR_APP_PASSWORD" psql -h localhost -p 6432 -U app -d app -c "SELECT 1;"

# Test admin console
PGPASSWORD="YOUR_POSTGRES_PASSWORD" psql -h localhost -p 6432 -U postgres -d pgbouncer -c "SHOW POOLS;"

# Expected output:
#  database |   user    | cl_active | cl_waiting | sv_active | sv_idle | sv_used | ...
# ----------+-----------+-----------+------------+-----------+---------+---------+-----
#  app      | app       | 1         | 0          | 1         | 0       | 1       | ...
#  pgbouncer| postgres  | 1         | 0          | 0         | 0       | 1       | ...

# Kill port-forward when done
pkill -f "port-forward.*pgbouncer"
```

## Service Migration

### Migration Strategy

We'll use a **rolling migration** approach:
1. Migrate one service at a time
2. Test each service before moving to the next
3. Monitor for issues
4. Keep ability to rollback

### Connection String Changes

**Before (direct PostgreSQL):**
```
postgres://app:app@postgres:5432/app?sslmode=disable
```

**After (via PgBouncer):**
```
postgres://app:app@pgbouncer:6432/app?sslmode=disable
```

**Change:** `postgres:5432` → `pgbouncer:6432`

### Update Secrets

Create a new secret with PgBouncer connection string:

```bash
# Create new secret file
cat > /tmp/tragge-secrets-pgbouncer.yaml <<'EOF'
apiVersion: v1
kind: Secret
metadata:
  name: tragge-secrets
  namespace: tragge
  labels:
    app.kubernetes.io/name: tragge
    app.kubernetes.io/part-of: tragge-platform
type: Opaque
stringData:
  # Database credentials (via PgBouncer)
  POSTGRES_DSN: "postgres://app:YOUR_PASSWORD@pgbouncer:6432/app?sslmode=disable"

  # JWT secret for authentication
  JWT_SECRET: "YOUR_JWT_SECRET"

  # Market data API keys
  TWELVEDATA_API_KEY: "YOUR_TWELVEDATA_KEY"
  MASSIVE_API_KEY: "YOUR_MASSIVE_KEY"
EOF

# Apply the updated secret
kubectl apply -f /tmp/tragge-secrets-pgbouncer.yaml

# Clean up
rm /tmp/tragge-secrets-pgbouncer.yaml
```

### Migrate Services One by One

Since we updated the secret, we need to restart services for them to pick up the new DSN:

```bash
# 1. Migrate user-bff (lowest priority)
kubectl rollout restart -n tragge deployment/user-bff
kubectl rollout status -n tragge deployment/user-bff

# Test user-bff
kubectl logs -n tragge -l app.kubernetes.io/name=user-bff --tail=20 | grep -i "database\|postgres\|connect"

# 2. Migrate admin-bff (low priority)
kubectl rollout restart -n tragge deployment/admin-bff
kubectl rollout status -n tragge deployment/admin-bff

# 3. Migrate market-ingestor (medium priority)
kubectl rollout restart -n tragge deployment/market-ingestor
kubectl rollout status -n tragge deployment/market-ingestor

# 4. Migrate leaderboard-worker (medium priority)
kubectl rollout restart -n tragge deployment/leaderboard-worker
kubectl rollout status -n tragge deployment/leaderboard-worker

# 5. Migrate trade-bff (high priority - user-facing)
kubectl rollout restart -n tragge deployment/trade-bff
kubectl rollout status -n tragge deployment/trade-bff

# 6. Migrate trading-engine (critical - trading functionality)
kubectl rollout restart -n tragge deployment/trading-engine
kubectl rollout status -n tragge deployment/trading-engine
```

### Verification After Each Service

After migrating each service, verify it's working:

```bash
SERVICE_NAME="user-bff"  # Replace with service name

# Check pod status
kubectl get pods -n tragge -l app.kubernetes.io/name=$SERVICE_NAME

# Check logs for database connections
kubectl logs -n tragge -l app.kubernetes.io/name=$SERVICE_NAME --tail=50

# Check health endpoints
kubectl exec -n tragge deploy/$SERVICE_NAME -- wget -O- http://localhost:8081/healthz
kubectl exec -n tragge deploy/$SERVICE_NAME -- wget -O- http://localhost:8081/readyz
```

## Testing

### 1. Functional Testing

Test each service's database operations:

```bash
# user-bff: Register a new user
curl -X POST http://gateway.tragge.svc:8080/api/user/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"Test123!","username":"testuser"}'

# user-bff: Login
curl -X POST http://gateway.tragge.svc:8080/api/user/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"Test123!"}'

# admin-bff: Create contest (requires admin token)
curl -X POST http://gateway.tragge.svc:8080/api/admin/contests \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Test Contest","start_time":"2024-01-10T00:00:00Z","end_time":"2024-01-17T00:00:00Z"}'
```

### 2. Connection Pool Testing

Monitor PgBouncer connection pools:

```bash
# Connect to PgBouncer admin console
kubectl exec -it -n tragge deploy/pgbouncer -c pgbouncer -- \
  psql -h localhost -p 6432 -U postgres -d pgbouncer

# Run monitoring queries:

-- Show pool statistics
SHOW POOLS;

-- Check for waiting clients (should be 0)
SELECT * FROM pgbouncer.pools WHERE cl_waiting > 0;

-- Show active server connections (should be < 100)
SELECT database, COUNT(*) as conn_count
FROM pgbouncer.servers
WHERE state = 'active'
GROUP BY database;

-- Show all clients
SHOW CLIENTS;

-- Show all servers
SHOW SERVERS;

-- Show stats
SHOW STATS;
```

### 3. Load Testing

Run load tests to verify pool behavior under stress:

```bash
# WebSocket load test
cd tools/ws-load-test
go run . \
  -email test@example.com \
  -password Test123! \
  -contest-id YOUR_CONTEST_ID \
  -n 100 \
  -duration 60s

# Order load test
cd tools/order-load-test
go run . \
  -contest-id YOUR_CONTEST_ID \
  -users 50 \
  -duration 120s

# Monitor PgBouncer during load test
watch -n 2 'kubectl exec -n tragge deploy/pgbouncer -c pgbouncer -- \
  psql -h localhost -p 6432 -U postgres -d pgbouncer -c "SHOW POOLS"'
```

### 4. Check PostgreSQL Connection Count

Verify that PostgreSQL connections are reduced:

```bash
kubectl exec -it -n tragge postgres-0 -- psql -U postgres -d app -c "
  SELECT
    COUNT(*) as total_connections,
    COUNT(*) FILTER (WHERE state = 'active') as active,
    COUNT(*) FILTER (WHERE state = 'idle') as idle
  FROM pg_stat_activity
  WHERE datname = 'app';
"

# Before PgBouncer: ~220 connections
# After PgBouncer: ~20-30 connections (from PgBouncer pool)
```

## Monitoring

### Prometheus Metrics

PgBouncer exporter exposes metrics on port 9127:

```bash
# Check metrics endpoint
kubectl port-forward -n tragge svc/pgbouncer 9127:9127
curl http://localhost:9127/metrics

# Key metrics to monitor:
# - pgbouncer_pools_server_active_connections
# - pgbouncer_pools_server_idle_connections
# - pgbouncer_pools_client_active_connections
# - pgbouncer_pools_client_waiting_connections
# - pgbouncer_stats_queries_total
# - pgbouncer_stats_transactions_total
```

### Grafana Dashboard

Create alerts for PgBouncer:

```yaml
# Alert: High client wait time
- alert: PgBouncerHighClientWait
  expr: pgbouncer_pools_client_waiting_connections > 10
  for: 5m
  annotations:
    summary: "PgBouncer has waiting clients"
    description: "{{ $value }} clients waiting for connections"

# Alert: Pool exhaustion
- alert: PgBouncerPoolExhaustion
  expr: pgbouncer_pools_server_active_connections / pgbouncer_config_max_db_connections > 0.9
  for: 5m
  annotations:
    summary: "PgBouncer pool near exhaustion"
    description: "{{ $value }}% of max_db_connections used"
```

### Logging

Monitor PgBouncer logs for issues:

```bash
# Follow logs
kubectl logs -f -n tragge -l app.kubernetes.io/name=pgbouncer -c pgbouncer

# Search for errors
kubectl logs -n tragge -l app.kubernetes.io/name=pgbouncer -c pgbouncer --tail=1000 | grep -i "error\|warning\|failed"
```

## Rollback

If issues occur, you can quickly rollback to direct PostgreSQL connections:

### Option 1: Rollback Secret (Fast)

```bash
# Restore original secret
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: tragge-secrets
  namespace: tragge
type: Opaque
stringData:
  POSTGRES_DSN: "postgres://app:YOUR_PASSWORD@postgres:5432/app?sslmode=disable"
  JWT_SECRET: "YOUR_JWT_SECRET"
  TWELVEDATA_API_KEY: "YOUR_TWELVEDATA_KEY"
  MASSIVE_API_KEY: "YOUR_MASSIVE_KEY"
EOF

# Restart all services
kubectl rollout restart -n tragge deployment/user-bff
kubectl rollout restart -n tragge deployment/trade-bff
kubectl rollout restart -n tragge deployment/admin-bff
kubectl rollout restart -n tragge deployment/trading-engine
kubectl rollout restart -n tragge deployment/market-ingestor
kubectl rollout restart -n tragge deployment/leaderboard-worker
```

### Option 2: Delete PgBouncer (if needed)

```bash
# Delete PgBouncer deployment
kubectl delete -f infra/k8s/base/pgbouncer.yaml

# Services will automatically fail over to direct PostgreSQL connections
```

### Verification After Rollback

```bash
# Check services are healthy
kubectl get pods -n tragge

# Verify PostgreSQL connections
kubectl exec -it -n tragge postgres-0 -- psql -U postgres -d app -c "
  SELECT application_name, COUNT(*)
  FROM pg_stat_activity
  WHERE datname = 'app'
  GROUP BY application_name;
"
```

## Troubleshooting

### Issue: Services can't connect to PgBouncer

**Symptoms:**
- Pods in CrashLoopBackOff
- Logs show "connection refused" or "could not connect to server"

**Solutions:**

```bash
# 1. Check PgBouncer pods are running
kubectl get pods -n tragge -l app.kubernetes.io/name=pgbouncer

# 2. Check PgBouncer service exists
kubectl get svc -n tragge pgbouncer

# 3. Test connectivity from a service pod
kubectl exec -it -n tragge deploy/user-bff -- nc -zv pgbouncer 6432

# 4. Check PgBouncer logs
kubectl logs -n tragge -l app.kubernetes.io/name=pgbouncer -c pgbouncer --tail=100
```

### Issue: Authentication failures

**Symptoms:**
- Logs show "password authentication failed"
- "no such user" errors

**Solutions:**

```bash
# 1. Verify userlist.txt has correct credentials
kubectl get secret -n tragge pgbouncer-secrets -o jsonpath='{.data.userlist\.txt}' | base64 -d

# 2. Regenerate MD5 passwords
echo -n "YOUR_PASSWORDapp" | md5sum | awk '{print "md5"$1}'

# 3. Update secret
kubectl edit secret -n tragge pgbouncer-secrets

# 4. Restart PgBouncer
kubectl rollout restart -n tragge deployment/pgbouncer
```

### Issue: High client wait times

**Symptoms:**
- SHOW POOLS shows cl_waiting > 0
- Application timeouts
- Slow queries

**Solutions:**

```bash
# 1. Check pool configuration
kubectl exec -n tragge deploy/pgbouncer -c pgbouncer -- \
  psql -h localhost -p 6432 -U postgres -d pgbouncer -c "SHOW CONFIG" | grep pool

# 2. Increase pool size (temporary)
kubectl exec -n tragge deploy/pgbouncer -c pgbouncer -- \
  psql -h localhost -p 6432 -U postgres -d pgbouncer -c "SET default_pool_size = 30"

# 3. Make permanent change
kubectl edit configmap -n tragge pgbouncer-config
# Change: default_pool_size = 30

kubectl rollout restart -n tragge deployment/pgbouncer
```

### Issue: PgBouncer can't connect to PostgreSQL

**Symptoms:**
- PgBouncer logs show "could not connect to server"
- Readiness probe failing

**Solutions:**

```bash
# 1. Verify PostgreSQL is running
kubectl get pods -n tragge -l app.kubernetes.io/name=postgres

# 2. Test PostgreSQL connectivity from PgBouncer pod
kubectl exec -it -n tragge deploy/pgbouncer -c pgbouncer -- nc -zv postgres 5432

# 3. Check PostgreSQL logs
kubectl logs -n tragge postgres-0 --tail=100

# 4. Verify PostgreSQL service
kubectl get svc -n tragge postgres
```

### Issue: Pool exhaustion

**Symptoms:**
- Metrics show server connections near max_db_connections
- Errors in logs: "no more connections allowed"

**Solutions:**

```bash
# Option 1: Increase PostgreSQL max_connections
kubectl exec -it -n tragge postgres-0 -- psql -U postgres -c "ALTER SYSTEM SET max_connections = 150;"
kubectl rollout restart -n tragge statefulset/postgres

# Option 2: Increase PgBouncer max_db_connections
kubectl edit configmap -n tragge pgbouncer-config
# Change: max_db_connections = 150
kubectl rollout restart -n tragge deployment/pgbouncer

# Option 3: Optimize pool sizes per service (reduce default_pool_size)
kubectl edit configmap -n tragge pgbouncer-config
# Change: default_pool_size = 15
kubectl rollout restart -n tragge deployment/pgbouncer
```

## Performance Tuning

### Adjust Pool Sizes Based on Service Needs

Create per-database pool size configuration:

```bash
kubectl edit configmap -n tragge pgbouncer-config
```

Add to `[databases]` section:
```ini
app = host=postgres port=5432 dbname=app pool_size=25 min_pool_size=10
```

### Monitor and Tune

Use these queries to understand usage patterns:

```sql
-- Average transactions per second by service
SELECT
  database,
  user,
  total_xact_time / NULLIF(total_xact_count, 0) as avg_xact_time_ms
FROM pgbouncer.stats;

-- Pool efficiency (should be low)
SELECT
  database,
  cl_active,
  sv_active,
  CASE
    WHEN sv_active > 0 THEN (cl_active::float / sv_active::float)
    ELSE 0
  END as connection_multiplier
FROM pgbouncer.pools;
```

---

## Summary

✅ **Deployment Checklist:**
- [ ] Generate production passwords
- [ ] Update pgbouncer-secrets.yaml
- [ ] Configure PostgreSQL max_connections
- [ ] Deploy PgBouncer resources
- [ ] Verify PgBouncer is running
- [ ] Test PgBouncer connectivity
- [ ] Update tragge-secrets with new DSN
- [ ] Migrate services one by one
- [ ] Run functional tests
- [ ] Run load tests
- [ ] Configure monitoring alerts
- [ ] Document any issues

✅ **Post-Migration:**
- Monitor connection pool metrics
- Set up Grafana dashboards
- Configure alerts for pool exhaustion
- Review and tune pool sizes based on actual usage
- Document any issues or optimizations

For additional help, see:
- PgBouncer documentation: https://www.pgbouncer.org/
- PgBouncer configuration reference: https://www.pgbouncer.org/config.html
- PostgreSQL connection pooling best practices: https://wiki.postgresql.org/wiki/Number_Of_Database_Connections
