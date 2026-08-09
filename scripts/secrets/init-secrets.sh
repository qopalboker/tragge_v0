#!/bin/bash
# ==============================================================================
# Initialize Docker Secrets Directory
# ==============================================================================
# This script creates the Docker secrets directory and generates secure
# random values for development use.
#
# Usage:
#   ./scripts/secrets/init-secrets.sh [--force]
#
# Options:
#   --force    Overwrite existing secret files
#
# ==============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SECRETS_DIR="$PROJECT_ROOT/infra/docker/secrets"

FORCE=false
if [[ "$1" == "--force" ]]; then
    FORCE=true
fi

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "=============================================="
echo "    Docker Secrets Initialization"
echo "=============================================="
echo ""

# Create secrets directory if it doesn't exist
mkdir -p "$SECRETS_DIR"

# Function to generate a random password
generate_password() {
    openssl rand -base64 32 | tr -d '=+/' | head -c 32
}

# Auth keys contain at least 256 bits of unpredictable source material.
generate_auth_secret() {
    openssl rand -base64 48 | tr -d '\n'
}

# Function to create a secret file
create_secret() {
    local filename="$1"
    local default_value="$2"
    local description="$3"
    local filepath="$SECRETS_DIR/$filename"

    if [[ -f "$filepath" && "$FORCE" != "true" ]]; then
        echo -e "${YELLOW}[SKIP]${NC} $filename (already exists)"
        return 0
    fi

    echo "$default_value" > "$filepath"
    chmod 600 "$filepath"
    echo -e "${GREEN}[CREATE]${NC} $filename - $description"
}

# Create secrets with generated values for development
echo "Creating secret files..."
echo ""

# PostgreSQL password (generate random)
create_secret "postgres_password.txt" "$(generate_password)" "PostgreSQL app user password"

# Redis password (generate random)
create_secret "redis_password.txt" "$(generate_password)" "Redis authentication password"

# Grafana admin password (generate random)
create_secret "grafana_admin_password.txt" "$(generate_password)" "Grafana admin password"

# JWT secret (generate random) — legacy combined secret, kept for services
# that still read JWT_SECRET directly (non-panel internal paths and older tools).
create_secret "jwt_secret.txt" "$(generate_password)" "JWT signing secret"

# User/Admin access and refresh signing contexts. All four values MUST differ.
create_secret "jwt_secret_user.txt" "$(generate_auth_secret)" "User access-token signing secret"
create_secret "jwt_refresh_secret_user.txt" "$(generate_auth_secret)" "User refresh-token signing secret"
create_secret "jwt_secret_admin.txt" "$(generate_auth_secret)" "Admin access-token signing secret"
create_secret "jwt_refresh_secret_admin.txt" "$(generate_auth_secret)" "Admin refresh-token signing secret"

# TwelveData API keys (placeholder - user must provide)
create_secret "twelvedata_api_keys.txt" "your-twelvedata-api-key" "TwelveData API key(s)"

# Massive API keys (placeholder - user must provide)
create_secret "massive_api_keys.txt" "your-massive-api-key" "Massive API key(s)"

# Dedicated security-code HMAC key (independent of JWT and provider keys)
create_secret "security_code_hash_secret.txt" "$(generate_auth_secret)" "Security-code HMAC key (local development)"

# Country-routed security email providers (both required for production)
create_secret "mailerino_api_key.txt" "" "Mailerino security email API key"
create_secret "resend_api_key.txt" "" "Resend security email API key"

# KaveNegar is required when SMS security-code delivery is enabled.
create_secret "kavenegar_api_key.txt" "" "KaveNegar SMS API key"

# Discord webhook URL (placeholder - user must provide)
create_secret "discord_webhook_url.txt" "" "Discord webhook URL (optional)"

# MinIO/S3 Storage (default dev credentials)
create_secret "minio_access_key.txt" "minioadmin" "MinIO access key"
create_secret "minio_secret_key.txt" "minioadmin" "MinIO secret key"

# TOTP encryption key (32 bytes = 64 hex chars, for AES-256-GCM encryption of 2FA secrets)
create_secret "totp_encryption_key.txt" "$(openssl rand -hex 32)" "TOTP encryption key (AES-256)"
create_secret "admin_mfa_encryption_key.txt" "$(openssl rand -hex 32)" "Admin-only MFA encryption key (AES-256-GCM)"
create_secret "admin_mfa_recovery_pepper.txt" "$(openssl rand -hex 32)" "Admin-only MFA recovery-code HMAC pepper"

# Jibit PPG payment gateway (placeholder - user must provide)
create_secret "jibit_api_key.txt" "" "Jibit PPG API key"
create_secret "jibit_secret_key.txt" "" "Jibit PPG secret key"
create_secret "jibit_kyc_api_key.txt" "" "Jibit KYC API key (optional)"
create_secret "jibit_kyc_secret_key.txt" "" "Jibit KYC secret key (optional)"

# NowPayments crypto gateway (placeholder - user must provide)
create_secret "nowpayments_api_key.txt" "" "NowPayments API key"
create_secret "nowpayments_ipn_secret.txt" "" "NowPayments IPN secret (optional)"

# Generate role-based database credentials (admin, app, readonly, pgbouncer)
# These are required by docker-compose.yml
echo ""
echo "Generating role-based database credentials..."
GENERATE_ARGS=""
if [[ "$FORCE" == "true" ]]; then
    GENERATE_ARGS="--force"
fi
"$SCRIPT_DIR/generate-db-credentials.sh" $GENERATE_ARGS

echo ""
echo "=============================================="
echo "    Setup Complete"
echo "=============================================="
echo ""
echo "Secret files created in: $SECRETS_DIR"
echo ""
echo -e "${YELLOW}IMPORTANT:${NC} Update the following files with your actual API keys:"
echo "  - twelvedata_api_keys.txt"
echo "  - massive_api_keys.txt"
echo ""
echo "Payment gateways (if using):"
echo "  - jibit_api_key.txt / jibit_secret_key.txt"
echo "  - nowpayments_api_key.txt"
echo ""
echo "Security-code delivery (required before production startup):"
echo "  - mailerino_api_key.txt"
echo "  - resend_api_key.txt"
echo "  - kavenegar_api_key.txt (when SMS_ENABLED=true)"
echo "  - security_code_hash_secret.txt is generated locally; provision independently in production"
echo ""
echo "Optional notifications:"
echo "  - discord_webhook_url.txt"
echo ""
echo "To verify credentials consistency:"
echo "  ./scripts/secrets/verify-db-credentials.sh"
echo ""
echo "To start services:"
echo "  cd infra/docker && docker compose up -d"
echo ""
