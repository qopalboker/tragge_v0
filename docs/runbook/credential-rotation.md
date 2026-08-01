# Credential Rotation Runbook

This document outlines procedures for rotating PostgreSQL credentials and other secrets in the tragge Trading Tournament Platform.

## Table of Contents

1. [Overview](#overview)
2. [Credential Inventory](#credential-inventory)
3. [Rotation Schedule](#rotation-schedule)
4. [PostgreSQL Credential Rotation](#postgresql-credential-rotation)
5. [JWT Secret Rotation](#jwt-secret-rotation)
6. [API Key Rotation](#api-key-rotation)
7. [Emergency Rotation](#emergency-rotation)
8. [Automation](#automation)

---

## Overview

### Security Requirements

| Credential Type | Rotation Frequency | Minimum Length | Complexity |
|----------------|-------------------|----------------|------------|
| PostgreSQL Admin Password | 90 days | 48 characters | URL-safe alphanumeric |
| PostgreSQL App Password | 90 days | 48 characters | URL-safe alphanumeric |
| PostgreSQL Readonly Password | 90 days | 48 characters | URL-safe alphanumeric |
| PgBouncer Auth Password | 90 days | 32 characters | URL-safe alphanumeric |
| JWT Secret | 180 days | 32 characters | Base64-encoded random |
| API Keys | As needed | Provider-defined | Provider-defined |
| Redis Password | 90 days | 32 characters | Alphanumeric + special |
| SSL Certificates | 365 days | N/A | RSA 2048+ / EC P-256+ |

### Prerequisites

Before rotating credentials:

1. Ensure you have administrative access to all affected systems
2. Schedule maintenance window (recommend 15-30 minutes)
3. Notify stakeholders of potential brief service interruption
4. Have rollback plan ready
5. Test in staging environment first

---

## Credential Inventory

### PostgreSQL Credentials (Role-Based Access Control)

| User | Purpose | Privileges | Connection Limit |
|------|---------|------------|------------------|
| `tragge_admin` | Migrations, DDL, maintenance | ALL on database | 10 |
| `tragge_app` | Application operations | SELECT, INSERT, UPDATE on tables | 100 |
| `tragge_readonly` | Replica connections, reporting | SELECT only | 50 |
| `tragge_replication` | Streaming replication | REPLICATION | 5 |
| `pgbouncer_auth` | PgBouncer authentication lookup | EXECUTE on auth function | 5 |

### Services and Database Users

| Service | Primary User | Replica User | Operations |
|---------|-------------|--------------|------------|
| user-bff | tragge_app | tragge_readonly | User auth, profiles, contests |
| admin-bff | tragge_app | tragge_readonly | Contest management, audit |
| trade-bff | tragge_app | tragge_readonly | WebSocket, trading |
| trading-engine | tragge_app | tragge_readonly | Order processing |
| leaderboard-worker | tragge_app | tragge_readonly | Leaderboard updates |
| Migrations | tragge_admin | N/A | Schema changes |

### Configuration Locations

| Environment | Location | Format |
|-------------|----------|--------|
| Docker Compose (dev) | `./secrets/*.txt` | Docker Secrets |
| Docker Compose (legacy) | `.env` file | Environment variables |
| Kubernetes (dev/staging) | `secrets.yaml` | Kubernetes Secret |
| Kubernetes (production) | External Secrets | AWS Secrets Manager / Vault |

### Secret File Structure

```
infra/docker/secrets/
├── postgres_admin_password.txt    # tragge_admin password
├── postgres_app_password.txt      # tragge_app password
├── postgres_readonly_password.txt # tragge_readonly password
├── pgbouncer_auth_password.txt    # pgbouncer_auth password
├── jwt_secret.txt                 # JWT signing secret
├── twelvedata_api_keys.txt        # Market data API keys
├── massive_api_keys.txt           # Market data API keys
└── .db_credentials                # Combined reference (restricted)
```

---

## Rotation Schedule

### Recommended Schedule

```
┌────────────────────────────────────────────────────────────┐
│                   Credential Rotation Calendar             │
├────────────────────────────────────────────────────────────┤
│ Q1 (Jan-Mar): PostgreSQL passwords, Redis password         │
│ Q2 (Apr-Jun): JWT secret rotation                          │
│ Q3 (Jul-Sep): PostgreSQL passwords, Redis password         │
│ Q4 (Oct-Dec): Annual security review + API key audit       │
└────────────────────────────────────────────────────────────┘
```

---

## PostgreSQL Credential Rotation

### Generating New Credentials

Use the credential generation script to create new strong passwords:

```bash
# Generate all new database credentials
./scripts/secrets/generate-db-credentials.sh --force

# Or for specific output directory
./scripts/secrets/generate-db-credentials.sh --output-dir /path/to/secrets --force

# Dry run to preview passwords without saving
./scripts/secrets/generate-db-credentials.sh --dry-run
```

The script generates:
- **48-character passwords** for database users (URL-safe)
- **32-character password** for PgBouncer auth user
- All passwords include mixed case letters, numbers, and safe special characters

### Method 1: Zero-Downtime Rolling Rotation (Recommended)

This method rotates credentials without service interruption.

#### Step 1: Generate New Passwords

```bash
# Generate new credentials
cd /path/to/tragge
./scripts/secrets/generate-db-credentials.sh --output-dir infra/docker/secrets --force

# Store old credentials for rollback
cp infra/docker/secrets/.db_credentials infra/docker/secrets/.db_credentials.backup
```

#### Step 2: Update PostgreSQL Users

```sql
-- Connect as admin user
psql -U tragge_admin -d app

-- Rotate application user password
ALTER USER tragge_app WITH PASSWORD 'NEW_APP_PASSWORD_FROM_FILE';

-- Rotate readonly user password
ALTER USER tragge_readonly WITH PASSWORD 'NEW_READONLY_PASSWORD_FROM_FILE';

-- Rotate PgBouncer auth user password
ALTER USER pgbouncer_auth WITH PASSWORD 'NEW_PGBOUNCER_PASSWORD_FROM_FILE';

-- Verify users can connect
\c app tragge_app
SELECT current_user;
\c app tragge_readonly
SELECT current_user;
```

#### Step 3: Update Services (Rolling)

**Docker Compose:**

```bash
cd infra/docker

# Rolling restart services one by one
for service in user-bff admin-bff trade-bff; do
    echo "Restarting $service..."
    docker compose up -d --no-deps $service
    sleep 15
    # Verify service health
    curl -sf http://localhost:808X/healthz && echo " ✓ $service healthy"
done
```

**Kubernetes:**

```bash
# Update secret with new credentials
kubectl create secret generic postgres-secrets \
    --from-file=POSTGRES_APP_PASSWORD=secrets/postgres_app_password.txt \
    --from-file=POSTGRES_READONLY_PASSWORD=secrets/postgres_readonly_password.txt \
    --from-file=PGBOUNCER_AUTH_PASSWORD=secrets/pgbouncer_auth_password.txt \
    --dry-run=client -o yaml | kubectl apply -f -

# Rolling restart deployments
for deploy in user-bff admin-bff trade-bff trading-engine leaderboard-worker; do
    kubectl rollout restart deployment/$deploy -n tragge
    kubectl rollout status deployment/$deploy -n tragge --timeout=120s
done
```

#### Step 4: Verify

```bash
# Test database connectivity with new credentials
PGPASSWORD=$(cat secrets/postgres_app_password.txt) \
    psql -h localhost -U tragge_app -d app -c "SELECT 1 AS connection_test;"

# Check all service health endpoints
for port in 8081 8083 8085; do
    curl -sf http://localhost:$port/healthz && echo " ✓ Port $port healthy"
done

# Check readiness endpoints for database connectivity
for port in 8081 8083 8085; do
    curl -sf http://localhost:$port/readyz && echo " ✓ Port $port database connected"
done
```

### Method 2: Admin User Rotation

For rotating the admin user (used for migrations), schedule during maintenance window:

```bash
#!/bin/bash
# rotate-admin-password.sh

set -e

echo "=== Admin User Password Rotation ==="
echo "This should be done during a maintenance window"

# Generate new admin password
NEW_ADMIN_PASSWORD=$(./scripts/secrets/generate-db-credentials.sh --dry-run 2>&1 | \
    grep POSTGRES_ADMIN_PASSWORD | cut -d= -f2)

# Connect as current admin and update password
docker compose exec -T postgres psql -U tragge_admin -d app -c \
    "ALTER USER tragge_admin WITH PASSWORD '$NEW_ADMIN_PASSWORD';"

# Update secret file
echo "$NEW_ADMIN_PASSWORD" > infra/docker/secrets/postgres_admin_password.txt
chmod 600 infra/docker/secrets/postgres_admin_password.txt

echo "=== Admin Password Rotated ==="
echo "Update migration scripts and CI/CD with new credentials"
```

### Method 3: Full Rotation Script

Complete rotation of all database credentials:

```bash
#!/bin/bash
# rotate-all-db-credentials.sh

set -e

SECRETS_DIR="${SECRETS_DIR:-infra/docker/secrets}"

echo "=============================================="
echo "    Full Database Credential Rotation"
echo "=============================================="

# 1. Generate new credentials
echo "Step 1: Generating new credentials..."
./scripts/secrets/generate-db-credentials.sh --output-dir "$SECRETS_DIR" --force

# 2. Read new passwords
ADMIN_PASS=$(cat "$SECRETS_DIR/postgres_admin_password.txt")
APP_PASS=$(cat "$SECRETS_DIR/postgres_app_password.txt")
READONLY_PASS=$(cat "$SECRETS_DIR/postgres_readonly_password.txt")
PGBOUNCER_PASS=$(cat "$SECRETS_DIR/pgbouncer_auth_password.txt")

# 3. Update PostgreSQL users
echo "Step 2: Updating PostgreSQL users..."
docker compose exec -T postgres psql -U tragge_admin -d app <<EOSQL
ALTER USER tragge_app WITH PASSWORD '$APP_PASS';
ALTER USER tragge_readonly WITH PASSWORD '$READONLY_PASS';
ALTER USER pgbouncer_auth WITH PASSWORD '$PGBOUNCER_PASS';
-- Admin password updated last
ALTER USER tragge_admin WITH PASSWORD '$ADMIN_PASS';
EOSQL

# 4. Restart services
echo "Step 3: Restarting services..."
docker compose up -d --force-recreate user-bff admin-bff trade-bff

# 5. Wait and verify
echo "Step 4: Waiting for services to stabilize..."
sleep 30

echo "Step 5: Verifying service health..."
for port in 8081 8083 8085; do
    if curl -sf http://localhost:$port/readyz > /dev/null; then
        echo " ✓ Port $port healthy and connected to database"
    else
        echo " ✗ Port $port FAILED health check"
        exit 1
    fi
done

echo ""
echo "=============================================="
echo "    Credential Rotation Complete"
echo "=============================================="
```

---

## JWT Secret Rotation

JWT secret rotation requires careful handling to avoid invalidating active sessions.

### Dual-Key Rotation Strategy

#### Step 1: Add New Secret

```bash
# Generate new JWT secret
NEW_JWT_SECRET=$(openssl rand -base64 32)

# Update environment to support both old and new secrets
# Services should be configured to accept both during transition
export JWT_SECRET_NEW="$NEW_JWT_SECRET"
```

#### Step 2: Deploy with Dual Verification

Update services to verify tokens against both old and new secrets:

```go
// Example: dual-key verification logic
func verifyToken(token string) bool {
    // Try new secret first
    if valid := verify(token, os.Getenv("JWT_SECRET_NEW")); valid {
        return true
    }
    // Fall back to old secret
    return verify(token, os.Getenv("JWT_SECRET"))
}
```

#### Step 3: Wait for Token Expiry

Wait for the access token TTL (typically 15-60 minutes) to ensure all old tokens expire.

#### Step 4: Remove Old Secret

```bash
# Replace old secret with new
export JWT_SECRET="$NEW_JWT_SECRET"
unset JWT_SECRET_NEW

# Restart services
docker compose restart user-bff admin-bff trade-bff
```

---

## API Key Rotation

### TwelveData / Massive API Keys

```bash
# Update .env with new API keys
TWELVEDATA_API_KEYS=new-key-1,new-key-2
MASSIVE_API_KEYS=new-key-1,new-key-2

# Restart market ingestor
docker compose restart market-ingestor

# Verify market data flow
curl http://localhost:8084/healthz
```

---

## Emergency Rotation

Use this procedure if credentials are compromised.

### Immediate Actions

1. **Isolate the breach**
   ```bash
   # Block external access if needed
   docker compose stop gateway
   ```

2. **Rotate all affected credentials immediately**
   ```bash
   # Generate all new credentials
   NEW_PG_PASSWORD=$(openssl rand -base64 32 | tr -d '=+/')
   NEW_JWT_SECRET=$(openssl rand -base64 32)
   NEW_REDIS_PASSWORD=$(openssl rand -base64 32 | tr -d '=+/')
   ```

3. **Update PostgreSQL**
   ```sql
   -- Terminate all existing connections
   SELECT pg_terminate_backend(pid) FROM pg_stat_activity
   WHERE usename = 'app' AND pid <> pg_backend_pid();

   -- Change password immediately
   ALTER USER app WITH PASSWORD 'NEW_PASSWORD';
   ```

4. **Update all services**
   ```bash
   # Update .env
   cat > .env.emergency << EOF
   POSTGRES_PASSWORD=$NEW_PG_PASSWORD
   JWT_SECRET=$NEW_JWT_SECRET
   REDIS_PASSWORD=$NEW_REDIS_PASSWORD
   EOF

   # Force restart all services
   docker compose down
   docker compose up -d
   ```

5. **Verify and monitor**
   ```bash
   # Check for unauthorized access attempts
   docker compose logs postgres | grep -i "authentication failed"
   ```

6. **Document the incident**
   - Record timeline of events
   - List affected credentials
   - Document remediation steps taken

---

## Automation

### Using External Secrets Operator (Kubernetes)

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: postgres-credentials
  namespace: tragge
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secrets-manager
    kind: SecretStore
  target:
    name: postgres-secrets
  data:
    - secretKey: POSTGRES_PASSWORD
      remoteRef:
        key: tragge/postgres
        property: password
```

### Automated Rotation with AWS Secrets Manager

```bash
# Enable automatic rotation
aws secretsmanager rotate-secret \
    --secret-id tragge/postgres \
    --rotation-lambda-arn arn:aws:lambda:REGION:ACCOUNT:function:SecretsManagerRotation \
    --rotation-rules AutomaticallyAfterDays=90
```

### CronJob for Rotation Reminders

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: credential-rotation-reminder
  namespace: tragge
spec:
  schedule: "0 9 1 */3 *"  # First day of each quarter at 9 AM
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
              curl -X POST "$SLACK_WEBHOOK_URL" \
                -H 'Content-Type: application/json' \
                -d '{"text":"🔐 Reminder: Quarterly credential rotation is due for tragge platform"}'
          restartPolicy: Never
```

---

## Checklist

### Pre-Rotation

- [ ] Scheduled maintenance window
- [ ] Notified stakeholders
- [ ] Tested procedure in staging
- [ ] Backup current credentials securely
- [ ] Prepared rollback plan

### Post-Rotation

- [ ] All services healthy
- [ ] Database connectivity verified
- [ ] Application functionality tested
- [ ] Old credentials securely deleted
- [ ] Documentation updated
- [ ] Rotation logged in audit trail

---

*Last updated: January 2026*
