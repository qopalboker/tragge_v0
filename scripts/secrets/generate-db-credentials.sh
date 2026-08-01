#!/bin/bash
# ==============================================================================
# Generate Secure Database Credentials
# ==============================================================================
# This script generates strong, unique passwords for PostgreSQL database users
# following security best practices.
#
# Users created:
#   - tragge_admin: Full administrative access for migrations and maintenance
#   - tragge_app: Application user with limited privileges (CRUD on app tables)
#   - tragge_readonly: Read-only access for replica connections and reporting
#   - tragge_replication: Replication user for streaming replication (optional)
#
# Usage:
#   ./scripts/secrets/generate-db-credentials.sh [--output-dir <path>] [--force]
#
# Options:
#   --output-dir    Directory to store credential files (default: infra/docker/secrets)
#   --force         Overwrite existing credential files
#   --dry-run       Print credentials without saving to files
#   --k8s           Also generate Kubernetes secret manifests
#   --verify        Verify existing credentials (consistency and strength)
#
# Security:
#   - Generates 48-character passwords with mixed case, numbers, and symbols
#   - Uses /dev/urandom for cryptographically secure randomness
#   - Sets restrictive file permissions (600)
#   - Excludes easily confused characters (0, O, l, 1, I)
#
# ==============================================================================

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DEFAULT_OUTPUT_DIR="$PROJECT_ROOT/infra/docker/secrets"
OUTPUT_DIR="$DEFAULT_OUTPUT_DIR"
FORCE=false
DRY_RUN=false
GENERATE_K8S=false
VERIFY_ONLY=false

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --force)
            FORCE=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --k8s)
            GENERATE_K8S=true
            shift
            ;;
        --verify)
            VERIFY_ONLY=true
            shift
            ;;
        -h|--help)
            head -35 "$0" | tail -30
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# If --verify mode, delegate to verify script and exit
if [[ "$VERIFY_ONLY" == "true" ]]; then
    exec "$SCRIPT_DIR/verify-db-credentials.sh" "$@"
fi

