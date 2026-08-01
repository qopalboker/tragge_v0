# Kubernetes Deployment

This directory contains Kubernetes manifests for deploying the tragge trading platform.

## Structure

```
k8s/
├── base/                    # Base manifests
│   ├── namespace.yaml       # Namespace definition
│   ├── configmap.yaml       # Configuration (non-sensitive)
│   ├── secrets.yaml         # Secrets (REPLACE VALUES!)
│   ├── postgres.yaml        # PostgreSQL StatefulSet
│   ├── redis.yaml           # Redis Deployment
│   ├── redpanda.yaml        # Redpanda (Kafka) StatefulSet
│   ├── user-bff.yaml        # User BFF service
│   ├── trade-bff.yaml       # Trade BFF service
│   ├── admin-bff.yaml       # Admin BFF service
│   ├── market-ingestor.yaml # Market data ingestion
│   ├── trading-engine.yaml  # Order processing engine
│   ├── leaderboard-worker.yaml # Leaderboard computation
│   ├── frontend.yaml   # User web app
│   ├── frontend.yaml  # Trading interface
│   ├── frontend.yaml  # Admin dashboard
│   ├── gateway.yaml         # Nginx gateway
│   ├── ingress.yaml         # Ingress routing
│   └── kustomization.yaml   # Base kustomization
└── overlays/
    └── production/
        └── kustomization.yaml  # Production overrides
```

## Prerequisites

1. **Kubernetes cluster** (1.25+)
2. **kubectl** configured
3. **NGINX Ingress Controller** installed
4. **Container registry** with built images

## Building Images

Build all Docker images from the repository root:

```bash
# Build all Go services
docker build -t tragge/user-bff:latest -f apps/user-bff/Dockerfile .
docker build -t tragge/trade-bff:latest -f apps/trade-bff/Dockerfile .
docker build -t tragge/admin-bff:latest -f apps/admin-bff/Dockerfile .
docker build -t tragge/market-ingestor:latest -f apps/market-ingestor/Dockerfile .
docker build -t tragge/trading-engine:latest -f apps/trading-engine/Dockerfile .
docker build -t tragge/leaderboard-worker:latest -f apps/leaderboard-worker/Dockerfile .

# Build frontend images
docker build -t tragge/frontend:latest apps/frontend/
docker build -t tragge/frontend:latest apps/frontend/
docker build -t tragge/frontend:latest apps/frontend/
```

## Deployment

### 1. Update Secrets

Before deploying, update the secrets in `base/secrets.yaml`:

```yaml
# IMPORTANT: Replace these values!
POSTGRES_DSN: "postgres://user:password@postgres:5432/app?sslmode=require"
JWT_SECRET: "your-strong-256-bit-secret-here"
TWELVEDATA_API_KEY: "your-api-key"
MASSIVE_API_KEY: "your-api-key"
POSTGRES_PASSWORD: "strong-database-password"
```

For production, use a secrets management solution like:
- Kubernetes Secrets with encryption at rest
- HashiCorp Vault
- AWS Secrets Manager
- Azure Key Vault

### 2. Deploy with Kustomize

```bash
# Preview what will be created
kubectl kustomize infra/k8s/base

# Deploy base configuration
kubectl apply -k infra/k8s/base

# Or deploy production overlay
kubectl apply -k infra/k8s/overlays/production
```

### 3. Verify Deployment

```bash
# Check all resources
kubectl get all -n tragge

# Check pods are running
kubectl get pods -n tragge

# Check services
kubectl get svc -n tragge

# Check ingress
kubectl get ingress -n tragge
```

## Accessing the Application

Once deployed with Ingress, access the application at:

| Path | Description |
|------|-------------|
| `/user` | User frontend |
| `/trade` | Trading interface |
| `/admin` | Admin dashboard |
| `/api/user/*` | User API |
| `/api/trade/*` | Trade API |
| `/api/admin/*` | Admin API |
| `/ws/trade` | WebSocket trading |

## Scaling

```bash
# Scale a specific deployment
kubectl scale deployment user-bff --replicas=5 -n tragge

# Or use HPA (Horizontal Pod Autoscaler)
kubectl autoscale deployment user-bff --min=2 --max=10 --cpu-percent=80 -n tragge
```

## Monitoring

The services expose health and readiness endpoints:

| Service | Liveness | Readiness |
|---------|----------|-----------|
| Go services | `/healthz` | `/readyz` |
| Frontends | `/health` | `/health` |
| Gateway | `/health` | `/health` |

## Database Migrations

Run migrations using a Job:

```bash
kubectl run migrate --image=tragge/migrate:latest \
  --rm -it --restart=Never -n tragge \
  -- migrate -database "$POSTGRES_DSN" -path /migrations up
```

## Logs

```bash
# View logs for a service
kubectl logs -l app.kubernetes.io/name=user-bff -n tragge

# Follow logs
kubectl logs -f deployment/trading-engine -n tragge

# All pods in namespace
kubectl logs -l app.kubernetes.io/part-of=tragge-platform -n tragge
```

## Troubleshooting

### Pods not starting

```bash
# Describe pod for events
kubectl describe pod <pod-name> -n tragge

# Check resource constraints
kubectl top pods -n tragge
```

### Database connection issues

```bash
# Test connection from within cluster
kubectl run psql --image=postgres:16-alpine --rm -it --restart=Never -n tragge \
  -- psql "$POSTGRES_DSN" -c "SELECT 1"
```

### Redis connection issues

```bash
kubectl run redis-cli --image=redis:7-alpine --rm -it --restart=Never -n tragge \
  -- redis-cli -h redis ping
```

## Cleanup

```bash
# Delete all resources
kubectl delete -k infra/k8s/base

# Or delete namespace (removes everything)
kubectl delete namespace tragge
```
