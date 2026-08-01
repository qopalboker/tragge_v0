# Staging Deployment Guide

Quick reference guide for deploying and managing the Tragge staging environment.

## Quick Start

```bash
# 1. Preview changes
kubectl kustomize infra/k8s/overlays/staging | less

# 2. Deploy to staging
kubectl apply -k infra/k8s/overlays/staging

# 3. Watch deployment
kubectl get pods -n tragge-staging -w

# 4. Verify health
kubectl get all -n tragge-staging
```

## Deployment Commands

### Initial Deployment

```bash
# Create staging namespace and deploy all resources
kubectl apply -k infra/k8s/overlays/staging

# Wait for all deployments to be ready
kubectl wait --for=condition=available --timeout=300s \
  deployment --all -n tragge-staging
```

### Update Image Tags

```bash
# Update to specific commit SHA
cd infra/k8s/overlays/staging

# Method 1: Using kustomize edit
kustomize edit set image \
  tragge/user-bff=tragge/user-bff:sha-abc123 \
  tragge/trade-bff=tragge/trade-bff:sha-abc123 \
  tragge/admin-bff=tragge/admin-bff:sha-abc123

# Method 2: Manual edit of kustomization.yaml
vim kustomization.yaml
# Update the 'images' section

# Apply changes
kubectl apply -k .

# Verify rollout
kubectl rollout status deployment/user-bff -n tragge-staging
```

### Rollback Deployment

```bash
# Rollback specific deployment
kubectl rollout undo deployment/user-bff -n tragge-staging

# Rollback to specific revision
kubectl rollout undo deployment/user-bff -n tragge-staging --to-revision=2

# Check rollout history
kubectl rollout history deployment/user-bff -n tragge-staging
```

### Scale Services

```bash
# Scale up for load testing
kubectl scale deployment/trade-bff --replicas=3 -n tragge-staging

# Scale down to save resources
kubectl scale deployment/trading-engine --replicas=0 -n tragge-staging

# Auto-scale with HPA (if configured)
kubectl autoscale deployment/trade-bff \
  --min=2 --max=5 --cpu-percent=70 -n tragge-staging
```

## Verification Commands

### Check Deployment Status

```bash
# All resources
kubectl get all -n tragge-staging

# Deployments only
kubectl get deployments -n tragge-staging

# Pods with node placement
kubectl get pods -n tragge-staging -o wide

# Services and endpoints
kubectl get svc,endpoints -n tragge-staging

# Ingress and TLS
kubectl get ingress,certificate -n tragge-staging
```

### Health Checks

```bash
# Check all pod health
kubectl get pods -n tragge-staging

# Check specific service
kubectl exec -it deployment/user-bff -n tragge-staging -- \
  wget -qO- http://localhost:8081/healthz

# Check from outside cluster
curl https://api-staging.tragge.example.com/user/healthz
curl https://api-staging.tragge.example.com/trade/healthz
curl https://api-staging.tragge.example.com/admin/healthz
```

### View Logs

```bash
# Tail logs from deployment
kubectl logs -f deployment/user-bff -n tragge-staging

# Logs from all pods of a deployment
kubectl logs -f deployment/user-bff --all-containers -n tragge-staging

# Logs from specific pod
kubectl logs <pod-name> -n tragge-staging

# Previous container logs (after crash)
kubectl logs <pod-name> -n tragge-staging --previous

# Logs with timestamps
kubectl logs deployment/user-bff -n tragge-staging --timestamps
```

### Resource Usage

```bash
# Pod resource usage
kubectl top pods -n tragge-staging

# Sorted by CPU
kubectl top pods -n tragge-staging --sort-by=cpu

# Sorted by memory
kubectl top pods -n tragge-staging --sort-by=memory

# Check against quota
kubectl describe resourcequota -n tragge-staging
```

## Database Operations

### PostgreSQL

```bash
# Connect to staging database
kubectl run -it --rm psql-client \
  --image=postgres:16-alpine \
  --restart=Never \
  --namespace=tragge-staging \
  -- psql "postgres://app:app@postgres-staging:5432/app_staging?sslmode=disable"

# Run migrations
kubectl exec -it deployment/user-bff -n tragge-staging -- \
  /app/migrate -database "postgres://..." -path /migrations up

# Backup staging database
kubectl exec postgres-staging-0 -n tragge-staging -- \
  pg_dump -U app app_staging > staging-backup-$(date +%Y%m%d).sql

# Restore staging database
kubectl exec -i postgres-staging-0 -n tragge-staging -- \
  psql -U app app_staging < staging-backup-20260106.sql
```

