# Tragge Trading Platform - Rollback Procedures

This document outlines the rollback procedures for the Tragge Trading Platform. Use these procedures when a deployment causes issues that require reverting to a previous state.

## Table of Contents

1. [Rollback Decision Matrix](#rollback-decision-matrix)
2. [Quick Rollback Commands](#quick-rollback-commands)
3. [Deployment Rollback](#deployment-rollback)
4. [StatefulSet Rollback](#statefulset-rollback)
5. [Database Rollback](#database-rollback)
6. [Configuration Rollback](#configuration-rollback)
7. [Full Environment Rollback](#full-environment-rollback)
8. [Post-Rollback Verification](#post-rollback-verification)
9. [Root Cause Analysis](#root-cause-analysis)

---

## Rollback Decision Matrix

| Severity | Symptoms | Action | Timeline |
|----------|----------|--------|----------|
| **Critical** | Platform down, no trading | Immediate rollback | < 5 min |
| **High** | Degraded trading, data loss risk | Rollback within | < 15 min |
| **Medium** | Feature broken, workaround exists | Assess and decide | < 1 hour |
| **Low** | Minor issues, non-blocking | Fix forward preferred | Next deploy |

### When to Rollback

**DO Rollback:**
- Error rate > 5% post-deployment
- Latency increase > 50%
- WebSocket connection failures
- Data corruption detected
- Security vulnerability discovered

**DON'T Rollback:**
- Minor UI bugs
- Log verbosity issues
- Non-critical feature regression
- Performance within acceptable range

---

## Quick Rollback Commands

### Single Service Rollback

```bash
# Rollback to previous revision
kubectl rollout undo deployment/USER-BFF -n tragge

# Rollback to specific revision
kubectl rollout undo deployment/trade-bff --to-revision=3 -n tragge

# Check rollback status
kubectl rollout status deployment/trade-bff -n tragge
```

### All Services Rollback

```bash
# Rollback all deployments to previous revision
for deploy in user-bff trade-bff admin-bff gateway market-ingestor leaderboard-worker; do
  kubectl rollout undo deployment/$deploy -n tragge
done

# Rollback frontends
for deploy in frontend frontend frontend; do
  kubectl rollout undo deployment/$deploy -n tragge
done
```

---

## Deployment Rollback

### Step 1: Identify the Problem

```bash
# Check deployment status
kubectl get deployments -n tragge

# View recent events
kubectl get events -n tragge --sort-by='.lastTimestamp' | tail -20

# Check pod logs
kubectl logs -l app.kubernetes.io/name=trade-bff -n tragge --tail=100

# Check metrics for anomalies
kubectl top pods -n tragge
```

### Step 2: View Rollout History

```bash
# List all revisions
kubectl rollout history deployment/trade-bff -n tragge

# View specific revision details
kubectl rollout history deployment/trade-bff -n tragge --revision=3

# Compare current vs previous
kubectl get deployment trade-bff -n tragge -o yaml > current.yaml
kubectl rollout history deployment/trade-bff -n tragge --revision=2 -o yaml > previous.yaml
diff current.yaml previous.yaml
```

### Step 3: Perform Rollback

```bash
# Rollback to previous revision
kubectl rollout undo deployment/trade-bff -n tragge

# Or rollback to specific revision
kubectl rollout undo deployment/trade-bff --to-revision=2 -n tragge

# Watch rollback progress
kubectl rollout status deployment/trade-bff -n tragge
```

### Step 4: Verify Rollback

```bash
# Check pods are running
kubectl get pods -l app.kubernetes.io/name=trade-bff -n tragge

# Verify image version
kubectl get deployment trade-bff -n tragge -o jsonpath='{.spec.template.spec.containers[0].image}'

# Test health endpoint
kubectl exec -it deploy/trade-bff -n tragge -- wget -qO- http://localhost:8082/healthz
```

---

## StatefulSet Rollback

### Trading Engine Rollback

The trading engine uses StatefulSet with sharded partition processing.

```bash
# View rollout history
kubectl rollout history statefulset/trading-engine -n tragge

# Rollback (pods restart one at a time)
kubectl rollout undo statefulset/trading-engine -n tragge

# Watch rollback progress
kubectl rollout status statefulset/trading-engine -n tragge

# Verify all shards are running
kubectl get pods -l app.kubernetes.io/name=trading-engine -n tragge
```

### PostgreSQL Rollback

**WARNING:** StatefulSet rollback does NOT restore data. See [Database Rollback](#database-rollback).

```bash
# Rollback StatefulSet configuration only
kubectl rollout undo statefulset/postgres -n tragge

# For data rollback, see Database Rollback section
```

### Redis Rollback

```bash
# Rollback StatefulSet configuration
kubectl rollout undo statefulset/redis -n tragge

# Verify Redis is responding
kubectl exec -it redis-0 -n tragge -- redis-cli ping
```

---

## Database Rollback

### Point-in-Time Recovery (PostgreSQL)

```bash
# Step 1: Stop application traffic
kubectl scale deployment user-bff trade-bff admin-bff --replicas=0 -n tragge

# Step 2: Find available backups
aws s3 ls s3://tragge-backups/backups/postgres/ --recursive | tail -10

# Step 3: Download desired backup
aws s3 cp s3://tragge-backups/backups/postgres/postgres_app_full_20260108_030000.sql.gz ./

# Step 4: Create restore job
kubectl apply -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: postgres-restore
  namespace: tragge
spec:
  template:
    spec:
      containers:
      - name: restore
        image: postgres:16-alpine
        command:
        - /bin/bash
        - -c
        - |
          # Download backup from S3
          aws s3 cp s3://tragge-backups/backups/postgres/postgres_app_full_20260108_030000.sql.gz /tmp/

          # Restore database
          export PGPASSWORD=\$POSTGRES_PASSWORD
          psql -h postgres -U tragge_admin -d postgres -c "DROP DATABASE IF EXISTS app_new;"
          psql -h postgres -U tragge_admin -d postgres -c "CREATE DATABASE app_new;"
          gunzip -c /tmp/postgres_app_full_*.sql.gz | psql -h postgres -U tragge_admin -d app_new

          # Swap databases
          psql -h postgres -U tragge_admin -d postgres -c "ALTER DATABASE app RENAME TO app_old;"
          psql -h postgres -U tragge_admin -d postgres -c "ALTER DATABASE app_new RENAME TO app;"
        env:
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secrets
              key: POSTGRES_ADMIN_PASSWORD
        - name: AWS_ACCESS_KEY_ID
          valueFrom:
            secretKeyRef:
              name: backup-secrets
              key: AWS_ACCESS_KEY_ID
        - name: AWS_SECRET_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: backup-secrets
              key: AWS_SECRET_ACCESS_KEY
      restartPolicy: Never
  backoffLimit: 1
EOF

# Step 5: Wait for restore to complete
kubectl wait --for=condition=complete job/postgres-restore -n tragge --timeout=1800s

# Step 6: Restart application pods
kubectl scale deployment user-bff trade-bff admin-bff --replicas=3 -n tragge

# Step 7: Verify application functionality
kubectl logs -l app.kubernetes.io/name=user-bff -n tragge --tail=20
```

### Migration Rollback

```bash
# View current migration version
kubectl run migrate-status --rm -it --restart=Never \
  --image=migrate/migrate:v4.17.0 \
  -n tragge \
  --env="POSTGRES_DSN=$(kubectl get secret tragge-secrets -n tragge -o jsonpath='{.data.POSTGRES_DSN}' | base64 -d)" \
  -- \
  -path=/migrations -database="${POSTGRES_DSN}" version

# Rollback one migration
kubectl run migrate-down --rm -it --restart=Never \
  --image=migrate/migrate:v4.17.0 \
  -n tragge \
  --env="POSTGRES_DSN=$(kubectl get secret tragge-secrets -n tragge -o jsonpath='{.data.POSTGRES_DSN}' | base64 -d)" \
  -- \
  -path=/migrations -database="${POSTGRES_DSN}" down 1

# Rollback to specific version
kubectl run migrate-goto --rm -it --restart=Never \
  --image=migrate/migrate:v4.17.0 \
  -n tragge \
  --env="POSTGRES_DSN=$(kubectl get secret tragge-secrets -n tragge -o jsonpath='{.data.POSTGRES_DSN}' | base64 -d)" \
  -- \
  -path=/migrations -database="${POSTGRES_DSN}" goto 5
```

---

## Configuration Rollback

### ConfigMap Rollback

```bash
# ConfigMaps don't have revision history, so use Git
git log --oneline infra/k8s/base/configmap.yaml

# Checkout previous version
git checkout HEAD~1 -- infra/k8s/base/configmap.yaml

# Apply previous ConfigMap
kubectl apply -f infra/k8s/base/configmap.yaml

# Restart pods to pick up new config
kubectl rollout restart deployment/user-bff -n tragge
```

### Secret Rollback

```bash
# For External Secrets, update the source (Vault/AWS SM)
# The ExternalSecret will sync automatically

# To force resync:
kubectl annotate externalsecret tragge-database-secrets force-sync="$(date)" -n tragge

# Manual secret rollback (emergency only)
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: tragge-secrets
  namespace: tragge
type: Opaque
data:
  # Base64 encoded values from backup
  POSTGRES_DSN: <previous_value>
  JWT_SECRET: <previous_value>
EOF
```

### Ingress/TLS Rollback

```bash
# Rollback Ingress configuration
git checkout HEAD~1 -- infra/k8s/base/ingress.yaml
kubectl apply -f infra/k8s/base/ingress.yaml

# Force certificate renewal if TLS issues
kubectl delete certificate tragge-tls-secret -n tragge
kubectl apply -f infra/k8s/base/certificate.yaml
```

---

## Full Environment Rollback

In extreme cases, rollback the entire environment:

### Step 1: Document Current State

```bash
# Save current state for analysis
kubectl get all -n tragge -o yaml > pre-rollback-state.yaml
kubectl get configmaps -n tragge -o yaml >> pre-rollback-state.yaml
kubectl get secrets -n tragge -o yaml >> pre-rollback-state.yaml
```

### Step 2: Checkout Previous Known-Good State

```bash
# Find last known-good commit
git log --oneline --grep="deploy"

# Checkout that state
git checkout <commit-hash>
```

### Step 3: Apply Previous State

```bash
# For base manifests
kubectl apply -k infra/k8s/base

# For production overlay
kubectl apply -k infra/k8s/overlays/production

# Watch rollout
watch kubectl get pods -n tragge
```

### Step 4: Restore Database (if needed)

Follow [Database Rollback](#database-rollback) procedures.

---

## Post-Rollback Verification

### Health Checks

```bash
# Verify all pods are running
kubectl get pods -n tragge | grep -v Running | grep -v Completed

# Check endpoints
kubectl get endpoints -n tragge

# Test health endpoints
for svc in user-bff trade-bff admin-bff; do
  echo "=== $svc ==="
  kubectl exec -it deploy/$svc -n tragge -- wget -qO- http://localhost:808X/healthz
done
```

### Functional Tests

```bash
# Test user authentication
TOKEN=$(curl -s -X POST https://api.tragge.io/user/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"testpass"}' | jq -r '.access_token')

# Test WebSocket connection
echo '{"type":"ping"}' | websocat wss://ws.tragge.io/ws/trade -H "Authorization: Bearer $TOKEN"

# Verify trading functionality
curl -s https://api.tragge.io/trade/contests -H "Authorization: Bearer $TOKEN"
```

### Metrics Check

```bash
# Check error rates returned to normal
kubectl port-forward svc/prometheus 9090:9090 -n monitoring &
curl -s "http://localhost:9090/api/v1/query?query=rate(http_requests_total{status=~\"5..\"}[5m])"

# Verify latency is normal
curl -s "http://localhost:9090/api/v1/query?query=histogram_quantile(0.99,rate(http_request_duration_seconds_bucket[5m]))"
```

---

## Root Cause Analysis

After stabilization, conduct RCA:

### Collect Evidence

```bash
# Export events from incident window
kubectl get events -n tragge --sort-by='.lastTimestamp' -o json > incident-events.json

# Export pod logs
for pod in $(kubectl get pods -n tragge -o name); do
  kubectl logs $pod -n tragge --since=2h > logs/$(basename $pod).log 2>&1
done

# Export metrics
curl -s "http://localhost:9090/api/v1/query_range?query=up&start=$(date -d '2 hours ago' +%s)&end=$(date +%s)&step=60" > metrics-up.json
```

### Document Findings

Create an incident report covering:

1. **Timeline** - When was the issue detected?
2. **Impact** - What was affected?
3. **Root Cause** - Why did it happen?
4. **Resolution** - How was it fixed?
5. **Prevention** - How do we prevent recurrence?

### Update Procedures

If rollback procedures were insufficient:
- Update this runbook
- Add new monitoring/alerting
- Enhance testing in CI/CD

---

## Emergency Contacts

| Role | Contact | When to Escalate |
|------|---------|------------------|
| On-Call Engineer | PagerDuty | Immediate |
| Database Admin | Slack #dba | Data issues |
| Security Team | security@tragge.io | Security incidents |
| Platform Lead | Phone | Extended outages |

---

## Revision Control Tips

### Always Maintain Rollback Capability

```bash
# Keep at least 10 revisions
kubectl patch deployment trade-bff -n tragge -p '{"spec":{"revisionHistoryLimit":10}}'

# Before major deploys, record current state
kubectl get deployment trade-bff -n tragge -o yaml > rollback/trade-bff-$(date +%Y%m%d).yaml
```

### Git-Based Rollback

```bash
# Tag deployments
git tag -a "deploy-production-$(date +%Y%m%d%H%M)" -m "Production deployment"
git push origin --tags

# Rollback to tagged state
git checkout deploy-production-20260107
kubectl apply -k infra/k8s/overlays/production
```

---

*Last Updated: January 2026*