# Password generation function
# Generates a cryptographically secure password with:
# - 48 characters minimum (exceeds NIST recommendations)
# - Mixed case letters, numbers, and special characters
# - Excludes easily confused characters (0, O, l, 1, I)
generate_password() {
    local length=${1:-48}
    local charset='A-HJ-NP-Za-km-z2-9!@#$%^&*()_+-=[]{}|;:,.<>?'

    # Generate password using /dev/urandom
    local password=""
    while [[ ${#password} -lt $length ]]; do
        local char=$(head -c 100 /dev/urandom | tr -dc "$charset" | head -c 1)
        password="${password}${char}"
    done

    # Ensure password meets complexity requirements
    # Must contain: uppercase, lowercase, number, special char
    while ! (echo "$password" | grep -q '[A-Z]' && \
             echo "$password" | grep -q '[a-z]' && \
             echo "$password" | grep -q '[0-9]' && \
             echo "$password" | grep -q '[!@#$%^&*()_+\-=\[\]{}|;:,.<>?]'); do
        password=""
        while [[ ${#password} -lt $length ]]; do
            local char=$(head -c 100 /dev/urandom | tr -dc "$charset" | head -c 1)
            password="${password}${char}"
        done
    done

    echo "$password"
}

# Generate URL-safe password (for connection strings)
# Avoids characters that need URL encoding
generate_url_safe_password() {
    local length=${1:-48}
    local charset='A-HJ-NP-Za-km-z2-9_-'

    local password=""
    while [[ ${#password} -lt $length ]]; do
        local char=$(head -c 100 /dev/urandom | tr -dc "$charset" | head -c 1)
        password="${password}${char}"
    done

    # Ensure password meets complexity requirements
    while ! (echo "$password" | grep -q '[A-Z]' && \
             echo "$password" | grep -q '[a-z]' && \
             echo "$password" | grep -q '[0-9]'); do
        password=""
        while [[ ${#password} -lt $length ]]; do
            local char=$(head -c 100 /dev/urandom | tr -dc "$charset" | head -c 1)
            password="${password}${char}"
        done
    done

    echo "$password"
}

echo "=============================================="
echo "    Secure Database Credentials Generator"
echo "=============================================="
echo ""

# Create output directory
if [[ "$DRY_RUN" != "true" ]]; then
    mkdir -p "$OUTPUT_DIR"
fi

# Generate credentials for each user
echo -e "${BLUE}Generating secure credentials...${NC}"
echo ""

# Admin user (full access)
ADMIN_PASSWORD=$(generate_url_safe_password 48)
echo -e "${GREEN}[GENERATED]${NC} tragge_admin password (48 chars, URL-safe)"

# Application user (limited access)
APP_PASSWORD=$(generate_url_safe_password 48)
echo -e "${GREEN}[GENERATED]${NC} tragge_app password (48 chars, URL-safe)"

# Read-only user (replica connections)
READONLY_PASSWORD=$(generate_url_safe_password 48)
echo -e "${GREEN}[GENERATED]${NC} tragge_readonly password (48 chars, URL-safe)"

# Replication user (streaming replication)
REPLICATION_PASSWORD=$(generate_url_safe_password 48)
echo -e "${GREEN}[GENERATED]${NC} tragge_replication password (48 chars, URL-safe)"

# PgBouncer auth user
PGBOUNCER_PASSWORD=$(generate_url_safe_password 32)
echo -e "${GREEN}[GENERATED]${NC} pgbouncer_auth password (32 chars, URL-safe)"

echo ""

if [[ "$DRY_RUN" == "true" ]]; then
    echo -e "${YELLOW}=== DRY RUN - Credentials (not saved) ===${NC}"
    echo ""
    echo "POSTGRES_ADMIN_USER=tragge_admin"
    echo "POSTGRES_ADMIN_PASSWORD=$ADMIN_PASSWORD"
    echo ""
    echo "POSTGRES_APP_USER=tragge_app"
    echo "POSTGRES_APP_PASSWORD=$APP_PASSWORD"
    echo ""
    echo "POSTGRES_READONLY_USER=tragge_readonly"
    echo "POSTGRES_READONLY_PASSWORD=$READONLY_PASSWORD"
    echo ""
    echo "POSTGRES_REPLICATION_USER=tragge_replication"
    echo "POSTGRES_REPLICATION_PASSWORD=$REPLICATION_PASSWORD"
    echo ""
    echo "PGBOUNCER_AUTH_USER=pgbouncer_auth"
    echo "PGBOUNCER_AUTH_PASSWORD=$PGBOUNCER_PASSWORD"
    echo ""
    exit 0
fi

# Function to save credential file
save_credential() {
    local filename="$1"
    local value="$2"
    local description="$3"
    local filepath="$OUTPUT_DIR/$filename"

    if [[ -f "$filepath" && "$FORCE" != "true" ]]; then
        echo -e "${YELLOW}[SKIP]${NC} $filename (already exists, use --force to overwrite)"
        return 0
    fi

    echo "$value" > "$filepath"
    chmod 600 "$filepath"
    echo -e "${GREEN}[SAVED]${NC} $filename - $description"
}

echo -e "${BLUE}Saving credentials to $OUTPUT_DIR${NC}"
echo ""

# Save individual credential files
save_credential "postgres_admin_password.txt" "$ADMIN_PASSWORD" "Admin user password"
save_credential "postgres_app_password.txt" "$APP_PASSWORD" "Application user password"
save_credential "postgres_readonly_password.txt" "$READONLY_PASSWORD" "Read-only user password"
save_credential "postgres_replication_password.txt" "$REPLICATION_PASSWORD" "Replication user password"
save_credential "pgbouncer_auth_password.txt" "$PGBOUNCER_PASSWORD" "PgBouncer auth password"

# For backwards compatibility, also update the main postgres_password.txt to use app password
save_credential "postgres_password.txt" "$APP_PASSWORD" "Default password (app user for backwards compat)"

# Create a combined credentials file (for reference only, with restricted permissions)
CREDENTIALS_FILE="$OUTPUT_DIR/.db_credentials"
if [[ ! -f "$CREDENTIALS_FILE" || "$FORCE" == "true" ]]; then
    cat > "$CREDENTIALS_FILE" << EOF
# PostgreSQL Database Credentials
# Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
# WARNING: This file contains sensitive credentials. Keep secure!

# Admin User (for migrations and maintenance)
POSTGRES_ADMIN_USER=tragge_admin
POSTGRES_ADMIN_PASSWORD=$ADMIN_PASSWORD

# Application User (for service connections)
POSTGRES_APP_USER=tragge_app
POSTGRES_APP_PASSWORD=$APP_PASSWORD

# Read-Only User (for replica connections)
POSTGRES_READONLY_USER=tragge_readonly
POSTGRES_READONLY_PASSWORD=$READONLY_PASSWORD

# Replication User (for streaming replication)
POSTGRES_REPLICATION_USER=tragge_replication
POSTGRES_REPLICATION_PASSWORD=$REPLICATION_PASSWORD

# PgBouncer Auth User
PGBOUNCER_AUTH_USER=pgbouncer_auth
PGBOUNCER_AUTH_PASSWORD=$PGBOUNCER_PASSWORD

# Connection String Examples:
# Admin:    postgres://tragge_admin:$ADMIN_PASSWORD@localhost:5432/app?sslmode=require
# App:      postgres://tragge_app:$APP_PASSWORD@localhost:5432/app?sslmode=require
# Readonly: postgres://tragge_readonly:$READONLY_PASSWORD@localhost:5432/app?sslmode=require
EOF
    chmod 600 "$CREDENTIALS_FILE"
    echo -e "${GREEN}[SAVED]${NC} .db_credentials - Combined credentials file (restricted)"
fi

# Generate Kubernetes secrets if requested
if [[ "$GENERATE_K8S" == "true" ]]; then
    echo ""
    echo -e "${BLUE}Generating Kubernetes secret manifests...${NC}"

    K8S_DIR="$PROJECT_ROOT/infra/k8s/base"
    K8S_SECRETS_FILE="$K8S_DIR/postgres-credentials-secret.yaml"

    # Base64 encode credentials
    ADMIN_B64=$(echo -n "$ADMIN_PASSWORD" | base64 -w0)
    APP_B64=$(echo -n "$APP_PASSWORD" | base64 -w0)
    READONLY_B64=$(echo -n "$READONLY_PASSWORD" | base64 -w0)
    REPLICATION_B64=$(echo -n "$REPLICATION_PASSWORD" | base64 -w0)
    PGBOUNCER_B64=$(echo -n "$PGBOUNCER_PASSWORD" | base64 -w0)

    cat > "$K8S_SECRETS_FILE" << EOF
# Generated by scripts/secrets/generate-db-credentials.sh
# DO NOT commit this file with real credentials to version control!
# Use External Secrets Operator or Sealed Secrets in production.
apiVersion: v1
kind: Secret
metadata:
  name: postgres-credentials
  namespace: tragge
  labels:
    app.kubernetes.io/name: postgres
    app.kubernetes.io/part-of: tragge-platform
  annotations:
    description: "PostgreSQL credentials - generated $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
type: Opaque
data:
  # Admin user (for migrations and maintenance)
  POSTGRES_ADMIN_USER: $(echo -n "tragge_admin" | base64 -w0)
  POSTGRES_ADMIN_PASSWORD: $ADMIN_B64

  # Application user (for service connections)
  POSTGRES_APP_USER: $(echo -n "tragge_app" | base64 -w0)
  POSTGRES_APP_PASSWORD: $APP_B64

  # Read-only user (for replica connections)
  POSTGRES_READONLY_USER: $(echo -n "tragge_readonly" | base64 -w0)
  POSTGRES_READONLY_PASSWORD: $READONLY_B64

  # Replication user (for streaming replication)
  POSTGRES_REPLICATION_USER: $(echo -n "tragge_replication" | base64 -w0)
  POSTGRES_REPLICATION_PASSWORD: $REPLICATION_B64

  # PgBouncer auth user
  PGBOUNCER_AUTH_USER: $(echo -n "pgbouncer_auth" | base64 -w0)
  PGBOUNCER_AUTH_PASSWORD: $PGBOUNCER_B64
EOF
    chmod 600 "$K8S_SECRETS_FILE"
    echo -e "${GREEN}[SAVED]${NC} $K8S_SECRETS_FILE"
    echo -e "${YELLOW}WARNING:${NC} Do not commit this file with real credentials!"
fi

echo ""
echo "=============================================="
echo "    Credential Generation Complete"
echo "=============================================="
echo ""
echo "Credentials saved to: $OUTPUT_DIR"
echo ""
echo -e "${YELLOW}IMPORTANT NEXT STEPS:${NC}"
echo ""
echo "1. Initialize PostgreSQL with these users:"
echo "   Run: make migrate-init-users"
echo "   Or manually apply: packages/db/init/01-users.sql"
echo ""
echo "2. Update docker-compose.yml to use new secrets"
echo ""
echo "3. For Kubernetes, either:"
echo "   a. Use --k8s flag to generate secret manifests"
echo "   b. Configure External Secrets Operator"
echo ""
echo "4. Rotate credentials every 90 days"
echo "   See: docs/runbook/credential-rotation.md"
echo ""
echo -e "${RED}SECURITY REMINDER:${NC}"
echo "- Never commit credentials to version control"
echo "- Store credentials in a secure vault"
echo "- Restrict file permissions to 600"
echo ""
