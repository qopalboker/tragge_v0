# Database Recovery Runbook

This document outlines procedures for recovering PostgreSQL and Redis databases in the tragge Trading Tournament Platform.

## Table of Contents

1. [Overview](#overview)
2. [Backup Locations](#backup-locations)
3. [PostgreSQL Recovery](#postgresql-recovery)
4. [Redis Recovery](#redis-recovery)
5. [Point-in-Time Recovery](#point-in-time-recovery)
6. [Data Integrity Verification](#data-integrity-verification)
7. [Emergency Procedures](#emergency-procedures)

---

## Overview

### Recovery Time Objectives (RTO)

| Database | Target RTO | Maximum RTO |
|----------|------------|-------------|
| PostgreSQL | 30 minutes | 2 hours |
| Redis | 15 minutes | 1 hour |

### Recovery Point Objectives (RPO)

| Database | Backup Frequency | Maximum Data Loss |
|----------|------------------|-------------------|
| PostgreSQL | Daily + WAL | 5 minutes |
| Redis | Daily | 24 hours |

### Prerequisites

Before starting recovery:

1. Ensure you have AWS credentials with S3 access
2. Verify backup integrity before restoring
3. Notify stakeholders of potential downtime
4. Have a rollback plan ready

---

## Backup Locations

### S3 Bucket Structure

```
s3://tragge-backups/
├── postgres/
│   ├── daily/
│   │   ├── postgres_app_full_YYYYMMDD_HHMMSS.sql.gz
│   │   └── latest_full.json
│   ├── wal/
│   │   └── [WAL archive files]
│   └── pre-restore/
│       └── [pre-restore backups]
├── redis/
│   ├── daily/
│   │   ├── redis_YYYYMMDD_HHMMSS.rdb.gz
│   │   └── latest.json
│   └── pre-restore/
│       └── [pre-restore backups]
└── manifests/
    └── backup-status.json
```

### Listing Available Backups

```bash
# PostgreSQL backups
./scripts/backup/restore-postgres.sh --list

# Redis backups
./scripts/backup/restore-redis.sh --list

# Manual listing
aws s3 ls s3://tragge-backups/postgres/daily/ --recursive
aws s3 ls s3://tragge-backups/redis/daily/ --recursive
```

---

## PostgreSQL Recovery

### Scenario 1: Full Database Recovery

Use this when the database is corrupted or completely unavailable.

#### Step 1: Stop Dependent Services

```bash
# Scale down services that depend on PostgreSQL
kubectl scale deployment user-bff --replicas=0
kubectl scale deployment trade-bff --replicas=0
kubectl scale deployment admin-bff --replicas=0
kubectl scale deployment trading-engine --replicas=0
kubectl scale deployment leaderboard-worker --replicas=0

# Verify services are stopped
kubectl get pods
```

#### Step 2: Restore from Latest Backup

```bash
# Set environment variables
export POSTGRES_HOST=postgres
export POSTGRES_PORT=5432
export POSTGRES_DB=app
export POSTGRES_USER=app
export POSTGRES_PASSWORD=$(kubectl get secret postgres-secret -o jsonpath='{.data.password}' | base64 -d)
export S3_BUCKET=tragge-backups
export AWS_REGION=us-east-1

# Restore from latest backup
./scripts/backup/restore-postgres.sh --latest --force
```

#### Step 3: Verify Restoration

```bash
# Connect to database
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DB

# Run verification queries
\dt                                    -- List tables
SELECT COUNT(*) FROM users;            -- Check user count
SELECT COUNT(*) FROM contests;         -- Check contest count
SELECT COUNT(*) FROM orders;           -- Check order count
SELECT MAX(created_at) FROM orders;    -- Check latest order timestamp
```

#### Step 4: Restart Services

```bash
# Scale services back up
kubectl scale deployment user-bff --replicas=2
kubectl scale deployment trade-bff --replicas=2
kubectl scale deployment admin-bff --replicas=2
kubectl scale deployment trading-engine --replicas=2
kubectl scale deployment leaderboard-worker --replicas=1

# Verify services are healthy
kubectl get pods
kubectl logs -l app=user-bff --tail=20
```

---

### Scenario 2: Restore Specific Backup

Use this when you need to restore from a specific point in time.

```bash
# List available backups
./scripts/backup/restore-postgres.sh --list

# Restore specific backup
./scripts/backup/restore-postgres.sh \
    --backup-file "postgres/daily/postgres_app_full_20240115_030000.sql.gz"
```

---

### Scenario 3: Table-Level Recovery

Use this when only specific tables are affected.

#### Step 1: Restore to Temporary Database

```bash
# Create temporary database
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d postgres -c "CREATE DATABASE app_recovery;"

# Restore backup to temporary database
export POSTGRES_DB=app_recovery
./scripts/backup/restore-postgres.sh --latest --force --no-pre-backup
```

#### Step 2: Copy Specific Tables

```bash
# Copy data from recovered table to production
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d app <<EOF
-- Disable triggers temporarily
ALTER TABLE target_table DISABLE TRIGGER ALL;

-- Clear existing data (if appropriate)
TRUNCATE target_table;

-- Copy from recovery database
INSERT INTO target_table
SELECT * FROM app_recovery.public.target_table;

-- Re-enable triggers
ALTER TABLE target_table ENABLE TRIGGER ALL;
EOF
```

#### Step 3: Cleanup

```bash
# Drop temporary database
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d postgres -c "DROP DATABASE app_recovery;"
```

---

### Scenario 4: Single Row Recovery

Use this to recover specific deleted or modified rows.

```bash
# Create recovery database
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d postgres -c "CREATE DATABASE app_recovery;"

# Restore backup
POSTGRES_DB=app_recovery ./scripts/backup/restore-postgres.sh --latest --force --no-pre-backup

# Find the row in recovery database
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d app_recovery -c \
    "SELECT * FROM users WHERE id = 'user-uuid-here';"

# Copy specific row back (adjust for your table)
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d app <<EOF
INSERT INTO users
SELECT * FROM app_recovery.public.users
WHERE id = 'user-uuid-here'
ON CONFLICT (id) DO UPDATE SET
    email = EXCLUDED.email,
    updated_at = NOW();
EOF

# Cleanup
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d postgres -c "DROP DATABASE app_recovery;"
```

---

## Redis Recovery

### Scenario 1: Full Redis Recovery

Use this when Redis is corrupted or has lost all data.

#### Step 1: Stop Services Using Redis

```bash
# Scale down services
kubectl scale deployment user-bff --replicas=0
kubectl scale deployment trade-bff --replicas=0
kubectl scale deployment market-ingestor --replicas=0
kubectl scale deployment leaderboard-worker --replicas=0
```

#### Step 2: Restore Redis

```bash
# Set environment variables
export REDIS_HOST=redis
export REDIS_PORT=6379
export REDIS_PASSWORD=$(kubectl get secret redis-secret -o jsonpath='{.data.password}' | base64 -d)
export REDIS_CONTAINER=redis-0
export S3_BUCKET=tragge-backups
export AWS_REGION=us-east-1

# Restore from latest backup
./scripts/backup/restore-redis.sh --latest --force
```

#### Step 3: Verify and Restart

```bash
# Test Redis connection
redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD PING

# Check key count
redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD DBSIZE

# Check leaderboard data
redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD KEYS "lb:*"

# Restart services
kubectl scale deployment user-bff --replicas=2
kubectl scale deployment trade-bff --replicas=2
kubectl scale deployment market-ingestor --replicas=1
kubectl scale deployment leaderboard-worker --replicas=1
```

---

### Scenario 2: Leaderboard Rebuild

If leaderboard data is lost but PostgreSQL is intact:

```bash
# Connect to PostgreSQL and rebuild leaderboard
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DB <<EOF
-- Get leaderboard data for each active contest
SELECT
    contest_id,
    user_id,
    total_pnl
FROM contest_participants
WHERE contest_id IN (
    SELECT id FROM contests WHERE status = 'running'
)
ORDER BY contest_id, total_pnl DESC;
EOF

# Rebuild Redis leaderboard
for contest_id in $(psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DB -t -c \
    "SELECT id FROM contests WHERE status = 'running';"); do

    psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DB -t -c \
        "SELECT user_id, total_pnl FROM contest_participants WHERE contest_id = '$contest_id';" | \
    while read user_id pnl; do
        redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD ZADD "lb:$contest_id" $pnl "$user_id"
    done
done
```

---

### Scenario 3: Session Invalidation

If sessions are corrupted, invalidate all sessions:

```bash
# Clear all session keys
redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD KEYS "session:*" | \
    xargs -r redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD DEL

# Users will need to log in again
```

---

## Point-in-Time Recovery

### PostgreSQL WAL Recovery

For recovery to a specific point in time (requires WAL archiving):

```bash
# Download WAL files
aws s3 sync s3://tragge-backups/postgres/wal/ /tmp/wal-recovery/

# Create recovery.conf
cat > /tmp/recovery.conf <<EOF
restore_command = 'cp /tmp/wal-recovery/%f %p'
recovery_target_time = '2024-01-15 14:30:00 UTC'
recovery_target_action = 'promote'
EOF

# Stop PostgreSQL
kubectl scale statefulset postgres --replicas=0

# Copy recovery configuration
kubectl cp /tmp/recovery.conf postgres-0:/var/lib/postgresql/data/

# Start PostgreSQL
kubectl scale statefulset postgres --replicas=1

# Monitor recovery
kubectl logs -f postgres-0
```

---

## Data Integrity Verification

### PostgreSQL Verification

```bash
# Run verification queries
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DB <<EOF
-- Check for orphaned records
SELECT 'Orphaned orders' as check, COUNT(*)
FROM orders o
WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = o.user_id);

SELECT 'Orphaned positions' as check, COUNT(*)
FROM positions p
WHERE NOT EXISTS (SELECT 1 FROM users u WHERE u.id = p.user_id);

-- Check referential integrity
SELECT 'Contest participants without contests' as check, COUNT(*)
FROM contest_participants cp
WHERE NOT EXISTS (SELECT 1 FROM contests c WHERE c.id = cp.contest_id);

-- Check for data anomalies
SELECT 'Orders with negative quantities' as check, COUNT(*)
FROM orders WHERE qty < 0;

SELECT 'Positions with invalid sides' as check, COUNT(*)
FROM positions WHERE side NOT IN ('long', 'short');

-- Validate indexes
SELECT schemaname, tablename, indexname
FROM pg_indexes
WHERE schemaname = 'public'
ORDER BY tablename;
EOF
```

### Redis Verification

```bash
# Check leaderboard consistency
for contest_id in $(redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD KEYS "lb:*" | sed 's/lb://'); do
    echo "Contest: $contest_id"
    redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD ZCARD "lb:$contest_id"
done

# Verify session format
redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD KEYS "session:*" | head -5 | while read key; do
    echo "Key: $key"
    redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD TYPE "$key"
done
```

---

## Emergency Procedures

### Database Connection Exhaustion

```bash
# Check active connections
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d postgres -c \
    "SELECT count(*) FROM pg_stat_activity;"

# Kill idle connections
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d postgres -c \
    "SELECT pg_terminate_backend(pid)
     FROM pg_stat_activity
     WHERE state = 'idle'
     AND query_start < now() - interval '10 minutes';"
```

### Database Disk Full

```bash
# Check disk usage
kubectl exec -it postgres-0 -- df -h

# Find large tables
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DB -c \
    "SELECT relname, pg_size_pretty(pg_total_relation_size(relid))
     FROM pg_catalog.pg_statio_user_tables
     ORDER BY pg_total_relation_size(relid) DESC
     LIMIT 10;"

# Vacuum to reclaim space
psql -h $POSTGRES_HOST -U $POSTGRES_USER -d $POSTGRES_DB -c "VACUUM FULL;"

# If needed, expand PVC
kubectl patch pvc postgres-data -p '{"spec":{"resources":{"requests":{"storage":"100Gi"}}}}'
```

### Redis Memory Full

```bash
# Check memory usage
redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD INFO memory

# Find large keys
redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD --bigkeys

# Clear expired sessions
redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD KEYS "session:*" | while read key; do
    ttl=$(redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD TTL "$key")
    if [ "$ttl" -eq -1 ]; then
        redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD EXPIRE "$key" 86400
    fi
done

# Set memory limit and eviction policy
redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD CONFIG SET maxmemory 1gb
redis-cli -h $REDIS_HOST -a $REDIS_PASSWORD CONFIG SET maxmemory-policy volatile-lru
```

---

## Quick Reference

### Recovery Commands Cheat Sheet

```bash
# PostgreSQL - Latest backup restore
./scripts/backup/restore-postgres.sh --latest

# PostgreSQL - Specific backup restore
./scripts/backup/restore-postgres.sh --backup-file "path/to/backup.sql.gz"

# PostgreSQL - List backups
./scripts/backup/restore-postgres.sh --list

# Redis - Latest backup restore
./scripts/backup/restore-redis.sh --latest

# Redis - List backups
./scripts/backup/restore-redis.sh --list

# Create manual backup before changes
./scripts/backup/backup-postgres.sh
./scripts/backup/backup-redis.sh
```

### Environment Variables

```bash
export POSTGRES_HOST=postgres
export POSTGRES_PORT=5432
export POSTGRES_DB=app
export POSTGRES_USER=app
export POSTGRES_PASSWORD=<from-secret>

export REDIS_HOST=redis
export REDIS_PORT=6379
export REDIS_PASSWORD=<from-secret>

export S3_BUCKET=tragge-backups
export AWS_REGION=us-east-1
```

---

*Last Updated: January 2026*
