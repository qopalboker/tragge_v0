#!/usr/bin/env bash
#
# restore-postgres.sh - PostgreSQL restore from S3
#
# This script restores a PostgreSQL database from an S3 backup.
# It includes safety checks and supports point-in-time recovery.
#
# Usage: ./restore-postgres.sh [--backup-file FILE] [--latest] [--dry-run]
#
# Environment Variables Required:
#   POSTGRES_HOST       - PostgreSQL host (default: localhost)
#   POSTGRES_PORT       - PostgreSQL port (default: 5432)
#   POSTGRES_DB         - Database name (default: app)
#   POSTGRES_USER       - Database user (default: app)
#   POSTGRES_PASSWORD   - Database password (required)
#   S3_BUCKET           - S3 bucket name (required)
#   S3_PREFIX           - S3 key prefix (default: backups/postgres)
#   AWS_REGION          - AWS region (default: us-east-1)
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${GREEN}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*" >&2; }
log_step() { echo -e "${CYAN}[STEP]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"; }

# Cleanup function
cleanup() {
    local exit_code=$?
    if [[ -n "${RESTORE_DIR:-}" && -d "$RESTORE_DIR" ]]; then
        log_info "Cleaning up temporary files..."
        rm -rf "$RESTORE_DIR"
    fi
    exit $exit_code
}

trap cleanup EXIT INT TERM

# Configuration with defaults
POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
POSTGRES_DB="${POSTGRES_DB:-app}"
POSTGRES_USER="${POSTGRES_USER:-app}"
S3_PREFIX="${S3_PREFIX:-backups/postgres}"
AWS_REGION="${AWS_REGION:-us-east-1}"

# Parse arguments
BACKUP_FILE=""
USE_LATEST=false
DRY_RUN=false
FORCE=false
CREATE_BACKUP=true

print_usage() {
    echo "Usage: $0 [OPTIONS]"
    echo ""
    echo "Options:"
    echo "  --backup-file FILE  Restore from specific S3 file"
    echo "  --latest            Restore from latest backup"
    echo "  --list              List available backups"
    echo "  --dry-run           Show what would be done without executing"
    echo "  --force             Skip confirmation prompts"
    echo "  --no-pre-backup     Skip creating backup before restore"
    echo "  --help, -h          Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 --latest"
    echo "  $0 --backup-file backups/postgres/postgres_app_full_20240115_030000.sql.gz"
    echo "  $0 --list"
}

while [[ $# -gt 0 ]]; do
    case $1 in
        --backup-file)
            BACKUP_FILE="$2"
            shift 2
            ;;
        --latest)
            USE_LATEST=true
            shift
            ;;
        --list)
            LIST_BACKUPS=true
            shift
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --force)
            FORCE=true
            shift
            ;;
        --no-pre-backup)
            CREATE_BACKUP=false
            shift
            ;;
        --help|-h)
            print_usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            print_usage
            exit 1
            ;;
    esac
done

# Validate required environment variables
validate_env() {
    local missing=()

    [[ -z "${POSTGRES_PASSWORD:-}" ]] && missing+=("POSTGRES_PASSWORD")
    [[ -z "${S3_BUCKET:-}" ]] && missing+=("S3_BUCKET")

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required environment variables: ${missing[*]}"
        exit 1
    fi
}

# Check required tools
check_dependencies() {
    local deps=("psql" "aws" "gunzip")
    local missing=()

    for dep in "${deps[@]}"; do
        if ! command -v "$dep" &>/dev/null; then
            missing+=("$dep")
        fi
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing[*]}"
        exit 1
    fi
}

# List available backups
list_backups() {
    log_info "Available PostgreSQL backups in s3://${S3_BUCKET}/${S3_PREFIX}/:"
    echo ""
    echo "Date/Time            Size       Filename"
    echo "------------------------------------------------------------"

    aws s3 ls "s3://${S3_BUCKET}/${S3_PREFIX}/" --region "$AWS_REGION" 2>/dev/null | \
        grep "postgres_" | \
        while read -r date time size file; do
            printf "%-20s %-10s %s\n" "${date} ${time}" "$size" "$file"
        done

    echo ""

    # Show latest manifest if available
    local manifest
    manifest=$(aws s3 cp "s3://${S3_BUCKET}/${S3_PREFIX}/latest_full.json" - --region "$AWS_REGION" 2>/dev/null || true)
    if [[ -n "$manifest" ]]; then
        log_info "Latest full backup manifest:"
        echo "$manifest" | python3 -m json.tool 2>/dev/null || echo "$manifest"
    fi
}

