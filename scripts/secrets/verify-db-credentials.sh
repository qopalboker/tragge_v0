#!/bin/bash
# ==============================================================================
# Verify Database Credentials Consistency
# ==============================================================================
# This script checks that individual password files match the combined
# .db_credentials reference file, and validates password strength.
#
# Usage:
#   ./scripts/secrets/verify-db-credentials.sh [--fix] [--quiet]
#
# Options:
#   --fix       Regenerate all credentials if issues are found
#   --quiet     Suppress output, only set exit code
#
# Exit codes:
#   0  All checks passed
#   1  Issues found (mismatch or weak passwords)
#   2  Missing files (secrets directory or .db_credentials not found)
#
# ==============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
SECRETS_DIR="$PROJECT_ROOT/infra/docker/secrets"
CREDENTIALS_FILE="$SECRETS_DIR/.db_credentials"

FIX=false
QUIET=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --fix)
            FIX=true
            shift
            ;;
        --quiet)
            QUIET=true
            shift
            ;;
        -h|--help)
            head -20 "$0" | tail -16
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

ISSUES=0

log() {
    if [[ "$QUIET" != "true" ]]; then
        echo -e "$@"
    fi
}

log_error() {
    ISSUES=$((ISSUES + 1))
    log "${RED}[FAIL]${NC} $1"
}

log_ok() {
    log "${GREEN}[ OK ]${NC} $1"
}

log_warn() {
    ISSUES=$((ISSUES + 1))
    log "${YELLOW}[WARN]${NC} $1"
}

log "=============================================="
log "    Database Credentials Verification"
log "=============================================="
log ""

# --- Check secrets directory exists ---
if [[ ! -d "$SECRETS_DIR" ]]; then
    log_error "Secrets directory not found: $SECRETS_DIR"
    log ""
    log "Run: ./scripts/secrets/init-secrets.sh"
    exit 2
fi

# --- Check required password files exist ---
REQUIRED_FILES=(
    "postgres_admin_password.txt"
    "postgres_app_password.txt"
    "postgres_readonly_password.txt"
    "pgbouncer_auth_password.txt"
    "postgres_password.txt"
)

log "Checking required files..."
for file in "${REQUIRED_FILES[@]}"; do
    filepath="$SECRETS_DIR/$file"
    if [[ ! -f "$filepath" ]]; then
        log_error "Missing: $file"
    elif [[ ! -s "$filepath" ]]; then
        log_error "Empty: $file"
    else
        log_ok "Exists: $file"
    fi
done
log ""

# --- Validate password strength ---
# Map of file -> role name for reporting
declare -A FILE_ROLES=(
    ["postgres_admin_password.txt"]="admin"
    ["postgres_app_password.txt"]="app"
    ["postgres_readonly_password.txt"]="readonly"
    ["postgres_replication_password.txt"]="replication"
    ["pgbouncer_auth_password.txt"]="pgbouncer"
)

MIN_PASSWORD_LENGTH=20

check_password_strength() {
    local password="$1"
    local role="$2"
    local issues=0

    # Check length
    if [[ ${#password} -lt $MIN_PASSWORD_LENGTH ]]; then
        log_warn "$role: Password too short (${#password} chars, minimum $MIN_PASSWORD_LENGTH)"
        issues=$((issues + 1))
    fi

    # Check for uppercase
    if ! echo "$password" | grep -q '[A-Z]'; then
        log_warn "$role: Password missing uppercase letters"
        issues=$((issues + 1))
    fi

    # Check for lowercase
    if ! echo "$password" | grep -q '[a-z]'; then
        log_warn "$role: Password missing lowercase letters"
        issues=$((issues + 1))
    fi

    # Check for digits
    if ! echo "$password" | grep -q '[0-9]'; then
        log_warn "$role: Password missing digits"
        issues=$((issues + 1))
    fi

    # Check for common weak patterns
    local lower_password
    lower_password=$(echo "$password" | tr '[:upper:]' '[:lower:]')
    case "$lower_password" in
        *password*|*123456*|*qwerty*|*admin*|*letmein*|*welcome*)
            log_warn "$role: Password contains common weak pattern"
            issues=$((issues + 1))
            ;;
    esac

    return $issues
}

log "Checking password strength..."
for file in "${!FILE_ROLES[@]}"; do
    filepath="$SECRETS_DIR/$file"
    role="${FILE_ROLES[$file]}"
    if [[ -f "$filepath" && -s "$filepath" ]]; then
        password=$(cat "$filepath" | tr -d '\n')
        if check_password_strength "$password" "$role"; then
            log_ok "$role: Password meets strength requirements (${#password} chars)"
        fi
    fi
done
log ""

# --- Check consistency with .db_credentials ---
if [[ ! -f "$CREDENTIALS_FILE" ]]; then
    log "${YELLOW}[SKIP]${NC} .db_credentials not found — cannot check consistency"
    log "       This is OK if credentials were created individually."
    log ""
else
    log "Checking consistency with .db_credentials..."

    # Map of env var name in .db_credentials -> individual file
    declare -A CRED_MAP=(
        ["POSTGRES_ADMIN_PASSWORD"]="postgres_admin_password.txt"
        ["POSTGRES_APP_PASSWORD"]="postgres_app_password.txt"
        ["POSTGRES_READONLY_PASSWORD"]="postgres_readonly_password.txt"
        ["POSTGRES_REPLICATION_PASSWORD"]="postgres_replication_password.txt"
        ["PGBOUNCER_AUTH_PASSWORD"]="pgbouncer_auth_password.txt"
    )

    for var_name in "${!CRED_MAP[@]}"; do
        file="${CRED_MAP[$var_name]}"
        filepath="$SECRETS_DIR/$file"

        # Extract value from .db_credentials
        cred_value=$(grep "^${var_name}=" "$CREDENTIALS_FILE" 2>/dev/null | head -1 | cut -d'=' -f2-)

        if [[ -z "$cred_value" ]]; then
            # Variable not in .db_credentials — skip
            continue
        fi

        if [[ ! -f "$filepath" ]]; then
            log_error "MISMATCH: $var_name exists in .db_credentials but $file is missing"
            continue
        fi

        file_value=$(cat "$filepath" | tr -d '\n')

        if [[ "$cred_value" != "$file_value" ]]; then
            log_error "MISMATCH: $file does not match .db_credentials ($var_name)"
            log "         .db_credentials: ${cred_value:0:4}...${cred_value: -4} (${#cred_value} chars)"
            log "         $file: ${file_value:0:4}...${file_value: -4} (${#file_value} chars)"
        else
            log_ok "Consistent: $file matches .db_credentials"
        fi
    done
    log ""
fi

# --- Summary ---
log "=============================================="
if [[ $ISSUES -eq 0 ]]; then
    log "${GREEN}All checks passed.${NC}"
    log "=============================================="
    exit 0
else
    log "${RED}Found $ISSUES issue(s).${NC}"
    log "=============================================="

    if [[ "$FIX" == "true" ]]; then
        log ""
        log "Regenerating credentials with --force..."
        "$SCRIPT_DIR/generate-db-credentials.sh" --force
        log ""
        log "${GREEN}Credentials regenerated. Please restart services.${NC}"
        exit 0
    else
        log ""
        log "To fix, run one of:"
        log "  $0 --fix"
        log "  ./scripts/secrets/generate-db-credentials.sh --force"
        exit 1
    fi
fi
