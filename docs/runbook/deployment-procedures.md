# Tragge Trading Platform - Deployment Procedures

This document outlines the deployment procedures for the Tragge Trading Platform Kubernetes infrastructure.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Environment Overview](#environment-overview)
3. [Initial Deployment](#initial-deployment)
4. [Rolling Updates](#rolling-updates)
5. [Database Migrations](#database-migrations)
6. [Canary Deployments](#canary-deployments)
7. [Blue-Green Deployments](#blue-green-deployments)
8. [Post-Deployment Verification](#post-deployment-verification)
9. [Troubleshooting](#troubleshooting)

---

## Prerequisites

### Required Tools

```bash
# Kubernetes CLI
kubectl version --client  # v1.28+

# Kustomize (built into kubectl or standalone)
kustomize version  # v5.0+

# Helm (for external dependencies)
helm version  # v3.12+

# AWS CLI (for backups and secrets)
aws --version  # v2.x
```

### Cluster Access

Ensure you have the correct kubeconfig:

```bash
# Staging cluster
export KUBECONFIG=~/.kube/tragge-staging

# Production cluster
export KUBECONFIG=~/.kube/tragge-production

# Verify access
kubectl cluster-info
kubectl auth can-i create deployments -n tragge
```

### Pre-Deployment Checklist

- [ ] All tests pass in CI/CD pipeline
- [ ] Docker images built and pushed to registry
- [ ] Database migrations reviewed (if any)
- [ ] External secrets configured in Vault
- [ ] NOWPayments API key and webhook secret configured when crypto payments are enabled
- [ ] NOWPayments webhook URL registered as `https://yourdomain.com/webhooks/nowpayments`
- [ ] Jibit credentials and callback allowlist configured when Rial payments are enabled
- [ ] Backup verification completed successfully
- [ ] On-call engineer notified
- [ ] Deployment window approved (for production)

---

## Environment Overview

### Staging

```bash
# Preview staging manifests
kubectl kustomize infra/k8s/overlays/staging

# Deploy to staging
kubectl apply -k infra/k8s/overlays/staging

# Verify deployment
kubectl get all -n tragge-staging
```

### Production

```bash
# Preview production manifests
kubectl kustomize infra/k8s/overlays/production

# Deploy to production
kubectl apply -k infra/k8s/overlays/production

# Verify deployment
kubectl get all -n tragge
```

---

## Initial Deployment

### Step 1: Create Namespace and Prerequisites

```bash
# Apply namespace first
kubectl apply -f infra/k8s/base/namespace.yaml

# Install External Secrets Operator (if not installed)
helm repo add external-secrets https://charts.external-secrets.io
helm install external-secrets external-secrets/external-secrets \
  --namespace external-secrets \
  --create-namespace

# Install cert-manager (if not installed)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml

# Install NGINX Ingress Controller (if not installed)
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx \
  --create-namespace
```

### Step 2: Configure External Secrets

```bash
# Verify ClusterSecretStore is configured
kubectl get clustersecretstore tragge-secret-store

# If not, configure your secret provider (Vault example)
# Update infra/k8s/base/external-secrets.yaml with your Vault config
kubectl apply -f infra/k8s/base/external-secrets.yaml
```

### Step 3: Deploy Base Infrastructure

```bash
# Deploy base configuration
kubectl apply -k infra/k8s/base

# Wait for infrastructure pods
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=postgres -n tragge --timeout=300s
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=redis -n tragge --timeout=300s
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=redpanda -n tragge --timeout=300s
```

### Step 4: Run Database Migrations

```bash
# Apply migrations
kubectl run migrate --rm -it --restart=Never \
  --image=migrate/migrate:v4.17.0 \
  -n tragge \
  --env="POSTGRES_DSN=$(kubectl get secret tragge-secrets -n tragge -o jsonpath='{.data.POSTGRES_DSN}' | base64 -d)" \
  -- \
  -path=/migrations -database="${POSTGRES_DSN}" up

# Or use the migration job
kubectl apply -f - <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: db-migration
  namespace: tragge
spec:
  template:
    spec:
      containers:
      - name: migrate
        image: migrate/migrate:v4.17.0
        command: ["/migrate"]
        args: ["-path=/migrations", "-database", "\$(POSTGRES_DSN)", "up"]
        env:
        - name: POSTGRES_DSN
          valueFrom:
            secretKeyRef:
              name: tragge-secrets
              key: POSTGRES_DSN
        volumeMounts:
        - name: migrations
          mountPath: /migrations
      restartPolicy: Never
      volumes:
      - name: migrations
        configMap:
          name: db-migrations
  backoffLimit: 3
EOF
```

### Step 5: Deploy Application Services

```bash
# Deploy all services
kubectl apply -k infra/k8s/overlays/production

# Wait for all deployments
kubectl wait --for=condition=available deployment --all -n tragge --timeout=600s

# Verify pods are running
kubectl get pods -n tragge -o wide
```

### Step 6: Verify Ingress and TLS

```bash
# Check ingress status
kubectl get ingress -n tragge

# Check certificate status
kubectl get certificate -n tragge

# Verify TLS secret exists
kubectl get secret tragge-tls-secret -n tragge
```

---

## Rolling Updates

### Update a Single Service

```bash
# Update image tag in kustomization.yaml or use --set-image
kubectl set image deployment/user-bff user-bff=tragge/user-bff:v1.0.1 -n tragge

# Watch rollout status
kubectl rollout status deployment/user-bff -n tragge

# Check rollout history
kubectl rollout history deployment/user-bff -n tragge
```

### Update All Services

```bash
# Update image tags in overlays/production/kustomization.yaml
# Then apply:
kubectl apply -k infra/k8s/overlays/production

# Watch all rollouts
watch kubectl get pods -n tragge

# Monitor rollout progress
for deploy in user-bff trade-bff admin-bff gateway; do
  kubectl rollout status deployment/$deploy -n tragge
done
```

### Pause/Resume Rollout

```bash
# Pause rollout (for investigation)
kubectl rollout pause deployment/trade-bff -n tragge

# Resume rollout
kubectl rollout resume deployment/trade-bff -n tragge
```

---

## Database Migrations

### Pre-Migration Backup

```bash
# Trigger manual backup before migration
kubectl create job --from=cronjob/postgres-daily-backup postgres-pre-migration-backup -n tragge

# Wait for backup to complete
kubectl wait --for=condition=complete job/postgres-pre-migration-backup -n tragge --timeout=600s

# Verify backup in S3
aws s3 ls s3://tragge-backups/backups/postgres/ --recursive | tail -5
```

### Apply Migration

```bash
# Run migration job
kubectl apply -f infra/k8s/jobs/db-migration.yaml

# Watch migration logs
kubectl logs -f job/db-migration -n tragge

# Verify migration completed
kubectl get job db-migration -n tragge
```

### Migration Rollback

If migration fails, see [Rollback Procedures](./rollback-procedures.md).

---

## Canary Deployments

For critical services, use canary deployments:

```bash
# Create canary deployment (10% traffic)
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: trade-bff-canary
  namespace: tragge
  labels:
    app.kubernetes.io/name: trade-bff
    app.kubernetes.io/version: canary
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: trade-bff
      app.kubernetes.io/version: canary
  template:
    metadata:
      labels:
        app.kubernetes.io/name: trade-bff
        app.kubernetes.io/version: canary
    spec:
      containers:
      - name: trade-bff
        image: tragge/trade-bff:v1.0.1-canary
        # ... rest of container spec
EOF

# Monitor canary metrics
kubectl top pods -l app.kubernetes.io/name=trade-bff -n tragge

# If canary succeeds, promote to production
kubectl set image deployment/trade-bff trade-bff=tragge/trade-bff:v1.0.1 -n tragge

# Delete canary
kubectl delete deployment trade-bff-canary -n tragge
```

---

## Blue-Green Deployments

For zero-downtime deployments with instant rollback capability:

```bash
# Create green deployment
kubectl apply -f infra/k8s/deployments/user-bff-green.yaml

# Wait for green to be ready
kubectl wait --for=condition=available deployment/user-bff-green -n tragge

# Switch service to green
kubectl patch service user-bff -n tragge -p '{"spec":{"selector":{"deployment":"green"}}}'

# Verify traffic flows to green
curl -s https://api.tragge.io/user/health

# If successful, scale down blue
kubectl scale deployment user-bff-blue --replicas=0 -n tragge

# If rollback needed, switch back to blue
kubectl patch service user-bff -n tragge -p '{"spec":{"selector":{"deployment":"blue"}}}'
```

---

## Post-Deployment Verification

### Health Checks

```bash
# Check all pods are running
kubectl get pods -n tragge | grep -v Running

# Check service endpoints
kubectl get endpoints -n tragge

# Verify health endpoints
for svc in user-bff trade-bff admin-bff; do
  echo "=== $svc ==="
  kubectl exec -it deploy/$svc -n tragge -- wget -qO- http://localhost:808X/healthz
done
```

### Smoke Tests

```bash
# Test user registration/login flow
curl -X POST https://api.tragge.io/user/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"testpass123"}'

# Test WebSocket connection
websocat wss://ws.tragge.io/ws/trade -H "Authorization: Bearer $TOKEN"

# Test admin API
curl -X GET https://api.tragge.io/admin/contests \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

### Metrics Verification

```bash
# Check Prometheus targets are up
kubectl port-forward svc/prometheus 9090:9090 -n monitoring &
open http://localhost:9090/targets

# Verify key metrics are being collected
curl -s http://localhost:9090/api/v1/query?query=up | jq '.data.result'

# Check Grafana dashboards
kubectl port-forward svc/grafana 3000:3000 -n monitoring &
open http://localhost:3000
```

---

## Troubleshooting

### Pod Not Starting

```bash
# Check pod events
kubectl describe pod <pod-name> -n tragge

# Check container logs
kubectl logs <pod-name> -n tragge --previous

# Check resource constraints
kubectl top pods -n tragge
kubectl describe nodes | grep -A 10 "Allocated resources"
```

### Service Not Accessible

```bash
# Check service endpoints
kubectl get endpoints <service-name> -n tragge

# Check network policies
kubectl get networkpolicies -n tragge
kubectl describe networkpolicy <policy-name> -n tragge

# Test connectivity from debug pod
kubectl run debug --rm -it --image=nicolaka/netshoot -n tragge -- /bin/bash
# Inside pod: curl http://user-bff:8081/healthz
```

### Database Connection Issues

```bash
# Check PostgreSQL pod
kubectl logs -l app.kubernetes.io/name=postgres -n tragge

# Test database connection
kubectl run psql --rm -it --restart=Never \
  --image=postgres:16-alpine \
  -n tragge \
  --env="PGPASSWORD=<password>" \
  -- psql -h postgres -U app -d app -c "SELECT 1"
```

### External Secrets Not Syncing

```bash
# Check ExternalSecret status
kubectl get externalsecrets -n tragge

# Describe for errors
kubectl describe externalsecret tragge-database-secrets -n tragge

# Check ClusterSecretStore
kubectl get clustersecretstore
kubectl describe clustersecretstore tragge-secret-store
```

---

## Deployment Schedule

### Recommended Windows

| Environment | Day | Time (UTC) | Duration |
|-------------|-----|------------|----------|
| Staging | Any | Any | - |
| Production | Tue-Thu | 14:00-16:00 | 2 hours |

### Blackout Periods

- **Never deploy during:**
  - Market open/close hours (09:30-16:00 ET on trading days)
  - Active trading contests
  - Major economic events

---

## Emergency Contacts

| Role | Contact | Escalation |
|------|---------|------------|
| On-Call Engineer | PagerDuty | Immediate |
| Platform Lead | Slack #platform | 15 min |
| SRE Manager | Phone | 30 min |

---

*Last Updated: January 2026*
