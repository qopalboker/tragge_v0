# Staging Overlay Summary

## Overview

The staging overlay provides a **resource-efficient, pre-production environment** for testing the Tragge trading platform before deploying to production.

## Quick Facts

| Aspect | Value |
|--------|-------|
| **Namespace** | `tragge-staging` |
| **Environment** | Pre-production testing |
| **Resource Usage** | 50% of production |
| **Replica Strategy** | 1-2 (vs 3 in production) |
| **Cost Impact** | ~60% savings vs production |

## File Structure

```
staging/
├── kustomization.yaml              # Main overlay configuration
├── namespace.yaml                  # Namespace with quotas & limits
├── patches/
│   ├── replicas-patch.yaml         # Reduce replicas to 1-2
│   ├── resources-patch.yaml        # 50% CPU/memory reduction
│   ├── ingress-patch.yaml          # Staging domain configuration
│   └── tls-patch.yaml              # Let's Encrypt staging issuer
├── README.md                       # Comprehensive documentation
├── DEPLOYMENT.md                   # Deployment commands reference
├── SUMMARY.md                      # This file
└── compare.sh                      # Staging vs production comparison
```

## Configuration Changes

### 1. Namespace Isolation

**Production**: `tragge`
**Staging**: `tragge-staging`

The staging namespace includes:
- **Resource Quotas**: Limits total resource consumption
- **Limit Ranges**: Sets default pod limits
- **Network Policies**: Inherits from base

### 2. Replica Reduction

| Service Type | Production | Staging | Savings |
|--------------|-----------|---------|---------|
| BFFs (user/trade/admin) | 3 | 2 | 33% |
| Gateway | 3 | 2 | 33% |
| Workers (engine/ingestor) | 3 | 1 | 67% |
| Frontends | 3 | 1 | 67% |

**Overall pod reduction**: ~50%

### 3. Resource Limits

All services reduced to **50% of production**:

```yaml
# Production (user-bff)
resources:
  requests:
    memory: "64Mi"
    cpu: "50m"
  limits:
    memory: "256Mi"
    cpu: "500m"

# Staging (user-bff)
resources:
  requests:
    memory: "32Mi"     # 50%
    cpu: "25m"         # 50%
  limits:
    memory: "128Mi"    # 50%
    cpu: "250m"        # 50%
```

**Total resource savings**: ~60-70% (combined replica + limit reduction)

### 4. Image Tags

| Environment | Tag Strategy |
|------------|--------------|
| Production | `latest` (stable releases) |
| Staging | `staging` or `sha-abc123` (test builds) |

**CI/CD Integration**: Automatically update tags on push to `develop` or `staging` branches.

### 5. Domain Configuration

| Type | Production | Staging |
|------|-----------|---------|
| Frontend | `tragge.example.com` | `staging.tragge.example.com` |
| API | `api.tragge.example.com` | `api-staging.tragge.example.com` |
| WebSocket | `ws.tragge.example.com` | `ws-staging.tragge.example.com` |

### 6. TLS Certificates

| Environment | Issuer | Trusted? | Rate Limit |
|------------|--------|----------|------------|
| Production | `letsencrypt-prod` | ✅ Yes | 50/week |
| Staging | `letsencrypt-staging` | ❌ No | Very High |

**Note**: Staging certificates will show browser warnings. This is expected.

### 7. Infrastructure

Staging connects to separate infrastructure:

| Service | Production | Staging |
|---------|-----------|---------|
| PostgreSQL | `postgres:5432/app` | `postgres-staging:5432/app_staging` |
| Redis | `redis:6379` | `redis-staging:6379` |
| Redpanda | `redpanda:9092` | `redpanda-staging:9092` |

## Deployment Workflow

```
┌─────────────────┐
│  Code Changes   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Build Images  │ (CI/CD)
│  Tag: staging   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Deploy Staging  │
│ kubectl apply   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Run Tests      │
│  - Integration  │
│  - E2E          │
│  - Load Test    │
└────────┬────────┘
         │
         ▼
    ┌────────┐
    │ Pass?  │
    └───┬─┬──┘
        │ │
    Yes │ │ No
        │ └──────► Fix Issues
        ▼
┌─────────────────┐
│ Promote to Prod │
│ Tag: latest     │
└─────────────────┘
```

## Use Cases

### 1. Pre-Production Testing

Test changes in a production-like environment before deploying to production:

```bash
# Deploy feature branch to staging
kubectl apply -k infra/k8s/overlays/staging

# Run integration tests
make e2e

# Run load tests
make load-test-ws
```

### 2. Database Migration Testing

Test schema changes without affecting production:

```bash
# Apply migrations to staging
kubectl exec -it deployment/user-bff -n tragge-staging -- \
  /app/migrate -database "$POSTGRES_DSN" up

# Verify data integrity
# Run queries, check constraints, etc.

# If successful, apply to production
```

### 3. TLS Certificate Testing

Test certificate issuance and renewal without hitting production rate limits:

```bash
# Deploy with staging issuer
kubectl apply -k infra/k8s/overlays/staging

# Verify certificate issuance
kubectl get certificate -n tragge-staging

# Test automatic renewal
kubectl delete certificate tragge-staging-tls-secret -n tragge-staging
# Watch cert-manager recreate it
```

### 4. Load Testing

Test system behavior under load without affecting production:

