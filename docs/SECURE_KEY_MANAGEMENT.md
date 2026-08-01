# Secure API Key Management

This document describes how API keys and other secrets are managed securely in the Tragge Trading Platform.

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Development Setup](#development-setup)
4. [Production Setup](#production-setup)
5. [Secret Types](#secret-types)
6. [Go Services Integration](#go-services-integration)
7. [Docker Compose Setup](#docker-compose-setup)
8. [Kubernetes Setup](#kubernetes-setup)
9. [Key Rotation](#key-rotation)
10. [Security Best Practices](#security-best-practices)
11. [Troubleshooting](#troubleshooting)

---

## Overview

The platform uses a layered approach to secrets management:

| Environment | Secrets Storage | Access Method |
|-------------|-----------------|---------------|
| Development | Docker secrets (files) | `/run/secrets/` mount |
| Staging | External Secrets Operator | Kubernetes secrets |
| Production | AWS Secrets Manager / HashiCorp Vault | External Secrets Operator |

### Key Principles

1. **Never commit secrets** - All secret files are in `.gitignore`
2. **Secrets as files** - Docker secrets pattern for portability
3. **Environment fallback** - Services support both file and env var sources
4. **Automatic rotation** - Support for multiple API keys with auto-rotation
5. **Least privilege** - Services only access secrets they need

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Secrets Flow                                  │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Development:                                                        │
│  ┌──────────────┐    ┌────────────────┐    ┌──────────────────────┐ │
│  │ secrets/*.txt │───►│ Docker Compose │───►│ /run/secrets/* mount │ │
│  └──────────────┘    └────────────────┘    └──────────────────────┘ │
│                                                                      │
│  Production:                                                         │
│  ┌──────────────┐    ┌────────────────────┐    ┌─────────────────┐  │
│  │ AWS Secrets  │───►│ External Secrets   │───►│ Kubernetes      │  │
│  │ Manager      │    │ Operator           │    │ Secrets         │  │
│  └──────────────┘    └────────────────────┘    └─────────────────┘  │
│                                                                      │
│  Service Loading:                                                    │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                    packages/secrets                           │   │
│  │  1. Check {NAME}_FILE env var → read file                    │   │
│  │  2. Check {NAME} env var → use value                         │   │
│  │  3. Check /run/secrets/{name} → read file                    │   │
│  └──────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Development Setup

### Quick Start

```bash
# 1. Initialize secrets (creates random passwords, placeholder API keys)
./scripts/secrets/init-secrets.sh

# 2. Edit API key files with your actual keys
nano infra/docker/secrets/twelvedata_api_keys.txt
nano infra/docker/secrets/massive_api_keys.txt

# 3. Start services
cd infra/docker && docker compose up -d
```

### Migrating from .env

If you have an existing `.env` file with secrets:

```bash
# Extract secrets from .env to Docker secrets
./scripts/secrets/migrate-from-env.sh

# Verify migration
ls -la infra/docker/secrets/

# Update .env to remove sensitive values
# (The migration script shows which values to remove)
```

### Secret Files Location

```
infra/docker/secrets/
├── .gitignore                    # Ignores *.txt files
├── README.md                     # Usage documentation
├── postgres_password.txt         # PostgreSQL app password
├── jwt_secret.txt               # JWT signing secret
├── twelvedata_api_keys.txt      # TwelveData API keys (comma-separated)
├── massive_api_keys.txt         # Massive API keys (comma-separated)
├── resend_api_key.txt           # Resend email API key
├── discord_webhook_url.txt      # Discord webhook URL
├── nowpayments_api_key.txt      # NOWPayments API key
├── nowpayments_ipn_secret.txt   # NOWPayments webhook secret
├── jibit_api_key.txt            # Jibit API key
├── jibit_secret_key.txt         # Jibit secret key
├── *.example.txt                # Example files (safe to commit)
```

### File Format

Each secret file contains a single value (or comma-separated list for API keys):

```bash
# Single value (postgres_password.txt)
my-secure-password-here

# Multiple values for rotation (twelvedata_api_keys.txt)
key1,key2,key3
```

---

## Production Setup

### AWS Secrets Manager

1. **Create secrets in AWS Secrets Manager:**

```bash
# Create market data secrets
aws secretsmanager create-secret \
    --name tragge/production/market-data \
    --secret-string '{
        "twelvedata_api_key": "your-primary-key",
        "twelvedata_api_key_alt": "your-backup-key",
        "massive_api_key": "your-primary-key",
        "massive_api_key_alt": "your-backup-key"
    }'

# Create database secrets
aws secretsmanager create-secret \
    --name tragge/production/database \
    --secret-string '{
        "dsn": "<managed PostgreSQL connection secret>"
    }'
```

2. **Configure External Secrets Operator:**

See `infra/k8s/base/external-secrets.yaml` for the configuration.

### HashiCorp Vault

1. **Store secrets in Vault:**

```bash
# Enable KV secrets engine
vault secrets enable -path=secret kv-v2

# Store market data secrets
vault kv put secret/tragge/production/market-data \
    twelvedata_api_key="your-key" \
    massive_api_key="your-key"
```

2. **Configure Kubernetes auth:**

```bash
vault write auth/kubernetes/role/tragge-app \
    bound_service_account_names=external-secrets \
    bound_service_account_namespaces=external-secrets \
    policies=tragge-read-secrets \
    ttl=1h
```

---

## Secret Types

### Managed Secrets

| Secret | Purpose | Rotation Frequency |
|--------|---------|-------------------|
| `POSTGRES_PASSWORD` | Database access | 90 days |
| `JWT_SECRET` | Token signing | 180 days |
| `TWELVEDATA_API_KEYS` | Market data (TwelveData) | On compromise |
| `MASSIVE_API_KEYS` | Market data (Massive) | On compromise |
| `RESEND_API_KEY` | Email notifications | On compromise |
| `DISCORD_WEBHOOK_URL` | Alert notifications | On compromise |
| `NOWPAYMENTS_API_KEY` | Crypto payments (NOWPayments) | On compromise |
| `NOWPAYMENTS_IPN_SECRET` | NOWPayments webhook signature verification | On compromise |
| `JIBIT_API_KEY` | Rial payments (Jibit) | On compromise |
| `JIBIT_SECRET_KEY` | Jibit request authentication | On compromise |
| `GRAFANA_ADMIN_PASSWORD` | Grafana dashboard access | 90 days |

### Multi-Key Support

API keys support multiple values for automatic rotation on rate limits:

```bash
# infra/docker/secrets/twelvedata_api_keys.txt
primary-key,secondary-key,tertiary-key
```

When the service hits rate limits, it automatically rotates to the next key:

```
[KeyRotator] Rotated to key index 2/3 (total rotations: 5)
```

---

## Go Services Integration

### Using the Secrets Package

The `packages/secrets` package provides a unified interface for loading secrets:

```go
import "github.com/Parsaeffatravesh/tragge/packages/secrets"

// Load single secret (checks file, env var, default path)
password := secrets.Load("POSTGRES_PASSWORD")

// Load with default value
jwtSecret := secrets.LoadWithDefault("JWT_SECRET", "dev-secret")

// Load required (panics if not found)
apiKey := secrets.LoadRequired("TWELVEDATA_API_KEYS")

// Load comma-separated list (for key rotation)
keys := secrets.LoadList("TWELVEDATA_API_KEYS")

// Build PostgreSQL DSN from components
dsn := secrets.BuildPostgresDSN()

// Get JWT secret with fallback
jwt := secrets.GetJWTSecret()
```

### Secret Loading Priority

1. `{NAME}_FILE` environment variable (path to secret file)
2. `{NAME}` environment variable (direct value)
3. `/run/secrets/{name}` (Docker secrets default path)

### Diagnostic Logging

```go
// Get loading diagnostics for debugging
diagnostics := secrets.DiagnosticReport("TWELVEDATA_API_KEYS", "POSTGRES_PASSWORD")
for _, diag := range diagnostics {
    log.Printf("[Secrets] %s: loaded=%v source=%s",
        diag.Name, diag.Loaded, diag.Source)
}
```

Output:
```
[Secrets] TWELVEDATA_API_KEYS: loaded=true source=file:/run/secrets/twelvedata_api_keys
[Secrets] POSTGRES_PASSWORD: loaded=true source=file:/run/secrets/postgres_password
```

---

## Docker Compose Setup

### Defining Secrets

```yaml
# docker-compose.yml
secrets:
  twelvedata_api_keys:
    file: ./secrets/twelvedata_api_keys.txt
  massive_api_keys:
    file: ./secrets/massive_api_keys.txt
  jwt_secret:
    file: ./secrets/jwt_secret.txt
  postgres_password:
    file: ./secrets/postgres_password.txt
  nowpayments_api_key:
    file: ./secrets/nowpayments_api_key.txt
  nowpayments_ipn_secret:
    file: ./secrets/nowpayments_ipn_secret.txt
```

### Using Secrets in Services

```yaml
services:
  market-ingestor:
    environment:
      # Point to secret files
      TWELVEDATA_API_KEYS_FILE: /run/secrets/twelvedata_api_keys
      MASSIVE_API_KEYS_FILE: /run/secrets/massive_api_keys
    secrets:
      - twelvedata_api_keys
      - massive_api_keys
```

### Verifying Secrets

```bash
# Check if secrets are mounted
docker compose exec market-ingestor ls -la /run/secrets/

# View secret content (development only!)
docker compose exec market-ingestor cat /run/secrets/twelvedata_api_keys
```

---

## Kubernetes Setup

### External Secrets Configuration

The `infra/k8s/base/external-secrets.yaml` defines how secrets are synced from external providers:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: tragge-market-data-secrets
  namespace: tragge
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: tragge-secret-store
    kind: ClusterSecretStore
  target:
    name: tragge-market-data-secrets
    creationPolicy: Owner
  data:
    - secretKey: TWELVEDATA_API_KEY
      remoteRef:
        key: tragge/production/market-data
        property: twelvedata_api_key
```

### Using Secrets in Deployments

```yaml
spec:
  containers:
    - name: market-ingestor
      env:
        - name: TWELVEDATA_API_KEYS
          valueFrom:
            secretKeyRef:
              name: tragge-market-data-secrets
              key: TWELVEDATA_API_KEY
```

### Verifying External Secrets

```bash
# Check ExternalSecret status
kubectl get externalsecret -n tragge

# View synced secret (base64 encoded)
kubectl get secret tragge-market-data-secrets -n tragge -o jsonpath='{.data}'

# Force refresh
kubectl annotate externalsecret tragge-market-data-secrets \
    -n tragge \
    force-sync=$(date +%s) --overwrite
```

---

## Key Rotation

### API Key Rotation Procedure

See `docs/runbook/api-key-rotation.md` for detailed instructions.

**Quick steps:**

1. Generate new key in provider dashboard
2. Add new key to secrets store
3. Restart affected services
4. Verify connectivity
5. Revoke old key

### Zero-Downtime Rotation

The platform supports zero-downtime rotation using multiple keys:

```bash
# Add new key alongside existing
echo "old-key,new-key" > infra/docker/secrets/twelvedata_api_keys.txt

# Restart service (picks up both keys)
docker compose restart market-ingestor

# After verification, remove old key
echo "new-key" > infra/docker/secrets/twelvedata_api_keys.txt

# Restart again
docker compose restart market-ingestor

# Revoke old key in provider dashboard
```

---

## Security Best Practices

### Do's

- **Generate strong secrets**: Use `openssl rand -base64 32`
- **Rotate regularly**: Follow the rotation schedule in credential-rotation.md
- **Use least privilege**: Only mount secrets that services need
- **Audit access**: Monitor secret access in production
- **Encrypt at rest**: Enable encryption in your secrets manager
- **Use separate keys per environment**: Dev, staging, production

### Don'ts

- **Never commit secrets**: Even to private repos
- **Never log secrets**: `secrets.MaskSecret()` emits only `[REDACTED]`; follow the [central secure-observability policy](security/secure-observability-and-redaction.md)
- **Never share secrets**: Use separate credentials per team member
- **Never use default values in production**: Change all defaults
- **Never store secrets in `.env` for production**: Use proper secrets management

### File Permissions

```bash
# Secret files should be readable only by owner
chmod 600 infra/docker/secrets/*.txt

# Verify permissions
ls -la infra/docker/secrets/
# Expected: -rw------- 1 user user ... filename.txt
```

---

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| "Secret not found" | File missing or wrong path | Check file exists and _FILE env var |
| "Permission denied" | File permissions too restrictive | Ensure Docker can read the file |
| "Empty secret" | File is empty or only whitespace | Add content to secret file |
| "Invalid API key" | Key revoked or typo | Verify key in provider dashboard |

### Debug Commands

```bash
# Check if secrets are mounted (Docker)
docker compose exec market-ingestor ls -la /run/secrets/

# Check environment variables
docker compose exec market-ingestor env | grep -E "_FILE|API"

# View service logs for secret loading
docker compose logs market-ingestor | grep -i secret

# Test secret loading manually
docker compose exec market-ingestor sh -c 'cat $TWELVEDATA_API_KEYS_FILE'
```

### Kubernetes Debug

```bash
# Check ExternalSecret status
kubectl describe externalsecret tragge-market-data-secrets -n tragge

# Check if secret was created
kubectl get secret tragge-market-data-secrets -n tragge

# Check pod environment
kubectl exec -it deployment/market-ingestor -n tragge -- env | grep API

# Check secret mount
kubectl exec -it deployment/market-ingestor -n tragge -- ls -la /etc/secrets/
```

---

## Related Documentation

- [API Key Rotation Guide](runbook/api-key-rotation.md)
- [Credential Rotation Runbook](runbook/credential-rotation.md)
- [External Secrets Configuration](../infra/k8s/base/external-secrets.yaml)
- [Docker Compose Secrets](../infra/docker/secrets/README.md)

---

*Last updated: January 2026*
