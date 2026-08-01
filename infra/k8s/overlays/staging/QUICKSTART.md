# Staging Overlay - Quick Start

## 🚀 Deploy to Staging (5 minutes)

```bash
# 1. Preview what will be deployed
kubectl kustomize infra/k8s/overlays/staging | less

# 2. Deploy to staging
kubectl apply -k infra/k8s/overlays/staging

# 3. Watch deployment
kubectl get pods -n tragge-staging -w

# 4. Verify health
kubectl get all -n tragge-staging
```

## 🔍 Verify Deployment

```bash
# Check pod status
kubectl get pods -n tragge-staging

# Test health endpoints
curl https://api-staging.tragge.example.com/user/healthz
curl https://api-staging.tragge.example.com/trade/healthz
curl https://api-staging.tragge.example.com/admin/healthz

# Check logs
kubectl logs -f deployment/user-bff -n tragge-staging
```

## 📊 Compare Staging vs Production

```bash
# Run comparison script
cd infra/k8s/overlays/staging
./compare.sh

# View full diff
diff <(kubectl kustomize ../production) <(kubectl kustomize .)
```

## 🔄 Update Image Tags

```bash
# Update to specific SHA (recommended)
cd infra/k8s/overlays/staging
kustomize edit set image tragge/user-bff=tragge/user-bff:sha-abc123

# Apply changes
kubectl apply -k .

# Monitor rollout
kubectl rollout status deployment/user-bff -n tragge-staging
```

## 🧪 Run Tests

```bash
# Integration tests
make test

# E2E tests
make e2e

# Load test
make load-test-ws \
  EMAIL=test@example.com \
  PASSWORD=test123 \
  CONTEST_ID=staging-contest \
  N=50 \
  DURATION=60s \
  WS_URL=wss://ws-staging.tragge.example.com/ws/trade
```

## ⬆️ Promote to Production

```bash
# 1. Verify staging is healthy
kubectl get pods -n tragge-staging
# All pods should be Running

# 2. Get staging image tags
kubectl get deployments -n tragge-staging -o json | \
  jq -r '.items[] | .metadata.name + ": " + .spec.template.spec.containers[0].image'

# 3. Update production kustomization with staging tags
cd infra/k8s/overlays/production
vim kustomization.yaml
# Update image tags from staging

# 4. Deploy to production
kubectl apply -k infra/k8s/overlays/production

# 5. Monitor production rollout
kubectl rollout status deployment/user-bff -n tragge
kubectl get pods -n tragge -w
```

## 🔧 Troubleshooting

```bash
# Pod not starting
kubectl describe pod <pod-name> -n tragge-staging
kubectl logs <pod-name> -n tragge-staging

# Check resource quota
kubectl describe resourcequota -n tragge-staging

# Check ingress
kubectl describe ingress -n tragge-staging

# TLS certificate issues
kubectl get certificate -n tragge-staging
kubectl describe certificate tragge-staging-tls-secret -n tragge-staging
```

## 🧹 Cleanup

```bash
# Delete staging environment
kubectl delete -k infra/k8s/overlays/staging

# Verify deletion
kubectl get ns tragge-staging
```

## 📚 Documentation

- **README.md** - Comprehensive documentation
- **DEPLOYMENT.md** - Detailed deployment commands
- **SUMMARY.md** - Quick overview and key facts
- **compare.sh** - Staging vs production comparison

## 🔑 Key Differences from Production

| Aspect | Production | Staging |
|--------|-----------|---------|
| Namespace | `tragge` | `tragge-staging` |
| Replicas | 3 | 1-2 |
| Resources | 100% | 50% |
| Domain | `tragge.example.com` | `staging.tragge.example.com` |
| TLS Issuer | `letsencrypt-prod` | `letsencrypt-staging` ⚠️ |
| Image Tags | `latest` | `staging` or `sha-xxx` |

⚠️ **Note**: Staging TLS certificates are NOT trusted by browsers (expected behavior)

## 💰 Cost Savings

- **Pods**: 50% fewer (15 vs 30)
- **CPU/Memory**: 75% less
- **Estimated Savings**: ~$135/month

## ✅ Pre-Deployment Checklist

- [ ] Staging infrastructure deployed (PostgreSQL, Redis, Redpanda)
- [ ] DNS configured (staging.tragge.example.com, api-staging.*, ws-staging.*)
- [ ] cert-manager installed
- [ ] NGINX Ingress Controller installed
- [ ] Secrets updated (JWT, API keys)
- [ ] Image tags set to staging builds

## 📞 Support

- **Full Docs**: `infra/k8s/overlays/staging/README.md`
- **Issues**: Create GitHub issue
- **Contact**: platform@tragge.example.com
