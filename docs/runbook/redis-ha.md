# Redis High Availability Guide

This document covers the setup, operation, and maintenance of Redis high availability configurations for the tragge trading platform.

## Table of Contents

1. [Overview](#overview)
2. [Option A: Redis Sentinel](#option-a-redis-sentinel)
3. [Option B: Redis Cluster](#option-b-redis-cluster)
4. [Go Client Configuration](#go-client-configuration)
5. [Migration from Standalone](#migration-from-standalone)
6. [Failover Testing](#failover-testing)
7. [Backup and Recovery](#backup-and-recovery)
8. [Monitoring and Alerts](#monitoring-and-alerts)
9. [Troubleshooting](#troubleshooting)

---

## Overview

The tragge platform uses Redis for:
- **Session storage**: User authentication sessions with TTL
- **Leaderboard**: Real-time ranking using sorted sets (ZSET)
- **Price caching**: Latest market tick data per symbol
- **Pub/Sub**: Real-time event distribution (future use)

### Choosing the Right Option

| Factor | Sentinel | Cluster |
|--------|----------|---------|
| Data size | < 10GB | > 10GB |
| Complexity | Lower | Higher |
| Write scaling | Single master | Multiple masters |
| Read scaling | Replicas | Replicas + sharding |
| Pub/Sub | Full support | Limited (single-node) |
| Multi-key operations | Full support | Limited to same slot |
| Recommended for | Most deployments | Large-scale platforms |

**Recommendation**: Start with **Redis Sentinel** for most deployments. It's simpler to operate and supports all tragge features. Move to Cluster only when data exceeds 10GB or you need horizontal write scaling.

---

## Option A: Redis Sentinel

### Architecture

```
                    ┌─────────────────────────────────────────┐
                    │           Application Layer             │
                    │  (user-bff, trade-bff, leaderboard-worker)
                    └─────────────────┬───────────────────────┘
                                      │
                    ┌─────────────────▼───────────────────────┐
                    │          Sentinel Discovery              │
                    │   sentinel-1, sentinel-2, sentinel-3     │
                    │         (Quorum: 2 of 3)                │
                    └─────────────────┬───────────────────────┘
                                      │
          ┌───────────────────────────┼───────────────────────────┐
          │                           │                           │
          ▼                           ▼                           ▼
    ┌──────────┐              ┌──────────┐              ┌──────────┐
    │  redis-0 │◀────────────▶│  redis-1 │◀────────────▶│  redis-2 │
    │ (master) │  replication │ (replica)│  replication │ (replica)│
    └──────────┘              └──────────┘              └──────────┘
```

### Kubernetes Deployment

```bash
# Deploy Redis Sentinel setup
kubectl apply -f infra/k8s/base/redis-sentinel.yaml

# Verify pods are running
kubectl get pods -n tragge -l app.kubernetes.io/name=redis
kubectl get pods -n tragge -l app.kubernetes.io/name=redis-sentinel

# Check Sentinel status
kubectl exec -it redis-sentinel-0 -n tragge -- redis-cli -p 26379 sentinel master mymaster
```

### Docker Compose (Local Development)

```bash
# Start with Sentinel HA
docker-compose -f docker-compose.yml -f docker-compose.redis-sentinel.yml up -d

# Verify setup
docker exec tragge_sentinel_1 redis-cli -p 26379 sentinel master mymaster

# Test failover
docker stop tragge_redis_master
# Watch sentinel elect new master
docker logs -f tragge_sentinel_1
```

### Configuration Files

| File | Purpose |
|------|---------|
| `infra/k8s/base/redis-sentinel.yaml` | Kubernetes StatefulSet + Deployment |
| `infra/docker/docker-compose.redis-sentinel.yml` | Local development |
| `packages/redis/client.go` | Go client with Sentinel support |

### Environment Variables

```bash
# For Sentinel mode
REDIS_MODE=sentinel
REDIS_SENTINEL_ADDRS=sentinel-1:26379,sentinel-2:26379,sentinel-3:26379
REDIS_SENTINEL_MASTER=mymaster
REDIS_PASSWORD=your-password  # Optional
REDIS_DB=0
```

---

## Option B: Redis Cluster

### Architecture

```
                    ┌─────────────────────────────────────────┐
                    │           Application Layer             │
                    │       (Cluster-aware client)            │
                    └─────────────────┬───────────────────────┘
                                      │
                                      │ MOVED/ASK redirects
                                      │
    ┌──────────────────┬──────────────┼──────────────┬──────────────────┐
    │                  │              │              │                  │
    ▼                  ▼              ▼              ▼                  ▼
┌────────┐        ┌────────┐    ┌────────┐    ┌────────┐        ┌────────┐
│node-0  │        │node-1  │    │node-2  │    │node-3  │        │node-4  │
│slots   │        │slots   │    │slots   │    │replica │        │replica │
│0-5460  │◀──────▶│5461-   │◀──▶│10923-  │    │of 0    │        │of 1    │
│(master)│ gossip │10922   │    │16383   │    │        │        │        │
└────────┘        │(master)│    │(master)│    └────────┘        └────────┘
                  └────────┘    └────────┘          │                 │
                                     │              │                 │
                              ┌──────┴──────────────┴─────────────────┘
                              │         Cluster Bus (port 16379)
                              │         Gossip protocol
```

### Hash Slot Distribution

Redis Cluster distributes data across 16384 hash slots:
- Keys are hashed using CRC16
- Each master owns a range of slots
- Use hash tags `{tag}key` to ensure related keys are on the same slot

**Important for tragge**:
```go
// Leaderboard keys should use hash tags to keep contest data together
leaderboardKey := fmt.Sprintf("lb:{%s}", contestID)

// Session keys can use user ID as hash tag
sessionKey := fmt.Sprintf("session:{%s}:%s", userID, sessionID)
```

### Kubernetes Deployment

```bash
# Deploy Redis Cluster setup
kubectl apply -f infra/k8s/base/redis-cluster.yaml

# Wait for all pods to be ready
kubectl rollout status statefulset/redis-cluster -n tragge

# The cluster-init Job runs automatically
# Check cluster status
kubectl exec -it redis-cluster-0 -n tragge -- redis-cli cluster info
kubectl exec -it redis-cluster-0 -n tragge -- redis-cli cluster nodes
```

### Docker Compose (Local Development)

```bash
# Start with Redis Cluster
docker-compose -f docker-compose.yml -f docker-compose.redis-cluster.yml up -d

# Wait for cluster initialization
docker logs -f tragge_redis_cluster_init

# Verify cluster
docker exec tragge_redis_node_1 redis-cli cluster info
docker exec tragge_redis_node_1 redis-cli cluster nodes
```

### Environment Variables

```bash
# For Cluster mode
REDIS_MODE=cluster
REDIS_CLUSTER_ADDRS=node-1:6379,node-2:6379,node-3:6379,node-4:6379,node-5:6379,node-6:6379
REDIS_PASSWORD=your-password  # Optional
```

---

## Go Client Configuration

### Using the packages/redis Package

```go
package main

import (
    "context"
    "log"
    "os"

    redisclient "github.com/Parsaeffatravesh/tragge/packages/redis"
)

func main() {
    // Option 1: Auto-detect from environment
    client, err := redisclient.NewClientFromEnv(os.Getenv)
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // Option 2: Explicit Sentinel configuration
    client, err = redisclient.NewClient(redisclient.Config{
        Mode:           redisclient.ModeSentinel,
        SentinelAddrs:  []string{"sentinel-1:26379", "sentinel-2:26379", "sentinel-3:26379"},
        SentinelMaster: "mymaster",
        Password:       os.Getenv("REDIS_PASSWORD"),
        DB:             0,
    })

    // Option 3: Explicit Cluster configuration
    client, err = redisclient.NewClient(redisclient.Config{
        Mode:         redisclient.ModeCluster,
        ClusterAddrs: []string{"node-1:6379", "node-2:6379", "node-3:6379"},
        Password:     os.Getenv("REDIS_PASSWORD"),
    })

    // Health check
    status, err := client.HealthCheck(context.Background())
    if err != nil {
        log.Printf("Health check failed: %v", err)
    }
    log.Printf("Redis health: %+v", status)

    // Use as normal redis.UniversalClient
    ctx := context.Background()
    client.Set(ctx, "key", "value", 0)
    val, _ := client.Get(ctx, "key").Result()
    log.Println("Value:", val)
}
```

### Direct go-redis Usage

```go
import "github.com/redis/go-redis/v9"

// Sentinel client
sentinelClient := redis.NewFailoverClient(&redis.FailoverOptions{
    MasterName:    "mymaster",
    SentinelAddrs: []string{"sentinel-1:26379", "sentinel-2:26379", "sentinel-3:26379"},
    Password:      "",
    DB:            0,
})

// Cluster client
clusterClient := redis.NewClusterClient(&redis.ClusterOptions{
    Addrs:    []string{"node-1:6379", "node-2:6379", "node-3:6379"},
    Password: "",
})
```

---

## Migration from Standalone

### Pre-Migration Checklist

- [ ] Backup existing Redis data (RDB dump)
- [ ] Test HA setup in staging environment
- [ ] Verify all services use compatible Redis client
- [ ] Update environment variables
- [ ] Plan maintenance window

### Migration Steps

#### 1. Backup Current Data

```bash
# Kubernetes
kubectl exec redis-0 -n tragge -- redis-cli BGSAVE
kubectl cp tragge/redis-0:/data/dump.rdb ./backup/dump.rdb

# Docker
docker exec tragge_redis redis-cli BGSAVE
docker cp tragge_redis:/data/dump.rdb ./backup/dump.rdb
```

#### 2. Deploy HA Infrastructure

```bash
# For Sentinel
kubectl apply -f infra/k8s/base/redis-sentinel.yaml

# For Cluster
kubectl apply -f infra/k8s/base/redis-cluster.yaml
```

#### 3. Restore Data (Sentinel mode)

```bash
# Copy dump to master pod
kubectl cp ./backup/dump.rdb tragge/redis-0:/data/dump.rdb

# Restart Redis to load data
kubectl delete pod redis-0 -n tragge
```

#### 4. Update Application Configuration

```bash
# Update ConfigMap or environment variables
kubectl set env deployment/user-bff -n tragge \
  REDIS_MODE=sentinel \
  REDIS_SENTINEL_ADDRS=redis-sentinel:26379 \
  REDIS_SENTINEL_MASTER=mymaster

# Restart applications
kubectl rollout restart deployment -n tragge -l tier=backend
```

#### 5. Verify Migration

```bash
# Check data integrity
kubectl exec redis-0 -n tragge -- redis-cli DBSIZE

# Verify application connectivity
kubectl logs -f deployment/user-bff -n tragge | grep -i redis
```

#### 6. Cleanup (after validation)

```bash
# Remove standalone Redis
kubectl delete -f infra/k8s/base/redis.yaml
```

---

## Failover Testing

### Sentinel Failover Test

```bash
# 1. Check current master
kubectl exec -it redis-sentinel-0 -n tragge -- \
  redis-cli -p 26379 sentinel get-master-addr-by-name mymaster

# 2. Force failover
kubectl exec -it redis-sentinel-0 -n tragge -- \
  redis-cli -p 26379 sentinel failover mymaster

# 3. Verify new master elected
watch -n 1 'kubectl exec -it redis-sentinel-0 -n tragge -- \
  redis-cli -p 26379 sentinel get-master-addr-by-name mymaster'

# 4. Test application connectivity during failover
kubectl exec -it deployment/user-bff -n tragge -- \
  wget --spider --timeout=2 http://localhost:8081/readyz
```

### Cluster Failover Test

```bash
# 1. Check cluster state
kubectl exec -it redis-cluster-0 -n tragge -- redis-cli cluster nodes

# 2. Identify a master node and its replica
# Example: redis-cluster-0 is master, redis-cluster-3 is its replica

# 3. Trigger manual failover on replica
kubectl exec -it redis-cluster-3 -n tragge -- redis-cli cluster failover

# 4. Verify failover completed
kubectl exec -it redis-cluster-0 -n tragge -- redis-cli cluster nodes

# 5. Test application connectivity
kubectl exec -it deployment/leaderboard-worker -n tragge -- \
  wget --spider --timeout=2 http://localhost:8086/readyz
```

### Chaos Testing with Docker Compose

```bash
# Sentinel mode
docker stop tragge_redis_master
# Watch failover
docker logs -f tragge_sentinel_1 2>&1 | grep -E "switch-master|sdown|odown"
# Verify new master
docker exec tragge_sentinel_1 redis-cli -p 26379 sentinel get-master-addr-by-name mymaster

# Cluster mode
docker stop tragge_redis_node_1
# Check cluster state
docker exec tragge_redis_node_2 redis-cli cluster nodes
```

---

## Backup and Recovery

### Automated Backups (Kubernetes CronJob)

Add to your backup CronJob:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: redis-backup
  namespace: tragge
spec:
  schedule: "0 */6 * * *"  # Every 6 hours
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: backup
              image: redis:7-alpine
              command:
                - /bin/sh
                - -c
                - |
                  # For Sentinel mode - backup from master
                  MASTER=$(redis-cli -h redis-sentinel -p 26379 \
                    sentinel get-master-addr-by-name mymaster | head -1)

                  redis-cli -h $MASTER BGSAVE
                  sleep 5

                  # Copy to backup location
                  redis-cli -h $MASTER --rdb /backup/redis-$(date +%Y%m%d-%H%M%S).rdb

                  # Upload to S3 (example)
                  # aws s3 cp /backup/*.rdb s3://tragge-backups/redis/
              volumeMounts:
                - name: backup
                  mountPath: /backup
          volumes:
            - name: backup
              persistentVolumeClaim:
                claimName: redis-backup-pvc
          restartPolicy: OnFailure
```

### Manual Backup

```bash
# Sentinel mode
MASTER=$(kubectl exec -it redis-sentinel-0 -n tragge -- \
  redis-cli -p 26379 sentinel get-master-addr-by-name mymaster | head -1)

kubectl exec -it $MASTER -n tragge -- redis-cli BGSAVE
kubectl cp tragge/$MASTER:/data/dump.rdb ./redis-backup-$(date +%Y%m%d).rdb

# Cluster mode (backup each master)
for i in 0 1 2; do
  kubectl exec redis-cluster-$i -n tragge -- redis-cli BGSAVE
  kubectl cp tragge/redis-cluster-$i:/data/dump.rdb ./redis-cluster-$i-$(date +%Y%m%d).rdb
done
```

### Recovery Procedure

#### Sentinel Mode Recovery

```bash
# 1. Scale down applications
kubectl scale deployment -n tragge --replicas=0 -l tier=backend

# 2. Stop Redis pods
kubectl delete pod -n tragge -l app.kubernetes.io/name=redis

# 3. Copy backup to PVC
kubectl cp ./redis-backup.rdb tragge/redis-0:/data/dump.rdb

# 4. Start Redis pods
kubectl rollout restart statefulset/redis -n tragge

# 5. Wait for replication
kubectl exec redis-0 -n tragge -- redis-cli info replication

# 6. Scale up applications
kubectl scale deployment -n tragge --replicas=2 -l tier=backend
```

#### Cluster Mode Recovery

```bash
# Full cluster recovery requires restoring all master nodes
# Each master has different data (different slots)

# 1. Scale down applications
kubectl scale deployment -n tragge --replicas=0 -l tier=backend

# 2. Restore each master's data
for i in 0 1 2; do
  kubectl delete pod redis-cluster-$i -n tragge
  kubectl cp ./redis-cluster-$i-backup.rdb tragge/redis-cluster-$i:/data/dump.rdb
done

# 3. Restart cluster
kubectl rollout restart statefulset/redis-cluster -n tragge

# 4. Verify cluster health
kubectl exec redis-cluster-0 -n tragge -- redis-cli cluster info

# 5. Scale up applications
kubectl scale deployment -n tragge --replicas=2 -l tier=backend
```

---

## Monitoring and Alerts

### Prometheus Metrics

Key metrics from redis_exporter:

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `redis_up` | Redis availability | == 0 |
| `redis_connected_clients` | Connected clients | > 1000 |
| `redis_memory_used_bytes` | Memory usage | > 80% maxmemory |
| `redis_connected_slaves` | Replica count | < expected |
| `redis_master_link_up` | Replication link | == 0 (replica) |
| `redis_cluster_state` | Cluster health | != 1 |
| `redis_cluster_slots_ok` | Healthy slots | < 16384 |

### Prometheus Alert Rules

Add to `infra/prometheus/rules/redis-alerts.yml`:

```yaml
groups:
  - name: redis-alerts
    rules:
      # Redis Down
      - alert: RedisDown
        expr: redis_up == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Redis instance down"
          description: "Redis instance {{ $labels.instance }} is down"

      # High Memory Usage
      - alert: RedisHighMemoryUsage
        expr: redis_memory_used_bytes / redis_memory_max_bytes > 0.8
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Redis high memory usage"
          description: "Redis {{ $labels.instance }} memory usage is above 80%"

      # Replication Broken (Sentinel)
      - alert: RedisReplicationBroken
        expr: redis_connected_slaves < 2
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Redis replication degraded"
          description: "Redis master has fewer than 2 replicas"

      # Master Link Down (Replica)
      - alert: RedisMasterLinkDown
        expr: redis_master_link_up == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Redis replica disconnected from master"
          description: "Redis replica {{ $labels.instance }} lost connection to master"

      # Cluster State Fail
      - alert: RedisClusterStateFail
        expr: redis_cluster_state != 1
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Redis cluster state is not OK"
          description: "Redis cluster is in failed state"

      # Cluster Slots Not OK
      - alert: RedisClusterSlotsNotOK
        expr: redis_cluster_slots_ok < 16384
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Redis cluster has unhealthy slots"
          description: "Redis cluster has {{ $value }} healthy slots (expected 16384)"

      # Too Many Connections
      - alert: RedisTooManyConnections
        expr: redis_connected_clients > 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Too many Redis connections"
          description: "Redis {{ $labels.instance }} has {{ $value }} connections"

      # Sentinel Master Down
      - alert: RedisSentinelMasterDown
        expr: redis_sentinel_master_status != 1
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Redis Sentinel reports master down"
          description: "Sentinel {{ $labels.instance }} reports master {{ $labels.master }} is down"
```

### Grafana Dashboard

Import the Redis dashboard: Grafana Dashboard ID `763` (Redis Dashboard for Prometheus Redis Exporter)

Or use our custom dashboard at `infra/grafana/provisioning/dashboards/json/redis-ha.json`.

---

## Troubleshooting

### Common Issues

#### Sentinel: Split-Brain Prevention

**Symptom**: Multiple masters elected

**Solution**:
```bash
# Check quorum
kubectl exec redis-sentinel-0 -n tragge -- \
  redis-cli -p 26379 sentinel ckquorum mymaster

# Force reset if needed
kubectl exec redis-sentinel-0 -n tragge -- \
  redis-cli -p 26379 sentinel reset mymaster
```

#### Cluster: MOVED/ASK Errors

**Symptom**: Application receives MOVED or ASK errors

**Solution**: Ensure you're using a cluster-aware client
```go
// Wrong - standalone client
client := redis.NewClient(&redis.Options{Addr: "node:6379"})

// Correct - cluster client
client := redis.NewClusterClient(&redis.ClusterOptions{
    Addrs: []string{"node-1:6379", "node-2:6379", "node-3:6379"},
})
```

#### Cluster: Cross-Slot Error

**Symptom**: `CROSSSLOT Keys in request don't hash to the same slot`

**Solution**: Use hash tags for related keys
```go
// Wrong
client.MGet(ctx, "user:1:sessions", "user:2:sessions")

// Correct - use hash tags
client.MGet(ctx, "{user:1}:sessions", "{user:1}:profile")
```

#### Memory Issues

**Symptom**: OOM kills or failed writes

**Solution**:
```bash
# Check memory usage
kubectl exec redis-0 -n tragge -- redis-cli info memory

# Clear expired keys
kubectl exec redis-0 -n tragge -- redis-cli --scan --pattern '*' | \
  xargs -L 1 redis-cli TTL

# Evict keys if needed (uses maxmemory-policy)
```

### Debug Commands

```bash
# Sentinel status
redis-cli -p 26379 sentinel masters
redis-cli -p 26379 sentinel slaves mymaster
redis-cli -p 26379 sentinel sentinels mymaster

# Cluster status
redis-cli cluster info
redis-cli cluster nodes
redis-cli cluster slots

# Performance debugging
redis-cli slowlog get 10
redis-cli client list
redis-cli info all
```

---

## Operational Procedures

### Daily Operations

#### Health Check
```bash
# Quick health check (Docker Compose)
docker exec tragge_redis_sentinel_1 redis-cli -p 26379 sentinel master mymaster | grep -E "^(name|ip|port|num-slaves|num-sentinels|flags)"

# Kubernetes
kubectl exec redis-sentinel-0 -n tragge -- redis-cli -p 26379 sentinel master mymaster | grep -E "^(name|ip|port|num-slaves|num-sentinels|flags)"
```

#### Verify Replication Lag
```bash
# Check replication info on master
kubectl exec redis-0 -n tragge -- redis-cli info replication

# Look for:
# - connected_slaves: should match expected replicas
# - slave0, slave1: lag should be 0 or minimal
```

### Manual Failover Procedure

#### Planned Failover (Maintenance)
```bash
# 1. Identify current master
kubectl exec redis-sentinel-0 -n tragge -- redis-cli -p 26379 sentinel get-master-addr-by-name mymaster

# 2. Initiate graceful failover
kubectl exec redis-sentinel-0 -n tragge -- redis-cli -p 26379 sentinel failover mymaster

# 3. Verify failover completed (repeat until master changes)
watch -n 2 'kubectl exec redis-sentinel-0 -n tragge -- redis-cli -p 26379 sentinel get-master-addr-by-name mymaster'

# 4. Verify applications reconnected (check logs)
kubectl logs -f deployment/trade-bff -n tragge | grep -i redis

# 5. Perform maintenance on old master
```

#### Emergency Failover (Master Failure)
```bash
# Sentinel will automatically detect and failover
# Monitor the process:

# 1. Check Sentinel logs for failover detection
kubectl logs -f redis-sentinel-0 -n tragge | grep -E "sdown|odown|switch-master|elected-leader"

# 2. If automatic failover doesn't occur within 30s:
kubectl exec redis-sentinel-0 -n tragge -- redis-cli -p 26379 sentinel failover mymaster

# 3. Check for split-brain
kubectl exec redis-sentinel-0 -n tragge -- redis-cli -p 26379 sentinel ckquorum mymaster

# 4. Verify application recovery
kubectl get pods -n tragge -l tier=backend -o wide
```

### Automated Failover Testing

Use the failover test script for regular testing:

```bash
# Local Docker Compose testing
./scripts/redis-failover-test.sh

# The script will:
# 1. Verify Sentinel health (2/3 quorum)
# 2. Identify current master
# 3. Kill master container
# 4. Wait for automatic failover
# 5. Verify new master elected
# 6. Test read/write operations
# 7. Restart old master as replica
```

For Kubernetes, use the chaos engineering suite:
```bash
# Run Redis failover scenario
cd tools/chaos-test
go run . -scenario redis-failover -namespace tragge
```

### Scaling Procedures

#### Add a New Replica
```bash
# 1. Update StatefulSet replicas
kubectl scale statefulset redis -n tragge --replicas=4

# 2. Wait for new pod to be ready
kubectl rollout status statefulset redis -n tragge

# 3. Verify replication
kubectl exec redis-0 -n tragge -- redis-cli info replication

# 4. Verify Sentinel detected new replica
kubectl exec redis-sentinel-0 -n tragge -- redis-cli -p 26379 sentinel slaves mymaster
```

#### Remove a Replica
```bash
# 1. Ensure not removing the master
kubectl exec redis-sentinel-0 -n tragge -- redis-cli -p 26379 sentinel get-master-addr-by-name mymaster

# 2. Scale down
kubectl scale statefulset redis -n tragge --replicas=2

# 3. Verify Sentinel updated
kubectl exec redis-sentinel-0 -n tragge -- redis-cli -p 26379 sentinel slaves mymaster
```

### Incident Response

#### Redis Completely Down

1. **Check pod status**
   ```bash
   kubectl get pods -n tragge -l app.kubernetes.io/name=redis
   kubectl describe pod redis-0 -n tragge
   ```

2. **Check PVC/storage**
   ```bash
   kubectl get pvc -n tragge -l app.kubernetes.io/name=redis
   kubectl describe pvc redis-data-redis-0 -n tragge
   ```

3. **Check resource limits**
   ```bash
   kubectl top pod redis-0 -n tragge
   ```

4. **Restore from backup if needed**
   ```bash
   # See Backup and Recovery section
   ```

#### Sentinel Quorum Lost

1. **Check Sentinel status**
   ```bash
   for i in 0 1 2; do
     echo "=== Sentinel $i ==="
     kubectl exec redis-sentinel-$i -n tragge -- redis-cli -p 26379 sentinel ckquorum mymaster
   done
   ```

2. **If only 1 Sentinel is up, don't perform failover**
   - Wait for other Sentinels to recover
   - Or manually restart Sentinel pods

3. **Reset Sentinel state if corrupted**
   ```bash
   kubectl exec redis-sentinel-0 -n tragge -- redis-cli -p 26379 sentinel reset mymaster
   ```

### Maintenance Windows

#### Planned Redis Upgrade

```bash
# 1. Notify stakeholders
# 2. Enable maintenance mode in application if available

# 3. Upgrade one replica at a time
kubectl set image statefulset/redis redis=redis:7.2-alpine -n tragge

# 4. Watch rollout (StatefulSet upgrades one at a time)
kubectl rollout status statefulset/redis -n tragge

# 5. Verify replication after each pod upgrade
kubectl exec redis-0 -n tragge -- redis-cli info replication

# 6. Upgrade Sentinels
kubectl set image deployment/redis-sentinel redis-sentinel=redis:7.2-alpine -n tragge
kubectl rollout status deployment/redis-sentinel -n tragge

# 7. Verify cluster health
kubectl exec redis-sentinel-0 -n tragge -- redis-cli -p 26379 sentinel master mymaster
```

---

## Additional Resources

- [Redis Sentinel Documentation](https://redis.io/docs/management/sentinel/)
- [Redis Cluster Documentation](https://redis.io/docs/management/scaling/)
- [go-redis Documentation](https://redis.uptrace.dev/)
- [Redis Exporter Metrics](https://github.com/oliver006/redis_exporter)
