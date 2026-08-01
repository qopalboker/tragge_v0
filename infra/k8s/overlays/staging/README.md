# Tragge Staging Overlay

This directory contains Kustomize overlays for deploying the Tragge trading platform to a **staging environment**.

## Overview

The staging overlay provides a pre-production environment for testing changes before deploying to production. It differs from production in several key ways:

| Aspect | Production | Staging |
|--------|-----------|---------|
| **Namespace** | `tragge` | `tragge-staging` |
| **Replicas** | 3 (all services) | 1-2 (reduced) |
| **Resources** | Full limits | 50% of production |
| **Domain** | `tragge.example.com` | `staging.tragge.example.com` |
| **TLS Issuer** | `letsencrypt-prod` | `letsencrypt-staging` |
| **Database** | Production PostgreSQL | Staging PostgreSQL |
| **Image Tags** | `latest` | `staging` or `sha-xxx` |

## Directory Structure

```
staging/
├── kustomization.yaml           # Main Kustomize configuration
├── namespace.yaml               # Namespace with resource quotas
├── patches/
│   ├── replicas-patch.yaml      # Reduce replicas (1-2)
│   ├── resources-patch.yaml     # Reduce CPU/memory (50%)
│   ├── ingress-patch.yaml       # Staging domain configuration
│   └── tls-patch.yaml           # Let's Encrypt staging issuer
└── README.md                    # This file
```

## Configuration Details

### Replica Configuration

| Service | Production | Staging | Rationale |
|---------|-----------|---------|-----------|
| **BFFs** (user/trade/admin) | 3 | 2 | HA testing, sticky sessions |
| **Gateway** | 3 | 2 | Load balancing testing |
| **Trading Engine** | 3 | 1 | Single worker sufficient |
| **Market Ingestor** | 3 | 1 | Single worker sufficient |
| **Leaderboard Worker** | 3 | 1 | Single worker sufficient |
| **Frontends** | 3 | 1 | Static content, low traffic |

### Resource Limits

All resource limits are reduced to **50% of production values**:

| Service | Production Memory | Staging Memory | Production CPU | Staging CPU |
|---------|------------------|----------------|----------------|-------------|
| user-bff | 64Mi / 256Mi | 32Mi / 128Mi | 50m / 500m | 25m / 250m |
| trade-bff | 128Mi / 512Mi | 64Mi / 256Mi | 100m / 1000m | 50m / 500m |
| admin-bff | 64Mi / 256Mi | 32Mi / 128Mi | 50m / 500m | 25m / 250m |
| trading-engine | 256Mi / 1Gi | 128Mi / 512Mi | 200m / 2000m | 100m / 1000m |
| market-ingestor | 128Mi / 512Mi | 64Mi / 256Mi | 100m / 1000m | 50m / 500m |
| leaderboard-worker | 128Mi / 512Mi | 64Mi / 256Mi | 100m / 1000m | 50m / 500m |

### Domain Configuration

**Staging domains** (replace `example.com` with your actual domain):

- **Frontend**: `staging.tragge.example.com`
- **API**: `api-staging.tragge.example.com`
- **WebSocket**: `ws-staging.tragge.example.com`

### TLS/SSL Configuration

Staging uses **Let's Encrypt staging issuer** for TLS certificates:

- **Issuer**: `letsencrypt-staging`
- **Certificate**: Not trusted by browsers (expected)
- **Rate Limits**: Much higher than production
- **Purpose**: Test certificate automation without production impact

**IMPORTANT**: Browsers will show "Not Secure" warnings. This is normal for staging.

### Database Configuration

Staging connects to separate infrastructure:

- **PostgreSQL**: `postgres-staging:5432/app_staging`
- **Redis**: `redis-staging:6379`
- **Redpanda**: `redpanda-staging:9092`

**IMPORTANT**: Update these values in `kustomization.yaml` to match your staging infrastructure.

## Deployment

### Prerequisites

1. **Kubernetes cluster** with kubectl access
2. **Kustomize** (v4.0+) or kubectl with built-in Kustomize
3. **cert-manager** installed for TLS certificates
4. **NGINX Ingress Controller** installed
5. **Staging infrastructure** (PostgreSQL, Redis, Redpanda) deployed

### Preview Changes

Before deploying, preview what will be applied:

