# PostgreSQL High Availability Runbook

This runbook covers operational procedures for the PostgreSQL HA cluster with Patroni, PgBouncer, and etcd.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Quick Reference Commands](#quick-reference-commands)
3. [Failover Procedures](#failover-procedures)
4. [Manual Promotion](#manual-promotion)
5. [Backup Strategy](#backup-strategy)
6. [Migration from Single Instance](#migration-from-single-instance)
7. [Troubleshooting](#troubleshooting)
8. [Maintenance Procedures](#maintenance-procedures)

---

## Architecture Overview

```
                    ┌─────────────────────────────────────────┐
                    │           Application Layer              │
                    │  (user-bff, trade-bff, admin-bff, etc.) │
                    └─────────────┬───────────────────────────┘
                                  │
                    ┌─────────────▼───────────────┐
                    │       Connection Pool        │
                    │         (PgBouncer)          │
                    └──────┬──────────────┬───────┘
                           │              │
              ┌────────────▼──┐    ┌──────▼────────────┐
              │ pgbouncer-    │    │   pgbouncer-      │
              │ primary:6432  │    │   replica:6432    │
              │ (read/write)  │    │   (read-only)     │
              └───────┬───────┘    └────────┬──────────┘
                      │                     │
                      ▼                     ▼
     ┌────────────────────────────────────────────────────────┐
     │                  PostgreSQL Cluster                     │
     │  ┌──────────┐   ┌──────────┐   ┌──────────┐           │
     │  │postgres- │   │postgres- │   │postgres- │           │
     │  │ha-0      │   │ha-1      │   │ha-2      │           │
     │  │(primary) │◀──│(replica) │   │(replica) │           │
     │  │          │──▶│          │◀──│          │           │
     │  └────┬─────┘   └────┬─────┘   └────┬─────┘           │
     │       │              │              │                  │
     │       └──────────────┼──────────────┘                  │
     │                      │                                  │
     │           Streaming Replication                        │
     └────────────────────────────────────────────────────────┘
                            │
                            ▼
     ┌────────────────────────────────────────────────────────┐
     │                    etcd Cluster                         │
     │     (Distributed Consensus for Leader Election)         │
     │  ┌─────────┐    ┌─────────┐    ┌─────────┐            │
     │  │ etcd-0  │◀──▶│ etcd-1  │◀──▶│ etcd-2  │            │
     │  └─────────┘    └─────────┘    └─────────┘            │
     └────────────────────────────────────────────────────────┘
```

### Components

| Component | Purpose | Replicas |
|-----------|---------|----------|
| PostgreSQL + Patroni | Database with HA management | 3 (1 primary + 2 replicas) |
| etcd | Distributed consensus for leader election | 3 |
| PgBouncer Primary | Connection pooling for writes | 2 |
| PgBouncer Replica | Connection pooling for reads | 2 |
| postgres_exporter | Prometheus metrics sidecar | 1 per PostgreSQL pod |

### Connection Strings

| Use Case | Connection String |
|----------|-------------------|
| Read/Write | `postgres://app:password@pgbouncer-primary:6432/app` |
| Read-Only | `postgres://app:password@pgbouncer-replica:6432/app` |
| Direct Primary | `postgres://app:password@postgres-primary:5432/app` |
| Direct Replica | `postgres://app:password@postgres-replica:5432/app` |

---

## Quick Reference Commands

### Cluster Status

```bash
# Check Patroni cluster status
kubectl exec -n tragge postgres-ha-0 -- patronictl list

# Check replication lag
kubectl exec -n tragge postgres-ha-0 -- psql -U app -d app -c \
  "SELECT client_addr, state, sent_lsn, write_lsn, flush_lsn, replay_lsn,
          pg_wal_lsn_diff(sent_lsn, replay_lsn) AS lag_bytes
   FROM pg_stat_replication;"

# Check replication lag in seconds
kubectl exec -n tragge postgres-ha-1 -- psql -U app -d app -c \
  "SELECT EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp())) AS lag_seconds;"

# Check etcd cluster health
kubectl exec -n tragge etcd-0 -- etcdctl endpoint health

# List etcd members
kubectl exec -n tragge etcd-0 -- etcdctl member list
```

### Pod Management

```bash
# Get all PostgreSQL HA pods
kubectl get pods -n tragge -l app=postgres-ha

# Get PgBouncer pods
kubectl get pods -n tragge -l app=pgbouncer

# Get etcd pods
kubectl get pods -n tragge -l app=etcd

# Check pod logs
kubectl logs -n tragge postgres-ha-0 -c postgres
kubectl logs -n tragge postgres-ha-0 -c postgres-exporter
```

---

## Failover Procedures

### Automatic Failover

Patroni handles automatic failover when:
1. The primary becomes unreachable
2. The primary fails health checks
3. The primary is manually demoted

**Failover process:**
1. Patroni detects primary failure (within 30s TTL)
2. etcd leader election selects new primary
3. Most up-to-date replica is promoted
4. Other replicas follow new primary
5. PgBouncer routes traffic to new primary

### Manual Failover (Switchover)

Use switchover for planned maintenance or to balance load.

```bash
# Perform switchover to specific member
kubectl exec -n tragge postgres-ha-0 -- patronictl switchover \
  --master postgres-ha-0 \
  --candidate postgres-ha-1 \
  --force

# Or interactive switchover
kubectl exec -it -n tragge postgres-ha-0 -- patronictl switchover
```

**Pre-switchover checklist:**
- [ ] All replicas are streaming with minimal lag
- [ ] No long-running transactions on primary
- [ ] Application can handle brief interruption
- [ ] Monitoring in place to verify success

### Failover Testing Procedure

Test failover regularly to ensure it works in production.

```bash
# Step 1: Record current state
kubectl exec -n tragge postgres-ha-0 -- patronictl list

# Step 2: Identify current primary
PRIMARY_POD=$(kubectl exec -n tragge postgres-ha-0 -- \
  patronictl list -f json | jq -r '.[] | select(.Role == "Leader") | .Member')

# Step 3: Simulate primary failure (delete pod)
kubectl delete pod -n tragge $PRIMARY_POD

# Step 4: Monitor failover (should complete within 30-60s)
watch kubectl exec -n tragge postgres-ha-0 -- patronictl list

# Step 5: Verify new primary is functional
kubectl exec -n tragge postgres-ha-0 -- psql -U app -d app -c "SELECT 1;"

# Step 6: Verify application connectivity
curl -s http://user-bff:8081/readyz
```

---

## Manual Promotion

In emergency situations, you may need to manually promote a replica.

### Using Patroni (Recommended)

```bash
# Promote specific replica
kubectl exec -n tragge postgres-ha-0 -- patronictl failover \
  --candidate postgres-ha-1 \
  --force

# Emergency failover (skips confirmation)
kubectl exec -n tragge postgres-ha-0 -- patronictl failover --force
```

### Direct PostgreSQL Promotion (Emergency Only)

Only use this if Patroni is unavailable.

```bash
# Step 1: Stop Patroni on the replica to promote
kubectl exec -n tragge postgres-ha-1 -- supervisorctl stop patroni

# Step 2: Promote the replica
kubectl exec -n tragge postgres-ha-1 -- \
  /usr/lib/postgresql/16/bin/pg_ctl promote -D /var/lib/postgresql/data/pgdata

# Step 3: Update PgBouncer to point to new primary
kubectl edit configmap pgbouncer-config -n tragge
# Change postgres-primary host to postgres-ha-1

# Step 4: Restart PgBouncer
kubectl rollout restart deployment pgbouncer-primary -n tragge

# Step 5: Fix Patroni cluster (may require reinitializing)
kubectl exec -n tragge postgres-ha-1 -- patronictl reinit postgres-ha-0 --force
```

### Post-Promotion Verification

```bash
# Verify new primary accepts writes
kubectl exec -n tragge postgres-ha-1 -- psql -U app -d app -c \
  "INSERT INTO audit_logs (actor_user_id, action, target_type)
   VALUES ('00000000-0000-0000-0000-000000000000', 'failover_test', 'system');"

# Verify replicas are following
kubectl exec -n tragge postgres-ha-0 -- patronictl list

# Check application health
for svc in user-bff trade-bff admin-bff; do
  echo "Checking $svc..."
  kubectl exec -n tragge deployment/$svc -- curl -s http://localhost:8081/readyz
done
```

---

## Backup Strategy

### Backup Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  PostgreSQL     │     │  Backup Job     │     │  S3 / MinIO     │
│  Primary        │────▶│  (pg_basebackup │────▶│  Bucket         │
│                 │     │   + WAL archive)│     │                 │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

### Backup Types

| Type | Frequency | Retention | Method |
|------|-----------|-----------|--------|
| WAL Archive | Continuous | 7 days | pg_receivewal |
| Base Backup | Daily | 30 days | pg_basebackup |
| Logical Backup | Weekly | 90 days | pg_dump |

### Running Backups

```bash
# Trigger base backup manually
kubectl create job -n tragge manual-backup-$(date +%Y%m%d) \
  --from=cronjob/postgres-backup

# Check backup status
kubectl logs -n tragge job/manual-backup-$(date +%Y%m%d)

# List available backups (from S3)
aws s3 ls s3://tragge-backups/postgres/ --recursive
```

### Backup from Replica

To avoid impacting primary performance, run backups from replica:

```bash
# Run pg_basebackup from replica
kubectl exec -n tragge postgres-ha-1 -- pg_basebackup \
  -D /tmp/backup \
  -Ft -z \
  -P \
  -X fetch \
  -U replicator

# Upload to S3
kubectl exec -n tragge postgres-ha-1 -- \
  aws s3 cp /tmp/backup.tar.gz s3://tragge-backups/postgres/$(date +%Y%m%d)/
```

### Point-in-Time Recovery (PITR)

```bash
# Step 1: Stop all application traffic
kubectl scale deployment user-bff trade-bff admin-bff --replicas=0 -n tragge

# Step 2: Restore base backup
kubectl exec -n tragge postgres-ha-0 -- \
  pg_basebackup -D /var/lib/postgresql/data/pgdata-restored \
  -Ft -z

# Step 3: Configure recovery target
kubectl exec -n tragge postgres-ha-0 -- bash -c 'cat > /var/lib/postgresql/data/pgdata-restored/recovery.signal'
kubectl exec -n tragge postgres-ha-0 -- bash -c 'cat >> /var/lib/postgresql/data/pgdata-restored/postgresql.auto.conf << EOF
recovery_target_time = '\''2024-01-15 14:30:00 UTC'\''
recovery_target_action = '\''promote'\''
restore_command = '\''cp /mnt/wal-archive/%f %p'\''
EOF'

# Step 4: Start PostgreSQL with recovered data
kubectl exec -n tragge postgres-ha-0 -- \
  pg_ctl start -D /var/lib/postgresql/data/pgdata-restored

# Step 5: Verify data and resume application
kubectl scale deployment user-bff trade-bff admin-bff --replicas=2 -n tragge
```

---

## Migration from Single Instance

### Pre-Migration Checklist

- [ ] Current database backup verified
- [ ] Application downtime window scheduled
- [ ] New HA cluster deployed and tested
- [ ] Connection strings updated in application configs
- [ ] Rollback plan documented

### Migration Steps

#### Step 1: Deploy HA Infrastructure

```bash
# Deploy etcd cluster
kubectl apply -f infra/k8s/base/postgres-ha.yaml

# Wait for etcd to be ready
kubectl rollout status statefulset/etcd -n tragge

# Wait for PostgreSQL HA to be ready
kubectl rollout status statefulset/postgres-ha -n tragge

# Verify cluster status
kubectl exec -n tragge postgres-ha-0 -- patronictl list
```

#### Step 2: Prepare Source Database

```bash
# On current single PostgreSQL instance
# Stop accepting new connections
kubectl exec -n tragge postgres-0 -- psql -U app -c \
  "ALTER DATABASE app CONNECTION LIMIT 0;"

# Wait for active transactions to complete
kubectl exec -n tragge postgres-0 -- psql -U app -c \
  "SELECT pid, now() - xact_start AS duration, query
   FROM pg_stat_activity
   WHERE datname = 'app' AND state = 'active';"

# Create final backup
kubectl exec -n tragge postgres-0 -- pg_dump -U app -Fc app > /tmp/app_final.dump
```

#### Step 3: Migrate Data

```bash
# Copy dump to new cluster
kubectl cp /tmp/app_final.dump tragge/postgres-ha-0:/tmp/app_final.dump

# Restore to new cluster
kubectl exec -n tragge postgres-ha-0 -- pg_restore \
  -U admin -d app -Fc --clean --if-exists /tmp/app_final.dump

# Verify data integrity
kubectl exec -n tragge postgres-ha-0 -- psql -U app -d app -c \
  "SELECT schemaname, tablename, n_live_tup
   FROM pg_stat_user_tables
   ORDER BY n_live_tup DESC;"
```

#### Step 4: Update Application Configuration

Update environment variables or ConfigMaps:

```yaml
# Before (single instance)
POSTGRES_DSN: postgres://app:password@postgres:5432/app?sslmode=disable

# After (HA with read/write splitting)
POSTGRES_PRIMARY_DSN: postgres://app:password@pgbouncer-primary:6432/app?sslmode=disable
POSTGRES_REPLICA_DSN: postgres://app:password@pgbouncer-replica:6432/app?sslmode=disable
```

#### Step 5: Update Application Code

Use the new `packages/db` read/write splitting:

```go
import "github.com/Parsaeffatravesh/tragge/packages/db"

func main() {
    pool, err := db.NewPool(ctx, db.Config{
        PrimaryDSN:  os.Getenv("POSTGRES_PRIMARY_DSN"),
        ReplicaDSNs: []string{os.Getenv("POSTGRES_REPLICA_DSN")},
        MaxReplicationLag: 10 * time.Second,
        RetryOnLag: true,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer pool.Close()

    // Write operations - use Primary
    _, err = pool.Primary().ExecContext(ctx,
        "INSERT INTO orders ...")

    // Read operations - use Replica
    rows, err := pool.Replica().QueryContext(ctx,
        "SELECT * FROM leaderboard...")

    // Read-after-write - use Primary
    row := pool.Primary().QueryRowContext(ctx,
        "SELECT * FROM orders WHERE id = ?", newOrderID)
}
```

#### Step 6: Deploy and Verify

```bash
# Rolling restart of applications
kubectl rollout restart deployment user-bff trade-bff admin-bff -n tragge

# Monitor for errors
kubectl logs -f -l app=user-bff -n tragge | grep -i error

# Verify readiness
for svc in user-bff trade-bff admin-bff; do
  kubectl exec -n tragge deployment/$svc -- curl -s localhost:8081/readyz
done
```

#### Step 7: Cleanup

```bash
# Remove old single instance (after verification period)
kubectl delete statefulset postgres -n tragge
kubectl delete pvc postgres-data-postgres-0 -n tragge
```

---

## Troubleshooting

### Replication Lag

**Symptoms:** Replica lag exceeds threshold, alerts firing

**Diagnosis:**
```bash
# Check lag on each replica
for pod in postgres-ha-1 postgres-ha-2; do
  echo "=== $pod ==="
  kubectl exec -n tragge $pod -- psql -U app -c \
    "SELECT EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp())) AS lag_seconds;"
done

# Check WAL sender status on primary
kubectl exec -n tragge postgres-ha-0 -- psql -U app -c \
  "SELECT * FROM pg_stat_replication;"

# Check for long-running transactions blocking replication
kubectl exec -n tragge postgres-ha-0 -- psql -U app -c \
  "SELECT pid, now() - xact_start AS duration, query
   FROM pg_stat_activity
   WHERE state = 'active'
   ORDER BY duration DESC
   LIMIT 10;"
```

**Resolution:**
1. Check network connectivity between primary and replicas
2. Verify replica has sufficient I/O capacity
3. Kill long-running transactions if safe
4. Increase `wal_sender_timeout` if network is unstable

### No Replicas Connected

**Symptoms:** Primary has no streaming replicas

**Diagnosis:**
```bash
# Check replication slots
kubectl exec -n tragge postgres-ha-0 -- psql -U app -c \
  "SELECT * FROM pg_replication_slots;"

# Check replica status
kubectl exec -n tragge postgres-ha-1 -- psql -U app -c \
  "SELECT pg_is_in_recovery();"

# Check Patroni logs
kubectl logs -n tragge postgres-ha-1 -c postgres | tail -50
```

**Resolution:**
```bash
# Reinitialize replica from primary
kubectl exec -n tragge postgres-ha-0 -- patronictl reinit postgres-ha-1 --force

# If reinit fails, delete PVC and let Patroni recreate
kubectl delete pvc postgres-data-postgres-ha-1 -n tragge
kubectl delete pod postgres-ha-1 -n tragge
```

### PgBouncer Issues

**Symptoms:** Connection errors, pool exhaustion

**Diagnosis:**
```bash
# Check PgBouncer stats
kubectl exec -n tragge deployment/pgbouncer-primary -- \
  psql -U admin -p 6432 pgbouncer -c "SHOW POOLS;"

# Check active connections
kubectl exec -n tragge deployment/pgbouncer-primary -- \
  psql -U admin -p 6432 pgbouncer -c "SHOW CLIENTS;"

# Check server connections
kubectl exec -n tragge deployment/pgbouncer-primary -- \
  psql -U admin -p 6432 pgbouncer -c "SHOW SERVERS;"
```

**Resolution:**
```bash
# Reload PgBouncer config
kubectl exec -n tragge deployment/pgbouncer-primary -- \
  psql -U admin -p 6432 pgbouncer -c "RELOAD;"

# Force disconnect idle clients
kubectl exec -n tragge deployment/pgbouncer-primary -- \
  psql -U admin -p 6432 pgbouncer -c "KILL app;"

# Restart PgBouncer if needed
kubectl rollout restart deployment pgbouncer-primary -n tragge
```

### etcd Issues

**Symptoms:** Patroni cannot elect leader, split-brain risk

**Diagnosis:**
```bash
# Check etcd endpoint health
kubectl exec -n tragge etcd-0 -- etcdctl endpoint health

# Check etcd member status
kubectl exec -n tragge etcd-0 -- etcdctl member list

# Check etcd logs
kubectl logs -n tragge etcd-0 | tail -50
```

**Resolution:**
```bash
# If a member is unhealthy, remove and re-add
# Get member ID
MEMBER_ID=$(kubectl exec -n tragge etcd-0 -- etcdctl member list | grep unhealthy | cut -d',' -f1)

# Remove member
kubectl exec -n tragge etcd-0 -- etcdctl member remove $MEMBER_ID

# Delete and recreate pod
kubectl delete pod etcd-2 -n tragge
```

### Idle Transactions

**Symptoms:** Connections stuck in "idle in transaction"

**Resolution:**
```bash
# Find and terminate idle transactions older than 5 minutes
kubectl exec -n tragge postgres-ha-0 -- psql -U app -c "
  SELECT pg_terminate_backend(pid)
  FROM pg_stat_activity
  WHERE state = 'idle in transaction'
  AND now() - xact_start > interval '5 minutes';"
```

### Long Queries

**Symptoms:** Queries running for extended periods

**Resolution:**
```bash
# Identify and optionally cancel long-running queries
kubectl exec -n tragge postgres-ha-0 -- psql -U app -c "
  SELECT pid, now() - query_start AS duration, query
  FROM pg_stat_activity
  WHERE state = 'active'
  AND now() - query_start > interval '1 minute'
  ORDER BY duration DESC;"

# Cancel specific query (non-destructive)
kubectl exec -n tragge postgres-ha-0 -- psql -U app -c \
  "SELECT pg_cancel_backend(<PID>);"

# Terminate specific connection (immediate)
kubectl exec -n tragge postgres-ha-0 -- psql -U app -c \
  "SELECT pg_terminate_backend(<PID>);"
```

---

## Maintenance Procedures

### Rolling Restart

```bash
# Restart replicas first, then primary
kubectl rollout restart statefulset/postgres-ha -n tragge

# Monitor restart progress
kubectl rollout status statefulset/postgres-ha -n tragge
```

### PostgreSQL Minor Version Upgrade

```bash
# Step 1: Update image tag in manifest
kubectl edit statefulset/postgres-ha -n tragge

# Step 2: Perform rolling restart
kubectl rollout restart statefulset/postgres-ha -n tragge

# Step 3: Verify all pods running new version
kubectl get pods -n tragge -l app=postgres-ha -o jsonpath='{.items[*].spec.containers[0].image}'
```

### Adding a New Replica

```bash
# Scale up the StatefulSet
kubectl scale statefulset/postgres-ha --replicas=4 -n tragge

# Wait for new replica to sync
kubectl exec -n tragge postgres-ha-0 -- patronictl list

# Verify replication is streaming
kubectl exec -n tragge postgres-ha-3 -- psql -U app -c \
  "SELECT pg_is_in_recovery();"
```

### Removing a Replica

```bash
# Scale down (removes highest-numbered replica)
kubectl scale statefulset/postgres-ha --replicas=2 -n tragge

# Clean up orphaned PVC if needed
kubectl delete pvc postgres-data-postgres-ha-2 -n tragge
```

---

## Connection String Examples for Services

### Using packages/db with Read/Write Splitting

```go
package main

import (
    "context"
    "os"
    "time"

    "github.com/Parsaeffatravesh/tragge/packages/db"
)

func main() {
    ctx := context.Background()

    pool, err := db.NewPool(ctx, db.Config{
        PrimaryDSN: os.Getenv("POSTGRES_PRIMARY_DSN"),
        ReplicaDSNs: []string{
            os.Getenv("POSTGRES_REPLICA_DSN"),
        },
        MaxOpenConns:      25,
        MaxIdleConns:      5,
        ConnMaxLifetime:   5 * time.Minute,
        MaxReplicationLag: 10 * time.Second,
        LagCheckInterval:  5 * time.Second,
        RetryOnLag:        true,
    })
    if err != nil {
        panic(err)
    }
    defer pool.Close()

    // === WRITE OPERATIONS (use Primary) ===

    // Insert new order
    _, err = pool.Primary().ExecContext(ctx,
        "INSERT INTO orders (user_id, contest_id, symbol, side, qty) VALUES ($1, $2, $3, $4, $5)",
        userID, contestID, symbol, side, qty)

    // Update user profile
    _, err = pool.Primary().ExecContext(ctx,
        "UPDATE users SET email = $1 WHERE id = $2",
        email, userID)

    // === READ OPERATIONS (use Replica) ===

    // Get leaderboard (can tolerate slight delay)
    rows, err := pool.Replica().QueryContext(ctx,
        "SELECT user_id, total_pnl FROM contest_participants WHERE contest_id = $1 ORDER BY total_pnl DESC",
        contestID)

    // Get audit logs (read-heavy, historical data)
    rows, err = pool.Replica().QueryContext(ctx,
        "SELECT * FROM audit_logs WHERE created_at > $1",
        since)

    // Get contest history
    rows, err = pool.Replica().QueryContext(ctx,
        "SELECT * FROM contests WHERE status = 'completed'")

    // === READ-AFTER-WRITE (use Primary) ===

    // After inserting, read back immediately
    _, err = pool.Primary().ExecContext(ctx, "INSERT INTO orders ...")
    row := pool.Primary().QueryRowContext(ctx,
        "SELECT * FROM orders WHERE id = $1", orderID)

    // === TRANSACTIONS (always use Primary) ===

    tx, err := pool.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    // Multiple operations in transaction
    tx.ExecContext(ctx, "UPDATE contests SET status = 'completed' WHERE id = $1", contestID)
    tx.ExecContext(ctx, "UPDATE contest_participants SET final_rank = ... WHERE contest_id = $1", contestID)

    return tx.Commit()
}
```

---

## Metrics Reference

### Key Metrics to Monitor

| Metric | Warning | Critical | Description |
|--------|---------|----------|-------------|
| `pg_replication_lag_seconds` | > 5s | > 10s | Time-based replication lag |
| `pg_replication_lag_bytes` | > 50MB | > 100MB | WAL bytes behind |
| `pgbouncer_pools_client_waiting_connections` | > 5 | > 10 | Clients waiting for connection |
| `pg_stat_activity_count{state="idle in transaction"}` | > 5 | > 10 | Idle transactions |
| `patroni_cluster_running_members` | < 3 | < 2 | Running cluster members |

### Grafana Dashboard

Access the PostgreSQL HA dashboard at:
`http://grafana:3000/d/postgres-ha`

---

*Last updated: January 2026*