### Redis

```bash
# Connect to staging Redis
kubectl run -it --rm redis-client \
  --image=redis:7-alpine \
  --restart=Never \
  --namespace=tragge-staging \
  -- redis-cli -h redis-staging

# Check Redis memory usage
kubectl exec -it redis-staging-0 -n tragge-staging -- \
  redis-cli INFO memory

# Flush staging Redis (CAREFUL!)
kubectl exec -it redis-staging-0 -n tragge-staging -- \
  redis-cli FLUSHDB
```

## TLS Certificate Management

### Check Certificate Status

```bash
# List certificates
kubectl get certificate -n tragge-staging

# Describe certificate
kubectl describe certificate tragge-staging-tls-secret -n tragge-staging

# Check certificate secret
kubectl get secret tragge-staging-tls-secret -n tragge-staging -o yaml

# View certificate details
kubectl get secret tragge-staging-tls-secret -n tragge-staging \
  -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -text -noout
```

### Force Certificate Renewal

```bash
# Delete certificate to force renewal
kubectl delete certificate tragge-staging-tls-secret -n tragge-staging

# Reapply ingress to trigger new certificate
kubectl apply -k infra/k8s/overlays/staging

# Watch cert-manager logs
kubectl logs -f -n cert-manager deployment/cert-manager
```

## Troubleshooting

### Pod Stuck in Pending

```bash
# Check pod events
kubectl describe pod <pod-name> -n tragge-staging

# Check resource availability
kubectl describe nodes

# Check resource quota
kubectl describe resourcequota -n tragge-staging
```

### Pod Crashing (CrashLoopBackOff)

```bash
# Check logs
kubectl logs <pod-name> -n tragge-staging

# Check previous logs
kubectl logs <pod-name> -n tragge-staging --previous

# Describe pod for events
kubectl describe pod <pod-name> -n tragge-staging

# Check readiness/liveness probes
kubectl get pod <pod-name> -n tragge-staging -o yaml | grep -A 10 "Probe"
```

### Service Not Accessible

```bash
# Check service endpoints
kubectl get endpoints -n tragge-staging

# Check service selector
kubectl describe service user-bff -n tragge-staging

# Test service from inside cluster
kubectl run -it --rm debug \
  --image=curlimages/curl \
  --restart=Never \
  --namespace=tragge-staging \
  -- curl http://user-bff:8081/healthz
```

### Ingress Not Working

```bash
# Check ingress status
kubectl describe ingress tragge-ingress -n tragge-staging

# Check NGINX controller logs
kubectl logs -n ingress-nginx deployment/ingress-nginx-controller

# Check ingress controller service
kubectl get svc -n ingress-nginx

# Test DNS resolution
nslookup staging.tragge.example.com
```

### Database Connection Issues

```bash
# Check database pod
kubectl get pods -n tragge-staging | grep postgres

# Check database logs
kubectl logs postgres-staging-0 -n tragge-staging

# Test connection from pod
kubectl exec -it deployment/user-bff -n tragge-staging -- \
  sh -c 'apk add postgresql-client && psql "$POSTGRES_DSN" -c "SELECT 1"'

# Check secret
kubectl get secret tragge-secrets -n tragge-staging -o yaml
```

## Comparison with Production

### View Differences

```bash
# Compare manifests
diff <(kubectl kustomize infra/k8s/overlays/production) \
     <(kubectl kustomize infra/k8s/overlays/staging)

# Compare specific resource
diff <(kubectl kustomize infra/k8s/overlays/production | grep -A 50 "kind: Deployment" | head -50) \
     <(kubectl kustomize infra/k8s/overlays/staging | grep -A 50 "kind: Deployment" | head -50)

# Compare running resources
diff <(kubectl get all -n tragge -o yaml) \
     <(kubectl get all -n tragge-staging -o yaml)
```

### Side-by-Side Comparison

```bash
# Resource requests/limits
echo "=== PRODUCTION ==="
kubectl get pods -n tragge -o json | \
  jq '.items[] | .spec.containers[] | {name, resources}'

echo "=== STAGING ==="
kubectl get pods -n tragge-staging -o json | \
  jq '.items[] | .spec.containers[] | {name, resources}'

# Replica counts
echo "=== PRODUCTION ==="
kubectl get deployments -n tragge

echo "=== STAGING ==="
kubectl get deployments -n tragge-staging
```

## Promoting to Production

### Pre-Promotion Checklist