```bash
# Preview all resources
kubectl kustomize infra/k8s/overlays/staging

# Preview specific resource types
kubectl kustomize infra/k8s/overlays/staging | grep "kind: Deployment" -A 20

# Compare with production
diff <(kubectl kustomize infra/k8s/overlays/production) \
     <(kubectl kustomize infra/k8s/overlays/staging)
```

### Deploy to Staging

```bash
# Deploy all resources
kubectl apply -k infra/k8s/overlays/staging

# Watch deployment progress
kubectl get pods -n tragge-staging -w

# Check deployment status
kubectl rollout status deployment/user-bff -n tragge-staging
kubectl rollout status deployment/trade-bff -n tragge-staging
kubectl rollout status deployment/admin-bff -n tragge-staging
```

### Verify Deployment

```bash
# Check all resources
kubectl get all -n tragge-staging

# Check pod status
kubectl get pods -n tragge-staging -o wide

# Check service endpoints
kubectl get svc -n tragge-staging

# Check ingress
kubectl get ingress -n tragge-staging

# Check TLS certificate
kubectl get certificate -n tragge-staging
kubectl describe certificate tragge-staging-tls-secret -n tragge-staging

# Check logs
kubectl logs -n tragge-staging deployment/user-bff --tail=100
kubectl logs -n tragge-staging deployment/trade-bff --tail=100
```

### Update Image Tags

To deploy specific image versions:

```bash
# Edit kustomization.yaml and update image tags
cd infra/k8s/overlays/staging

# Update to specific SHA
kustomize edit set image tragge/user-bff=tragge/user-bff:sha-abc123
kustomize edit set image tragge/trade-bff=tragge/trade-bff:sha-abc123

# Or use staging tag
kustomize edit set image tragge/user-bff=tragge/user-bff:staging-2024-01-15

# Apply changes
kubectl apply -k .
```

### CI/CD Integration

Example GitHub Actions workflow:

```yaml
name: Deploy to Staging

on:
  push:
    branches: [develop, staging]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Update image tags
        working-directory: infra/k8s/overlays/staging
        run: |
          # Update all images to commit SHA
          COMMIT_SHA=${{ github.sha }}
          kustomize edit set image tragge/user-bff=tragge/user-bff:${COMMIT_SHA:0:7}
          kustomize edit set image tragge/trade-bff=tragge/trade-bff:${COMMIT_SHA:0:7}
          # ... update other images

      - name: Deploy to staging
        run: kubectl apply -k infra/k8s/overlays/staging
```

## Testing in Staging

### Smoke Tests

```bash
# Test user registration
curl -X POST https://api-staging.tragge.example.com/user/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"test123"}'

# Test login
curl -X POST https://api-staging.tragge.example.com/user/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"test123"}'

# Test health endpoints
curl https://api-staging.tragge.example.com/user/healthz
curl https://api-staging.tragge.example.com/trade/healthz
curl https://api-staging.tragge.example.com/admin/healthz
```

### WebSocket Testing

```bash
# Test WebSocket connection
wscat -c wss://ws-staging.tragge.example.com/ws/trade \
  -H "Authorization: Bearer <token>" \
  -H "X-Contest-ID: <contest-id>"
```

### Load Testing

```bash
# Run WebSocket load test against staging
make load-test-ws \
  EMAIL=test@example.com \
  PASSWORD=test123 \
  CONTEST_ID=staging-contest-1 \
  N=50 \
  DURATION=60s \
  WS_URL=wss://ws-staging.tragge.example.com/ws/trade
```

## Promoting to Production

Once staging is validated, promote to production:

### Option 1: Manual Promotion

```bash
# 1. Tag staging images as production
docker tag tragge/user-bff:staging tragge/user-bff:latest
docker tag tragge/trade-bff:staging tragge/trade-bff:latest
# ... tag other images

# 2. Push to registry
docker push tragge/user-bff:latest
docker push tragge/trade-bff:latest
# ... push other images

# 3. Deploy to production
kubectl apply -k infra/k8s/overlays/production

# 4. Monitor rollout
kubectl rollout status deployment/user-bff -n tragge
```

### Option 2: Automated Promotion (GitOps)