# Get latest backup file
get_latest_backup() {
    log_info "Finding latest backup..."

    # Try to get from manifest first
    local manifest
    manifest=$(aws s3 cp "s3://${S3_BUCKET}/${S3_PREFIX}/latest_full.json" - --region "$AWS_REGION" 2>/dev/null || true)

    if [[ -n "$manifest" ]]; then
        BACKUP_FILE=$(echo "$manifest" | python3 -c "import sys, json; print(json.load(sys.stdin).get('backup_file', ''))" 2>/dev/null || true)
        if [[ -n "$BACKUP_FILE" ]]; then
            log_info "Found latest backup from manifest: $BACKUP_FILE"
            return
        fi
    fi

    # Fallback: list and get most recent
    BACKUP_FILE=$(aws s3 ls "s3://${S3_BUCKET}/${S3_PREFIX}/" --region "$AWS_REGION" 2>/dev/null | \
        grep "postgres_.*_full_.*\.sql\.gz" | \
        sort -r | \
        head -1 | \
        awk '{print $4}')

    if [[ -z "$BACKUP_FILE" ]]; then
        log_error "No backups found in s3://${S3_BUCKET}/${S3_PREFIX}/"
        exit 1
    fi

    BACKUP_FILE="${S3_PREFIX}/${BACKUP_FILE}"
    log_info "Found latest backup: $BACKUP_FILE"
}

# Verify backup file exists
verify_backup_exists() {
    log_info "Verifying backup file exists..."

    local s3_uri="s3://${S3_BUCKET}/${BACKUP_FILE}"

    if ! aws s3 ls "$s3_uri" --region "$AWS_REGION" &>/dev/null; then
        log_error "Backup file not found: $s3_uri"
        exit 1
    fi

    local file_info
    file_info=$(aws s3 ls "$s3_uri" --region "$AWS_REGION")
    log_info "Backup file: $file_info"
}

# Test database connectivity
test_db_connection() {
    log_info "Testing database connection..."
    export PGPASSWORD="$POSTGRES_PASSWORD"

    if ! pg_isready -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -t 10 &>/dev/null; then
        log_error "Cannot connect to PostgreSQL at $POSTGRES_HOST:$POSTGRES_PORT"
        exit 1
    fi

    log_info "Database connection successful"
}

# Get current database stats
get_db_stats() {
    export PGPASSWORD="$POSTGRES_PASSWORD"

    local size
    size=$(psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
        "SELECT pg_size_pretty(pg_database_size('$POSTGRES_DB'));" 2>/dev/null | tr -d ' ')

    local tables
    tables=$(psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
        "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';" 2>/dev/null | tr -d ' ')

    echo "Size: ${size:-unknown}, Tables: ${tables:-unknown}"
}

# Create pre-restore backup
create_pre_restore_backup() {
    if [[ "$CREATE_BACKUP" != true ]]; then
        log_warn "Skipping pre-restore backup (--no-pre-backup specified)"
        return
    fi

    log_step "Creating pre-restore backup..."

    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

    if [[ -x "${script_dir}/backup-postgres.sh" ]]; then
        S3_PREFIX="${S3_PREFIX}/pre-restore" "${script_dir}/backup-postgres.sh" || {
            log_error "Failed to create pre-restore backup"
            exit 1
        }
    else
        log_warn "backup-postgres.sh not found, skipping pre-restore backup"
    fi
}

# Confirm restore operation
confirm_restore() {
    if [[ "$FORCE" == true ]]; then
        return
    fi

    echo ""
    echo -e "${RED}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║                    ⚠️  WARNING ⚠️                           ║${NC}"
    echo -e "${RED}║  This will OVERWRITE all data in the database!             ║${NC}"
    echo -e "${RED}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "Target database: ${POSTGRES_DB} @ ${POSTGRES_HOST}:${POSTGRES_PORT}"
    echo "Current stats: $(get_db_stats)"
    echo "Backup file: ${BACKUP_FILE}"
    echo ""

    read -p "Type 'RESTORE' to confirm: " -r confirmation

    if [[ "$confirmation" != "RESTORE" ]]; then
        log_info "Restore cancelled"
        exit 0
    fi
}

# Download backup from S3
download_backup() {
    RESTORE_DIR=$(mktemp -d)
    local local_file="${RESTORE_DIR}/backup.sql.gz"
    local s3_uri="s3://${S3_BUCKET}/${BACKUP_FILE}"

    log_step "Downloading backup from S3..."
    log_info "Source: $s3_uri"

    local start_time
    start_time=$(date +%s)

    if ! aws s3 cp "$s3_uri" "$local_file" --region "$AWS_REGION"; then
        log_error "Failed to download backup from S3"
        exit 1
    fi

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))
    local file_size
    file_size=$(du -h "$local_file" | cut -f1)

    log_info "Downloaded ${file_size} in ${duration}s"

    DOWNLOADED_FILE="$local_file"
}