```bash
# Run WebSocket load test
make load-test-ws \
  WS_URL=wss://ws-staging.tragge.example.com/ws/trade \
  N=100 \
  DURATION=300s

# Monitor metrics in Grafana
# Check for errors, latency spikes, resource exhaustion
```

### 5. Configuration Changes

Test configuration changes (environment variables, feature flags) in isolation:

```bash
# Edit staging ConfigMap
kubectl edit configmap staging-config -n tragge-staging

# Restart pods to pick up changes
kubectl rollout restart deployment/user-bff -n tragge-staging

# Verify behavior
curl https://api-staging.tragge.example.com/user/healthz
```

## Cost Analysis

### Resource Comparison

| Metric | Production | Staging | Savings |
|--------|-----------|---------|---------|
| **Pods** | ~30 | ~15 | 50% |
| **CPU Requests** | ~3000m | ~750m | 75% |
| **CPU Limits** | ~15000m | ~3750m | 75% |
| **Memory Requests** | ~3Gi | ~750Mi | 75% |
| **Memory Limits** | ~12Gi | ~3Gi | 75% |

### Estimated Monthly Cost

Assuming AWS EKS pricing (us-east-1):

| Environment | vCPU-hours/month | Memory GB-hours/month | Estimated Cost |
|------------|------------------|----------------------|----------------|
| Production | ~360 | ~8640 | ~$180/month |
| Staging | ~90 | ~2160 | ~$45/month |
| **Savings** | **75%** | **75%** | **$135/month** |

**Note**: Actual costs vary by provider, region, and instance types.

## Limitations

### What Staging Does NOT Test

1. **Production Scale**: Staging has fewer replicas and lower resources
2. **Production Traffic**: Staging won't see real user load patterns
3. **Production Data**: Staging uses separate, smaller datasets
4. **Multi-Region**: Staging is typically single-region
5. **TLS Trust**: Staging certificates aren't trusted by browsers

### Staging is NOT a Substitute For

- **Canary Deployments**: Gradual production rollouts
- **Blue-Green Deployments**: Zero-downtime production switches
- **Chaos Engineering**: Production resilience testing
- **Security Audits**: Full security assessments

## Comparison Commands

### Quick Comparison

```bash
# Run comparison script
./infra/k8s/overlays/staging/compare.sh

# Compare specific resource type
./infra/k8s/overlays/staging/compare.sh Deployment
./infra/k8s/overlays/staging/compare.sh Ingress
```

### Manual Comparison

```bash
# Preview both environments
kubectl kustomize infra/k8s/overlays/production > prod.yaml
kubectl kustomize infra/k8s/overlays/staging > staging.yaml

# View diff
diff prod.yaml staging.yaml

# Or use a better diff tool
code --diff prod.yaml staging.yaml
```

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| Pods pending | Resource quota exceeded | Check `kubectl describe resourcequota -n tragge-staging` |
| TLS not working | Staging issuer not trusted | Expected behavior, use production issuer for trusted certs |
| DB connection failed | Wrong DSN | Check secret: `kubectl get secret tragge-secrets -n tragge-staging -o yaml` |
| Ingress 404 | DNS not configured | Verify DNS points to ingress controller |
| OOM kills | Memory limits too low | Increase limits in `resources-patch.yaml` |

### Debug Commands

```bash
# Check overall status
kubectl get all -n tragge-staging

# Check pod logs
kubectl logs -f deployment/user-bff -n tragge-staging

# Describe problem pod
kubectl describe pod <pod-name> -n tragge-staging

# Check resource usage
kubectl top pods -n tragge-staging

# Check quota
kubectl describe resourcequota -n tragge-staging
```

## Best Practices

### ✅ DO

- Deploy all changes to staging first
- Run full test suite after deployment
- Monitor resource usage and adjust limits
- Keep staging in sync with production config
- Use specific image tags (SHA or version)
- Document any staging-specific quirks
- Test rollback procedures in staging
- Backup staging database before risky changes

### ❌ DON'T

- Deploy untested changes directly to production
- Use staging for long-running experiments
- Share secrets between staging and production
- Rely on staging for production-scale testing
- Skip monitoring and logging setup
- Use `:latest` or `:staging` tags in production
- Leave staging running when not in use (cost)
- Store production data in staging

## Promotion Checklist

Before promoting staging to production:

- [ ] All tests passing (unit, integration, E2E)
- [ ] No errors in logs for 24 hours
- [ ] Performance metrics acceptable
- [ ] Database migrations successful
- [ ] TLS certificates working (staging issuer)
- [ ] WebSocket connections stable
- [ ] Load testing completed successfully
- [ ] Security scan passed
- [ ] Documentation updated
- [ ] Rollback plan documented
- [ ] Team notified of deployment
- [ ] Monitoring dashboards configured

## Next Steps

1. **Customize Configuration**: Update domains, secrets, and resource limits
2. **Set Up CI/CD**: Automate deployments from `develop` branch
3. **Configure Monitoring**: Set up Grafana dashboards for staging
4. **Test Thoroughly**: Run full test suite in staging
5. **Promote to Production**: Follow promotion checklist

## Resources

- **Comprehensive Docs**: [README.md](./README.md)
- **Deployment Guide**: [DEPLOYMENT.md](./DEPLOYMENT.md)
- **Comparison Tool**: [compare.sh](./compare.sh)
- **Kustomize Docs**: https://kustomize.io
- **cert-manager Docs**: https://cert-manager.io

---

**Created**: 2026-01-06
**Last Updated**: 2026-01-06
**Maintainer**: Platform Team