- [ ] All staging tests passing
- [ ] No critical errors in logs
- [ ] Performance metrics acceptable
- [ ] Database migrations tested
- [ ] TLS certificates working (staging issuer)
- [ ] WebSocket connections stable
- [ ] Load testing completed
- [ ] Security scan passed

### Promotion Steps

```bash
# 1. Get current staging image tags
kubectl get deployments -n tragge-staging -o json | \
  jq -r '.items[] | .metadata.name + ": " + .spec.template.spec.containers[0].image'

# 2. Update production kustomization.yaml with staging tags
cd infra/k8s/overlays/production
# Edit kustomization.yaml and update image tags

# 3. Preview production changes
kubectl kustomize infra/k8s/overlays/production | less

# 4. Deploy to production (with caution!)
kubectl apply -k infra/k8s/overlays/production

# 5. Monitor rollout
kubectl rollout status deployment/user-bff -n tragge
kubectl rollout status deployment/trade-bff -n tragge
kubectl rollout status deployment/admin-bff -n tragge

# 6. Verify production health
kubectl get pods -n tragge
curl https://api.tragge.example.com/user/healthz
curl https://api.tragge.example.com/trade/healthz

# 7. Monitor logs and metrics
kubectl logs -f deployment/user-bff -n tragge
```

## Cleanup

### Delete Staging Environment

```bash
# Delete all resources
kubectl delete -k infra/k8s/overlays/staging

# Verify deletion
kubectl get all -n tragge-staging

# Force delete namespace if stuck
kubectl delete ns tragge-staging --force --grace-period=0
```

### Delete Specific Components

```bash
# Delete specific deployment
kubectl delete deployment user-bff -n tragge-staging

# Delete ingress
kubectl delete ingress tragge-ingress -n tragge-staging

# Delete TLS certificate
kubectl delete certificate tragge-staging-tls-secret -n tragge-staging
```

## Monitoring & Metrics

### Prometheus Queries

```promql
# Request rate by service (staging)
rate(http_requests_total{namespace="tragge-staging"}[5m])

# Error rate
rate(http_requests_total{namespace="tragge-staging",status=~"5.."}[5m])

# Pod CPU usage
container_cpu_usage_seconds_total{namespace="tragge-staging"}

# Pod memory usage
container_memory_usage_bytes{namespace="tragge-staging"}
```

### Grafana Dashboards

Access Grafana at `http://grafana.tragge.example.com`:

1. **Tragge Staging Overview** - Overall platform health
2. **Staging WebSocket Metrics** - WebSocket performance
3. **Staging Database Metrics** - PostgreSQL performance
4. **Staging Kafka Metrics** - Message queue health

### Alerts

Common alerts to monitor:

- Pod crash loop
- High error rate (>5%)
- Memory usage >80%
- CPU throttling
- Database connection pool exhausted
- Kafka consumer lag

## Best Practices

1. **Always preview before applying**: Use `kubectl kustomize` first
2. **Use specific image tags**: Never use `:latest` or `:staging` in production
3. **Monitor resource usage**: Ensure staging stays within quota
4. **Test TLS thoroughly**: Staging issuer certificates won't be trusted
5. **Keep staging in sync**: Regularly update to match production config
6. **Document changes**: Add comments to patches explaining why
7. **Test rollbacks**: Practice rollback procedures in staging
8. **Monitor logs**: Watch for errors after deployment
9. **Backup before changes**: Always backup staging database
10. **Use GitOps**: Commit changes to version control before applying

## Useful Scripts

### Deploy Script

```bash
#!/bin/bash
# deploy-staging.sh
set -e

echo "Deploying to staging..."

# Preview changes
kubectl kustomize infra/k8s/overlays/staging

read -p "Continue with deployment? (y/n) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    exit 1
fi

# Deploy
kubectl apply -k infra/k8s/overlays/staging

# Wait for rollout
kubectl wait --for=condition=available --timeout=300s \
  deployment --all -n tragge-staging

echo "Deployment complete!"
```

### Health Check Script

```bash
#!/bin/bash
# health-check.sh

echo "=== Pod Status ==="
kubectl get pods -n tragge-staging

echo -e "\n=== Service Endpoints ==="
kubectl get endpoints -n tragge-staging

echo -e "\n=== Ingress Status ==="
kubectl get ingress -n tragge-staging

echo -e "\n=== Health Checks ==="
curl -f https://api-staging.tragge.example.com/user/healthz && echo "✓ user-bff"
curl -f https://api-staging.tragge.example.com/trade/healthz && echo "✓ trade-bff"
curl -f https://api-staging.tragge.example.com/admin/healthz && echo "✓ admin-bff"
```

---

**Last Updated**: 2026-01-06
