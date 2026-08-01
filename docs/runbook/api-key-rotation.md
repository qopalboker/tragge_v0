# API Key Rotation Guide

This document provides step-by-step instructions for rotating API keys for market data providers (TwelveData and Massive) in the Tragge Trading Platform.

## Table of Contents

1. [Overview](#overview)
2. [TwelveData API Key Rotation](#twelvedata-api-key-rotation)
3. [Massive API Key Rotation](#massive-api-key-rotation)
4. [Zero-Downtime Rotation Strategy](#zero-downtime-rotation-strategy)
5. [Emergency Rotation](#emergency-rotation)
6. [Verification](#verification)
7. [Automation](#automation)

---

## Overview

### Key Inventory

| Provider | Key Type | Rate Limits | Rotation Frequency |
|----------|----------|-------------|-------------------|
| TwelveData | WebSocket API Key | 8 symbols (free tier) | As needed / On compromise |
| Massive | WebSocket API Key | Per-plan limits | As needed / On compromise |

### Where Keys Are Stored

| Environment | Location | Format |
|-------------|----------|--------|
| Development | `.env` file | Environment variables |
| Docker Compose | Docker secrets | Files in `/run/secrets/` |
| Kubernetes | External Secrets | Synced from Vault/AWS SM |

---

## TwelveData API Key Rotation

### Step 1: Generate New Key in TwelveData Dashboard

1. Log in to [TwelveData Dashboard](https://twelvedata.com/account/api-keys)
2. Navigate to **API Keys** section
3. Click **Generate New API Key**
4. Copy the new API key securely (it won't be shown again)
5. Add a label for identification (e.g., `tragge-prod-2026-01`)

### Step 2: Add New Key to Secrets Store

**For Docker Compose (Development):**

```bash
# Add new key to secrets file (supports multiple keys for rotation)
echo "new-key-here" >> secrets/twelvedata_api_keys.txt

# Or replace entirely
echo "new-key-1
new-key-2" > secrets/twelvedata_api_keys.txt
```

**For Kubernetes (Production):**

```bash
# Update in AWS Secrets Manager
aws secretsmanager put-secret-value \
    --secret-id tragge/production/market-data \
    --secret-string '{
        "twelvedata_api_key": "new-primary-key",
        "twelvedata_api_key_alt": "old-key-for-rollback"
    }'

# Or HashiCorp Vault
vault kv put secret/tragge/production/market-data \
    twelvedata_api_key="new-primary-key" \
    twelvedata_api_key_alt="old-key-for-rollback"
```

### Step 3: Restart Market Ingestor

**Docker Compose:**

```bash
# Graceful restart (recommended)
docker compose restart market-ingestor

# Or with full recreation
docker compose up -d --force-recreate market-ingestor
```

**Kubernetes:**

```bash
# Trigger secret refresh (External Secrets will sync automatically)
kubectl annotate externalsecret tragge-market-data-secrets \
    -n tragge \
    force-sync=$(date +%s) --overwrite

# Rolling restart to pick up new secrets
kubectl rollout restart deployment/market-ingestor -n tragge
kubectl rollout status deployment/market-ingestor -n tragge
```

### Step 4: Verify New Key

```bash
# Check health endpoint
curl http://localhost:8084/readyz | jq .

# Expected response:
# {
#   "status": "ready",
#   "provider": "twelvedata",
#   "websocket": "connected",
#   "using_fallback": false
# }

# Check logs for successful connection
docker compose logs market-ingestor --tail=50 | grep -i "connected"
```

### Step 5: Revoke Old Key

1. Return to [TwelveData Dashboard](https://twelvedata.com/account/api-keys)
2. Locate the old API key
3. Click **Delete** or **Revoke**
4. Confirm deletion

---

## Massive API Key Rotation

### Step 1: Generate New Key in Massive Dashboard

1. Log in to the Massive provider dashboard
2. Navigate to **API Keys** section
3. Click **Generate API Key** (or regenerate existing)
4. Copy the new API key

### Step 2: Add New Key to Secrets Store

**For Docker Compose (Development):**

```bash
# Update secrets file
echo "new-massive-key" > secrets/massive_api_keys.txt
```

**For Kubernetes (Production):**

```bash
# Update in AWS Secrets Manager
aws secretsmanager put-secret-value \
    --secret-id tragge/production/market-data \
    --secret-string '{
        "massive_api_key": "new-primary-key",
        "massive_api_key_alt": "old-key-for-rollback"
    }'
```

### Step 3: Restart Market Ingestor

Same as TwelveData steps above.

### Step 4: Verify Connection

```bash
# Force fallback to Massive for testing (temporary)
# In .env or secrets:
# MARKET_PROVIDER=massive

# Check health
curl http://localhost:8084/readyz | jq .
```

---

## Zero-Downtime Rotation Strategy

For production environments requiring zero downtime:

### Multi-Key Rotation

The market-ingestor supports multiple API keys for automatic rotation:

```bash
# Format: comma-separated keys
TWELVEDATA_API_KEYS=key1,key2,key3
MASSIVE_API_KEYS=key1,key2
```

When a key hits rate limits, the service automatically rotates to the next key.

### Rolling Update Procedure

1. **Add new key alongside existing keys:**
   ```bash
   # Update secret with both old and new keys
   TWELVEDATA_API_KEYS=old-key,new-key
   ```

2. **Deploy with both keys active:**
   ```bash
   kubectl rollout restart deployment/market-ingestor -n tragge
   ```

3. **Monitor for stability (wait 24h):**
   ```bash
   # Watch for rate limit errors
   kubectl logs -f deployment/market-ingestor -n tragge | grep -i "rate"
   ```

4. **Remove old key:**
   ```bash
   TWELVEDATA_API_KEYS=new-key
   ```

5. **Revoke old key in provider dashboard**

---

## Emergency Rotation

Use this procedure if API keys are compromised.

### Immediate Actions

```bash
#!/bin/bash
# emergency-rotate-api-keys.sh

set -e

echo "=== EMERGENCY API KEY ROTATION ==="
echo "Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)"

# 1. Scale down to stop using compromised keys
echo "Scaling down market-ingestor..."
kubectl scale deployment/market-ingestor -n tragge --replicas=0

# 2. Generate new keys (manual step)
echo ""
echo "ACTION REQUIRED:"
echo "1. Go to https://twelvedata.com/account/api-keys"
echo "2. Revoke ALL existing keys immediately"
echo "3. Generate new key"
echo "4. Enter new TwelveData key:"
read -s TWELVEDATA_NEW_KEY

echo "5. Go to the Massive provider dashboard"
echo "6. Regenerate API key"
echo "7. Enter new Massive key:"
read -s MASSIVE_NEW_KEY

# 3. Update secrets
echo "Updating secrets..."
kubectl create secret generic tragge-market-data-secrets \
    --from-literal=TWELVEDATA_API_KEY="$TWELVEDATA_NEW_KEY" \
    --from-literal=MASSIVE_API_KEY="$MASSIVE_NEW_KEY" \
    --dry-run=client -o yaml | kubectl apply -n tragge -f -

# 4. Scale back up
echo "Scaling up market-ingestor..."
kubectl scale deployment/market-ingestor -n tragge --replicas=2

# 5. Verify
echo "Waiting for pods to be ready..."
kubectl wait --for=condition=ready pod -l app=market-ingestor -n tragge --timeout=120s

# 6. Check health
echo "Checking health..."
kubectl exec -it deployment/market-ingestor -n tragge -- wget -qO- http://localhost:8084/readyz

echo ""
echo "=== EMERGENCY ROTATION COMPLETE ==="
echo "Document this incident in the incident log"
```

### Post-Incident Actions

1. **Document the incident:**
   - Timeline of events
   - How compromise was discovered
   - Affected systems
   - Remediation steps taken

2. **Review access logs:**
   ```bash
   # Check for unauthorized API usage
   # TwelveData: Check usage in dashboard
   # Massive: Check API call logs
   ```

3. **Notify stakeholders:**
   - Security team
   - Platform operations
   - Management (if data breach suspected)

---

## Verification

### Health Check Commands

```bash
# Basic health check
curl -s http://localhost:8084/healthz | jq .

# Readiness check (includes WebSocket status)
curl -s http://localhost:8084/readyz | jq .

# Check provider status in logs
docker compose logs market-ingestor --tail=20 | grep -E "(Connected|TwelveData|Massive)"
```

### Expected Log Output (Successful Rotation)

```
[TwelveData] Connecting with key index 1/2
[TwelveData] WebSocket connected
[ProviderManager] Connected to twelvedata
[TwelveData] Subscribed to: [AAPL MSFT GOOGL]
```

### Monitoring Dashboards

- **Grafana WebSocket Dashboard:** Check for connection drops
- **Prometheus Metrics:** `market_ingestor_websocket_connected`
- **Alerts:** Monitor for provider failover notifications

---

## Automation

### Scheduled Key Rotation Reminder

Add to Kubernetes CronJob:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: api-key-rotation-reminder
  namespace: tragge
spec:
  schedule: "0 9 1 */3 *"  # Quarterly reminder
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: reminder
            image: curlimages/curl
            command:
            - /bin/sh
            - -c
            - |
              curl -X POST "$DISCORD_WEBHOOK_URL" \
                -H 'Content-Type: application/json' \
                -d '{
                  "embeds": [{
                    "title": "API Key Rotation Reminder",
                    "description": "Quarterly reminder to review and rotate market data API keys for TwelveData and Massive",
                    "color": 16776960,
                    "fields": [
                      {"name": "Action Required", "value": "Review API key age and rotate if needed"},
                      {"name": "Documentation", "value": "See docs/runbook/api-key-rotation.md"}
                    ]
                  }]
                }'
            envFrom:
            - secretRef:
                name: tragge-notification-secrets
          restartPolicy: Never
```

### AWS Secrets Manager Automatic Rotation

```bash
# Create rotation Lambda function (use AWS template)
aws secretsmanager rotate-secret \
    --secret-id tragge/production/market-data \
    --rotation-lambda-arn arn:aws:lambda:REGION:ACCOUNT:function:MarketDataKeyRotation \
    --rotation-rules AutomaticallyAfterDays=90

# Note: This requires custom Lambda to interact with TwelveData/Massive APIs
```

---

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| `TWELVEDATA_API_KEYS not configured` | Secret not mounted | Check secret volume mounts |
| `dial failed` | Invalid/revoked key | Verify key in provider dashboard |
| `rate limit exceeded` | Too many requests | Add more keys for rotation |
| `Switching to fallback` | Primary provider down | Check TwelveData service status |

### Debug Commands

```bash
# Check if secrets are mounted (Docker)
docker compose exec market-ingestor ls -la /run/secrets/

# Check if secrets are mounted (Kubernetes)
kubectl exec -it deployment/market-ingestor -n tragge -- ls -la /etc/secrets/

# Check environment variables
kubectl exec -it deployment/market-ingestor -n tragge -- env | grep -E "(TWELVE|MASSIVE)"
```

---

*Last updated: January 2026*