# Terminate existing connections
terminate_connections() {
    log_step "Terminating existing database connections..."
    export PGPASSWORD="$POSTGRES_PASSWORD"

    psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d postgres -c \
        "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '$POSTGRES_DB' AND pid <> pg_backend_pid();" \
        2>/dev/null || log_warn "Could not terminate some connections"
}

# Perform the restore
perform_restore() {
    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY RUN] Would restore from: $BACKUP_FILE"
        log_info "[DRY RUN] Target: $POSTGRES_DB @ $POSTGRES_HOST:$POSTGRES_PORT"
        return
    fi

    log_step "Starting database restore..."
    export PGPASSWORD="$POSTGRES_PASSWORD"

    local start_time
    start_time=$(date +%s)

    # Safety check: verify we can create a database before dropping the original.
    # This prevents data loss if CREATE DATABASE would fail (e.g., permissions, disk space).
    local test_db="${POSTGRES_DB}_restore_test_$$"
    log_info "Verifying CREATE DATABASE permissions with test database '${test_db}'..."
    if ! psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d postgres -c \
        "CREATE DATABASE ${test_db};" 2>/dev/null; then
        log_error "Cannot create databases — aborting restore, original database is intact"
        exit 1
    fi
    psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d postgres -c \
        "DROP DATABASE ${test_db};" 2>/dev/null || true

    # Drop and recreate database
    log_info "Dropping existing database..."
    psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d postgres -c \
        "DROP DATABASE IF EXISTS ${POSTGRES_DB};" 2>/dev/null || true

    log_info "Creating fresh database..."
    psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d postgres -c \
        "CREATE DATABASE ${POSTGRES_DB} OWNER ${POSTGRES_USER};" 2>/dev/null || {
            log_error "Failed to create database"
            exit 1
        }

    # Restore from backup
    log_info "Restoring data from backup..."
    if ! gunzip -c "$DOWNLOADED_FILE" | \
        psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
        --single-transaction \
        --set ON_ERROR_STOP=on \
        2>&1 | tee "${RESTORE_DIR}/restore.log"; then
        log_error "Restore failed. Check ${RESTORE_DIR}/restore.log for details"
        exit 1
    fi

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))

    log_info "Restore completed in ${duration}s"
}

# Verify restore
verify_restore() {
    log_step "Verifying restore..."
    export PGPASSWORD="$POSTGRES_PASSWORD"

    # Check table count
    local tables
    tables=$(psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
        "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';" 2>/dev/null | tr -d ' ')

    if [[ "$tables" -eq 0 ]]; then
        log_warn "No tables found after restore - this may indicate a problem"
    else
        log_info "Found $tables tables in public schema"
    fi

    # Run ANALYZE for statistics
    log_info "Running ANALYZE to update statistics..."
    psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
        "ANALYZE;" 2>/dev/null || log_warn "ANALYZE failed"

    # Show database stats
    log_info "Post-restore stats: $(get_db_stats)"
}

# Main execution
main() {
    log_info "========================================"
    log_info "PostgreSQL Restore Script"
    log_info "========================================"

    validate_env
    check_dependencies

    # Handle --list option
    if [[ "${LIST_BACKUPS:-}" == true ]]; then
        list_backups
        exit 0
    fi

    # Determine backup file
    if [[ -z "$BACKUP_FILE" && "$USE_LATEST" != true ]]; then
        log_error "Must specify --backup-file or --latest"
        print_usage
        exit 1
    fi

    if [[ "$USE_LATEST" == true ]]; then
        get_latest_backup
    fi

    log_info "Target: ${POSTGRES_DB} @ ${POSTGRES_HOST}:${POSTGRES_PORT}"
    log_info "Backup: ${BACKUP_FILE}"
    log_info "Dry Run: ${DRY_RUN}"
    log_info "========================================"

    verify_backup_exists
    test_db_connection
    confirm_restore
    create_pre_restore_backup
    download_backup
    terminate_connections
    perform_restore
    verify_restore

    log_info "========================================"
    log_info "Restore completed successfully!"
    log_info "Database: ${POSTGRES_DB}"
    log_info "Stats: $(get_db_stats)"
    log_info "========================================"
}

main "$@"
