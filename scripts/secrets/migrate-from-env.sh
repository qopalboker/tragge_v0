#!/bin/bash
# ==============================================================================
# Migrate Secrets from .env to Docker Secrets
# ==============================================================================
# This script migrates sensitive values from a .env file to Docker secrets.
# It extracts the values and creates the appropriate secret files.
#
# Usage:
#   ./scripts/secrets/migrate-from-env.sh [path/to/.env]
#
# If no path is provided, it looks for .env in the project root.
#
# ==============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SECRETS_DIR="$PROJECT_ROOT/infra/docker/secrets"

# Default .env path
ENV_FILE="${1:-$PROJECT_ROOT/.env}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "=============================================="
echo "    Migrate .env to Docker Secrets"
echo "=============================================="
echo ""

# Check if .env file exists
if [[ ! -f "$ENV_FILE" ]]; then
    echo -e "${RED}Error:${NC} .env file not found at: $ENV_FILE"
    echo ""
    echo "Usage: $0 [path/to/.env]"
    exit 1
fi

echo -e "Source file: ${BLUE}$ENV_FILE${NC}"
echo -e "Target directory: ${BLUE}$SECRETS_DIR${NC}"
echo ""

# Create secrets directory
mkdir -p "$SECRETS_DIR"

# Function to extract value from .env file
get_env_value() {
    local key="$1"
    local value=$(grep "^${key}=" "$ENV_FILE" 2>/dev/null | cut -d'=' -f2- | tr -d '"' | tr -d "'")
    echo "$value"
}

# Function to create secret file from env value
migrate_secret() {
    local env_key="$1"
    local secret_file="$2"
    local description="$3"
    local filepath="$SECRETS_DIR/$secret_file"

    local value=$(get_env_value "$env_key")

    if [[ -z "$value" ]]; then
        echo -e "${YELLOW}[SKIP]${NC} $env_key not found in .env"
        return 0
    fi

    # Check if file exists and has content
    if [[ -f "$filepath" ]]; then
        local existing=$(cat "$filepath" 2>/dev/null | tr -d '\n')
        if [[ "$existing" == "$value" ]]; then
            echo -e "${BLUE}[SAME]${NC} $env_key -> $secret_file (value unchanged)"
            return 0
        fi
        echo -e "${YELLOW}[UPDATE]${NC} $env_key -> $secret_file"
    else
        echo -e "${GREEN}[CREATE]${NC} $env_key -> $secret_file"
    fi

    echo "$value" > "$filepath"
    chmod 600 "$filepath"
}

echo "Migrating secrets..."
echo ""

# Migrate each secret
migrate_secret "POSTGRES_PASSWORD" "postgres_password.txt" "PostgreSQL password"
migrate_secret "JWT_SECRET" "jwt_secret.txt" "JWT signing secret"
migrate_secret "TWELVEDATA_API_KEYS" "twelvedata_api_keys.txt" "TwelveData API keys"
migrate_secret "MASSIVE_API_KEYS" "massive_api_keys.txt" "Massive API keys"
migrate_secret "SECURITY_CODE_HASH_SECRET" "security_code_hash_secret.txt" "Security-code HMAC key"
migrate_secret "MAILERINO_API_KEY" "mailerino_api_key.txt" "Mailerino security email API key"
migrate_secret "RESEND_API_KEY" "resend_api_key.txt" "Resend security email API key"
migrate_secret "KAVENEGAR_API_KEY" "kavenegar_api_key.txt" "KaveNegar SMS API key"
migrate_secret "DISCORD_WEBHOOK_URL" "discord_webhook_url.txt" "Discord webhook URL"

echo ""
echo "=============================================="
echo "    Migration Complete"
echo "=============================================="
echo ""
echo -e "${GREEN}Secrets have been migrated to Docker secrets format.${NC}"
echo ""
echo "Next steps:"
echo ""
echo "1. Update your .env file to remove sensitive values:"
echo "   - POSTGRES_PASSWORD"
echo "   - JWT_SECRET"
echo "   - TWELVEDATA_API_KEYS"
echo "   - MASSIVE_API_KEYS"
echo "   - SECURITY_CODE_HASH_SECRET"
echo "   - MAILERINO_API_KEY"
echo "   - RESEND_API_KEY"
echo "   - KAVENEGAR_API_KEY"
echo "   - DISCORD_WEBHOOK_URL"
echo ""
echo "2. Restart services to use the new secrets:"
echo "   cd infra/docker && docker compose down && docker compose up -d"
echo ""
echo "3. Verify services are healthy:"
echo "   docker compose ps"
echo "   curl http://localhost:8084/readyz  # market-ingestor"
echo ""
echo -e "${YELLOW}Security note:${NC} The secrets directory is in .gitignore."
echo "Never commit actual secret values to version control."
echo ""
