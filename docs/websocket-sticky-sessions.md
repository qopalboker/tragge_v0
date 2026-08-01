# WebSocket Sticky Sessions by Contest

## Problem

When trade-bff scales to multiple pods behind a Kubernetes Service, WebSocket connections are distributed randomly. This means clients for the same contest may be spread across all pods. Each pod must then independently process ALL contest events and perform its own Redis lookups for leaderboard data, even if it serves very few clients for a given contest.

## Solution: Contest-Aware Load Balancing

### Option A — Nginx Ingress Header-Based Routing (Current, v1)

Use the `contest_id` query parameter from the WebSocket connection URL as a consistent hash key in the Nginx Ingress controller.

**How it works:**

The `tragge-websocket-ingress` Ingress resource in `infra/k8s/base/ingress.yaml` uses the annotation:

```yaml
nginx.ingress.kubernetes.io/upstream-hash-by: "$arg_contest_id"
```

This tells the Nginx Ingress controller to use consistent hashing on the `contest_id` query parameter (`$arg_contest_id` is Nginx's variable for a URL query parameter named `contest_id`). All WebSocket connections with the same `contest_id` value will be routed to the same trade-bff pod.

**Client connection URL format:**

```
wss://ws.tragge.io/ws?contest_id=<uuid>&token=<jwt>
```

**Benefits:**

- All clients for the same contest land on the same pod, maximizing cache locality for leaderboard data and contest state
- Reduces redundant Redis queries — one pod handles all clients per contest instead of every pod processing every contest's events
- Reconnections for the same contest naturally land on the same pod (same hash key = same pod)
- Consistent hashing minimizes disruption during pod scaling — only contests hashed to added/removed pods are affected

**Fallback behavior:**

- The trade-bff handler requires `contest_id` as a mandatory query parameter (returns HTTP 400 if missing), so all WebSocket connections will always have a valid hash key
- Non-WebSocket requests to the same Ingress (e.g., health checks) without `contest_id` hash on an empty string, which is acceptable

**Scaling characteristics:**

- Works well up to ~5000 concurrent users
- Contest distribution across pods depends on hash distribution — with many contests this is naturally balanced
- Monitor for "hot contests" (contests with disproportionately many users) via Prometheus metrics on per-pod connection counts

### Option B — Application-Level Routing (Future, 5000+ Users)

For larger deployments where precise control over contest-to-pod mapping is needed:

1. **Trade-bff pods register** which contests they handle in Redis (e.g., `contest-routing:{contest_id} -> pod_ip`)
2. **A thin routing proxy** sits in front of trade-bff, reads the contest-to-pod mapping from Redis, and routes WebSocket connections accordingly
3. **Rebalancing logic** handles pod scaling events by migrating contests between pods with graceful WebSocket handoff

**When to consider Option B:**

- More than 5000 concurrent users across many contests
- Need to rebalance contests across pods without relying on hash distribution
- Hot contests need to be explicitly isolated to dedicated pods
- Need to drain a specific pod's contests before maintenance

**Trade-offs:**

- Significantly more complex (custom proxy, Redis registration, rebalancing)
- Additional point of failure (the routing proxy)
- More operational overhead

## Verification

After deploying the updated Ingress, verify contest-aware routing:

```bash
# Check that the annotation is applied
kubectl get ingress tragge-websocket-ingress -n tragge -o jsonpath='{.metadata.annotations.nginx\.ingress\.kubernetes\.io/upstream-hash-by}'
# Expected: $arg_contest_id

# Monitor per-pod WebSocket connections
kubectl exec -n tragge deploy/trade-bff -- curl -s localhost:8082/ws/stats

# Verify same contest routes to same pod (connect twice with same contest_id)
# Both connections should be handled by the same pod
```

## Related Files

| File | Purpose |
|------|---------|
| `infra/k8s/base/ingress.yaml` | Ingress resources with contest-aware hashing |
| `apps/trade-bff/main.go` | WebSocket handler (reads `contest_id` from query params) |
| `infra/k8s/base/trade-bff.yaml` | Trade-BFF deployment and service |
| `apps/gateway/nginx.conf` | Development gateway (Docker Compose) |