```yaml
# GitHub Actions workflow
name: Promote to Production

on:
  workflow_dispatch:
    inputs:
      staging_tag:
        description: 'Staging tag to promote'
        required: true

jobs:
  promote:
    runs-on: ubuntu-latest
    steps:
      - name: Retag images
        run: |
          # Pull staging images
          docker pull tragge/user-bff:${{ github.event.inputs.staging_tag }}

          # Retag as latest
          docker tag tragge/user-bff:${{ github.event.inputs.staging_tag }} \
                     tragge/user-bff:latest

          # Push to registry
          docker push tragge/user-bff:latest

      - name: Deploy to production
        run: kubectl apply -k infra/k8s/overlays/production
```

## Troubleshooting

### Pods Not Starting

```bash
# Check pod status
kubectl get pods -n tragge-staging

# Describe pod
kubectl describe pod <pod-name> -n tragge-staging

# Check logs
kubectl logs <pod-name> -n tragge-staging

# Check events
kubectl get events -n tragge-staging --sort-by='.lastTimestamp'
```

### TLS Certificate Issues

```bash
# Check certificate status
kubectl get certificate -n tragge-staging
kubectl describe certificate tragge-staging-tls-secret -n tragge-staging

# Check cert-manager logs
kubectl logs -n cert-manager deployment/cert-manager

# Delete and recreate certificate
kubectl delete certificate tragge-staging-tls-secret -n tragge-staging
kubectl apply -k infra/k8s/overlays/staging
```

### Database Connection Issues

```bash
# Test PostgreSQL connection
kubectl run -it --rm postgres-test \
  --image=postgres:16-alpine \
  --restart=Never \
  --namespace=tragge-staging \
  -- psql "postgres://app:app@postgres-staging:5432/app_staging?sslmode=disable"

# Test Redis connection
kubectl run -it --rm redis-test \
  --image=redis:7-alpine \
  --restart=Never \
  --namespace=tragge-staging \
  -- redis-cli -h redis-staging
```

### Ingress Not Working

```bash
# Check ingress status
kubectl get ingress -n tragge-staging
kubectl describe ingress tragge-ingress -n tragge-staging

# Check NGINX controller logs
kubectl logs -n ingress-nginx deployment/ingress-nginx-controller

# Verify DNS resolution
nslookup staging.tragge.example.com
nslookup api-staging.tragge.example.com
nslookup ws-staging.tragge.example.com
```

## Cleanup

To completely remove the staging environment:

```bash
# Delete all resources
kubectl delete -k infra/k8s/overlays/staging

# Verify namespace is deleted
kubectl get ns tragge-staging

# If namespace is stuck, force delete
kubectl delete ns tragge-staging --force --grace-period=0
```

## Resource Monitoring

### Check Resource Usage

```bash
# Pod resource usage
kubectl top pods -n tragge-staging

# Node resource usage
kubectl top nodes

# Check against quotas
kubectl describe resourcequota -n tragge-staging
```

### Grafana Dashboards

Access staging metrics in Grafana:

1. Navigate to `http://grafana.tragge.example.com`
2. Select "Tragge Staging" dashboard
3. Filter by `environment=staging` label

## Best Practices

1. **Always test in staging first** before deploying to production
2. **Use specific image tags** (SHA or version) instead of `staging` tag for reproducibility
3. **Monitor resource usage** to ensure staging doesn't consume excessive cluster resources
4. **Keep staging in sync** with production configuration (except overrides)
5. **Test TLS certificates** in staging before promoting to production
6. **Run integration tests** against staging after each deployment
7. **Document any staging-specific quirks** or differences from production

## Environment Variables

Staging-specific environment variables are set in `kustomization.yaml`:

```yaml
configMapGenerator:
  - name: staging-config
    literals:
      - ENVIRONMENT=staging
      - LOG_LEVEL=debug
      - ENABLE_METRICS=true
      - ENABLE_TRACING=true
```

To add more environment variables:

```bash
cd infra/k8s/overlays/staging

# Edit kustomization.yaml
# Add to configMapGenerator or secretGenerator

# Apply changes
kubectl apply -k .
```

## Support

For issues or questions:

- **Documentation**: See `docs/` directory
- **Runbooks**: See `docs/runbook/`
- **Issues**: Create a GitHub issue
- **Contact**: platform@tragge.example.com

---

**Last Updated**: 2026-01-06
