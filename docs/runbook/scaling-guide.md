# Scaling Guide

This document outlines procedures for scaling the tragge Trading Tournament Platform to handle increased load.

## Table of Contents

1. [Scaling Overview](#scaling-overview)
2. [Horizontal Pod Autoscaling](#horizontal-pod-autoscaling)
3. [Manual Scaling Procedures](#manual-scaling-procedures)
4. [Database Scaling](#database-scaling)
5. [Kafka Scaling](#kafka-scaling)
6. [Capacity Planning](#capacity-planning)
7. [Performance Monitoring](#performance-monitoring)

---

## Scaling Overview

### Service Scaling Characteristics

| Service | Stateless | Scale Strategy | Max Replicas | Notes |
|---------|-----------|----------------|--------------|-------|
| user-bff | Yes | Horizontal | 10 | Scale on CPU/requests |
| trade-bff | Yes | Horizontal | 20 | Scale on connections |
| admin-bff | Yes | Horizontal | 5 | Low traffic |
| trading-engine | Yes* | Horizontal | 5 | Kafka partitions limit |
| market-ingestor | Yes | Single | 1 | Single leader pattern |
| leaderboard-worker | Yes | Single | 1 | Single consumer pattern |
| gateway | Yes | Horizontal | 10 | Scale on connections |

*trading-engine maintains in-memory price book, replicated across instances

### Current Resource Limits

```yaml
# Default resource configuration
resources:
  user-bff:
    requests: { cpu: 100m, memory: 128Mi }
    limits: { cpu: 500m, memory: 512Mi }
  trade-bff:
    requests: { cpu: 200m, memory: 256Mi }
    limits: { cpu: 1000m, memory: 1Gi }
  trading-engine:
    requests: { cpu: 500m, memory: 512Mi }
    limits: { cpu: 2000m, memory: 2Gi }
```

---

## Horizontal Pod Autoscaling

### Enable HPA for Services

```yaml
# hpa-user-bff.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: user-bff-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: user-bff
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
      - type: Pods
        value: 2
        periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
      - type: Pods
        value: 1
        periodSeconds: 120
```

```yaml
# hpa-trade-bff.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: trade-bff-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: trade-bff
  minReplicas: 2
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 60
  - type: Pods
    pods:
      metric:
        name: websocket_connections
      target:
        type: AverageValue
        averageValue: 1000
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 30
      policies:
      - type: Pods
        value: 4
        periodSeconds: 30
    scaleDown:
      stabilizationWindowSeconds: 600
      policies:
      - type: Pods
        value: 1
        periodSeconds: 300
```

### Apply HPA

```bash
# Apply HPA configurations
kubectl apply -f infra/k8s/hpa/

# Verify HPA status
kubectl get hpa

# Watch HPA scaling
kubectl get hpa -w
```

### Custom Metrics for HPA

For WebSocket-based scaling, configure custom metrics:

```yaml
# custom-metrics-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-adapter-config
data:
  config.yaml: |
    rules:
    - seriesQuery: 'websocket_connections_total{namespace!="",pod!=""}'
      resources:
        overrides:
          namespace: {resource: "namespace"}
          pod: {resource: "pod"}
      name:
        matches: "^(.*)_total"
        as: "${1}"
      metricsQuery: 'sum(<<.Series>>{<<.LabelMatchers>>}) by (<<.GroupBy>>)'
```

---

## Manual Scaling Procedures

### Scale Up for Expected Load

Before a major contest or expected traffic spike:

```bash
#!/bin/bash
# scale-up.sh - Prepare for high load

echo "Scaling up for high load..."

# Scale BFF services
kubectl scale deployment user-bff --replicas=5
kubectl scale deployment trade-bff --replicas=10
kubectl scale deployment admin-bff --replicas=3

# Scale trading infrastructure
kubectl scale deployment trading-engine --replicas=3
kubectl scale deployment gateway --replicas=5

# Verify scaling
kubectl get pods

# Check resource availability
kubectl top nodes
kubectl top pods

echo "Scale up complete. Current pod count:"
kubectl get pods --no-headers | wc -l
```

### Scale Down After Peak

After the peak traffic period:

```bash
#!/bin/bash
# scale-down.sh - Return to normal capacity

echo "Scaling down to normal capacity..."

# Return to normal replicas
kubectl scale deployment user-bff --replicas=2
kubectl scale deployment trade-bff --replicas=2
kubectl scale deployment admin-bff --replicas=1
kubectl scale deployment trading-engine --replicas=1
kubectl scale deployment gateway --replicas=2

# Verify
kubectl get pods

echo "Scale down complete."
```

### Emergency Scale Up

For unexpected traffic spikes:

```bash
#!/bin/bash
# emergency-scale.sh

echo "EMERGENCY SCALE UP"
echo "=================="

# Immediately scale critical services
kubectl scale deployment trade-bff --replicas=15
kubectl scale deployment user-bff --replicas=8
kubectl scale deployment trading-engine --replicas=4
kubectl scale deployment gateway --replicas=8

# Monitor scaling progress
kubectl rollout status deployment/trade-bff --timeout=120s
kubectl rollout status deployment/user-bff --timeout=120s

# Check node capacity
echo "Node capacity:"
kubectl top nodes

# If nodes are exhausted, consider cluster autoscaler
# or manually adding nodes
```

---

## Database Scaling

### PostgreSQL Read Replicas

For read-heavy workloads, add read replicas:

```yaml
# postgres-read-replica.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres-replica
spec:
  replicas: 2
  selector:
    matchLabels:
      app: postgres-replica
  template:
    spec:
      containers:
      - name: postgres
        image: postgres:16-alpine
        env:
        - name: POSTGRES_REPLICA
          value: "true"
        - name: POSTGRES_MASTER_HOST
          value: "postgres"
```

### Connection Pooling with PgBouncer

```yaml
# pgbouncer.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: pgbouncer
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: pgbouncer
        image: pgbouncer/pgbouncer:latest
        env:
        - name: DATABASES_HOST
          value: "postgres"
        - name: DATABASES_PORT
          value: "5432"
        - name: DATABASES_USER
          value: "app"
        - name: POOL_MODE
          value: "transaction"
        - name: MAX_CLIENT_CONN
          value: "1000"
        - name: DEFAULT_POOL_SIZE
          value: "50"
```

### PostgreSQL Performance Tuning

```sql
-- Increase connection limits
ALTER SYSTEM SET max_connections = 500;

-- Increase shared buffers (25% of RAM)
ALTER SYSTEM SET shared_buffers = '4GB';

-- Increase work memory for complex queries
ALTER SYSTEM SET work_mem = '256MB';

-- Increase maintenance work memory
ALTER SYSTEM SET maintenance_work_mem = '1GB';

-- Optimize for SSD storage
ALTER SYSTEM SET random_page_cost = 1.1;
ALTER SYSTEM SET effective_io_concurrency = 200;

-- Reload configuration
SELECT pg_reload_conf();
```

### Redis Scaling

For high-traffic caching scenarios:

```yaml
# redis-cluster.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redis-cluster
spec:
  replicas: 6  # 3 masters + 3 replicas
  template:
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        command:
        - redis-server
        - --cluster-enabled yes
        - --cluster-config-file nodes.conf
        - --maxmemory 2gb
        - --maxmemory-policy allkeys-lru
```

---

## Kafka Scaling

### Increase Partitions

For higher throughput:

```bash
# Increase partitions for high-volume topics
rpk topic alter-config ticks.v1 --set partitions=12
rpk topic alter-config orders.v1 --set partitions=6
rpk topic alter-config fills.v1 --set partitions=6
rpk topic alter-config pnl.v1 --set partitions=6

# Verify partition count
rpk topic describe ticks.v1
```

### Scale Redpanda Cluster

```bash
# Add brokers to the cluster
kubectl scale statefulset redpanda --replicas=5

# Rebalance partitions
rpk cluster partitions rebalance

# Monitor rebalance progress
rpk cluster partitions status
```

### Consumer Group Scaling

When scaling trading-engine, ensure Kafka partitions match:

```bash
# Check current consumer group
rpk group describe trading-engine-group

# If scaling to 4 replicas, ensure at least 4 partitions
rpk topic describe orders.v1

# trading-engine replicas should not exceed partition count
# Scale partitions first if needed
```

---

## Capacity Planning

### Load Testing

Before scaling for a major event:

```bash
# Run load test with expected traffic
cd tools/ws-load-test
go run main.go \
    --url ws://trade-bff:8082/ws/trade \
    --connections 5000 \
    --rate 100 \
    --duration 10m

# Monitor during test
kubectl top pods
watch kubectl get hpa
```

### Resource Estimation

| Concurrent Users | trade-bff Replicas | trading-engine Replicas | Gateway Replicas |
|------------------|-------------------|------------------------|------------------|
| 1,000 | 2 | 1 | 2 |
| 5,000 | 5 | 2 | 3 |
| 10,000 | 10 | 3 | 5 |
| 25,000 | 15 | 4 | 8 |
| 50,000 | 25 | 5 | 10 |

### Node Sizing Recommendations

```yaml
# Production node pool configuration
nodePool:
  name: app-pool
  machineType: n2-standard-8  # 8 vCPU, 32GB RAM
  minNodes: 3
  maxNodes: 20

  # For high-memory services (trading-engine)
  highMemoryPool:
    machineType: n2-highmem-8  # 8 vCPU, 64GB RAM
    minNodes: 1
    maxNodes: 5
```

---

## Performance Monitoring

### Key Metrics to Watch

```bash
# CPU and memory usage
kubectl top pods

# Request latency (from Prometheus)
curl -s 'http://prometheus:9090/api/v1/query?query=histogram_quantile(0.99,rate(http_request_duration_seconds_bucket[5m]))'

# WebSocket connection count
curl -s 'http://prometheus:9090/api/v1/query?query=websocket_connections_total'

# Kafka consumer lag
rpk group describe trading-engine-group

# Database connections
psql -h postgres -U app -c "SELECT count(*) FROM pg_stat_activity WHERE state = 'active';"
```

### Grafana Dashboard Queries

```promql
# Request rate by service
sum(rate(http_requests_total[5m])) by (service)

# P99 latency
histogram_quantile(0.99, sum(rate(http_request_duration_seconds_bucket[5m])) by (le, service))

# Error rate
sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m]))

# Pod CPU usage
sum(rate(container_cpu_usage_seconds_total[5m])) by (pod)

# Memory usage percentage
sum(container_memory_working_set_bytes) by (pod) / sum(container_spec_memory_limit_bytes) by (pod) * 100
```

### Alerting Thresholds

```yaml
# High CPU alert
- alert: HighCPUUsage
  expr: sum(rate(container_cpu_usage_seconds_total[5m])) by (pod) > 0.8
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "High CPU usage on {{ $labels.pod }}"

# High memory alert
- alert: HighMemoryUsage
  expr: container_memory_working_set_bytes / container_spec_memory_limit_bytes > 0.9
  for: 5m
  labels:
    severity: warning

# Scaling event alert
- alert: HPAMaxedOut
  expr: kube_hpa_status_current_replicas == kube_hpa_spec_max_replicas
  for: 15m
  labels:
    severity: warning
  annotations:
    summary: "HPA {{ $labels.hpa }} at max replicas"
```

---

## Scaling Cheat Sheet

### Quick Commands

```bash
# Current replica counts
kubectl get deployments -o custom-columns=NAME:.metadata.name,REPLICAS:.spec.replicas

# Scale specific service
kubectl scale deployment/<name> --replicas=N

# Check HPA status
kubectl get hpa

# Watch pod scaling
watch -n2 kubectl get pods

# Check node capacity
kubectl describe nodes | grep -A5 "Allocated resources"

# Pod resource usage
kubectl top pods --sort-by=cpu
kubectl top pods --sort-by=memory
```

### Pre-Event Checklist

- [ ] Review expected traffic and user count
- [ ] Scale services according to capacity table
- [ ] Verify HPA is configured correctly
- [ ] Check node auto-scaling is enabled
- [ ] Increase database connection limits
- [ ] Scale Kafka partitions if needed
- [ ] Run load test at expected scale
- [ ] Set up monitoring dashboards
- [ ] Prepare rollback plan
- [ ] Brief on-call team

---

*Last Updated: January 2026*
