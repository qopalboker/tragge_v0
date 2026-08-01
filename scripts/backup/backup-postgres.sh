#!/usr/bin/env bash
#
# backup-postgres.sh - PostgreSQL backup to S3
#
# This script performs a full PostgreSQL backup using pg_dump and uploads
# it to S3 with proper encryption and retention tagging.
#
# Usage: ./backup-postgres.sh [--full|--schema-only]
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
#   RETENTION_DAYS      - Backup retention in days (default: 30)
#   ENCRYPTION_KEY_ID   - KMS key ID for encryption (optional)
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${GREEN}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*"; }
log_error() { echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $*" >&2; }

# Cleanup function
cleanup() {
    local exit_code=$?
    if [[ -n "${BACKUP_FILE:-}" && -f "$BACKUP_FILE" ]]; then
        log_info "Cleaning up temporary backup file..."
        rm -f "$BACKUP_FILE"
    fi
    if [[ -n "${BACKUP_DIR:-}" && -d "$BACKUP_DIR" ]]; then
        rmdir "$BACKUP_DIR" 2>/dev/null || true
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
RETENTION_DAYS="${RETENTION_DAYS:-30}"

# Parse arguments
BACKUP_TYPE="full"
while [[ $# -gt 0 ]]; do
    case $1 in
        --full)
            BACKUP_TYPE="full"
            shift
            ;;
        --schema-only)
            BACKUP_TYPE="schema"
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [--full|--schema-only]"
            echo ""
            echo "Options:"
            echo "  --full         Full database backup (default)"
            echo "  --schema-only  Schema-only backup (no data)"
            echo ""
            echo "Environment Variables:"
            echo "  POSTGRES_HOST       PostgreSQL host (default: localhost)"
            echo "  POSTGRES_PORT       PostgreSQL port (default: 5432)"
            echo "  POSTGRES_DB         Database name (default: app)"
            echo "  POSTGRES_USER       Database user (default: app)"
            echo "  POSTGRES_PASSWORD   Database password (required)"
            echo "  S3_BUCKET           S3 bucket name (required)"
            echo "  S3_PREFIX           S3 key prefix (default: backups/postgres)"
            echo "  AWS_REGION          AWS region (default: us-east-1)"
            echo "  RETENTION_DAYS      Backup retention in days (default: 30)"
            echo "  ENCRYPTION_KEY_ID   KMS key ID for encryption (optional)"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
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
    local deps=("pg_dump" "aws" "gzip")
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

# Test database connectivity
test_db_connection() {
    log_info "Testing database connection..."
    export PGPASSWORD="$POSTGRES_PASSWORD"

    if ! pg_isready -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t 10 &>/dev/null; then
        log_error "Cannot connect to PostgreSQL at $POSTGRES_HOST:$POSTGRES_PORT"
        exit 1
    fi

    log_info "Database connection successful"
}

# Get database size for logging
get_db_size() {
    export PGPASSWORD="$POSTGRES_PASSWORD"
    psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -t -c \
        "SELECT pg_size_pretty(pg_database_size('$POSTGRES_DB'));" 2>/dev/null | tr -d ' '
}

# Perform the backup
perform_backup() {
    TIMESTAMP=$(date '+%Y%m%d_%H%M%S')
    BACKUP_DIR=$(mktemp -d)
    BACKUP_FILENAME="postgres_${POSTGRES_DB}_${BACKUP_TYPE}_${TIMESTAMP}.sql.gz"
    BACKUP_FILE="${BACKUP_DIR}/${BACKUP_FILENAME}"

    log_info "Starting $BACKUP_TYPE backup of database '$POSTGRES_DB'..."
    log_info "Database size: $(get_db_size)"

    export PGPASSWORD="$POSTGRES_PASSWORD"

    # Build pg_dump command
    local pg_dump_opts=(
        "-h" "$POSTGRES_HOST"
        "-p" "$POSTGRES_PORT"
        "-U" "$POSTGRES_USER"
        "-d" "$POSTGRES_DB"
        "--no-owner"
        "--no-acl"
        "--verbose"
    )

    if [[ "$BACKUP_TYPE" == "schema" ]]; then
        pg_dump_opts+=("--schema-only")
    else
        # Full backup with custom format options
        pg_dump_opts+=("--large-objects")
    fi

    # Run pg_dump with compression
    local start_time
    start_time=$(date +%s)

    if ! pg_dump "${pg_dump_opts[@]}" 2>/dev/null | gzip -9 > "$BACKUP_FILE"; then
        log_error "pg_dump failed"
        exit 1
    fi

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))

    local backup_size
    backup_size=$(du -h "$BACKUP_FILE" | cut -f1)

    log_info "Backup completed in ${duration}s, size: ${backup_size}"
}

# Upload to S3
upload_to_s3() {
    local s3_key="${S3_PREFIX}/${BACKUP_FILENAME}"
    local s3_uri="s3://${S3_BUCKET}/${s3_key}"

    log_info "Uploading backup to $s3_uri..."

    # Build AWS S3 upload command
    local aws_opts=(
        "s3" "cp"
        "$BACKUP_FILE"
        "$s3_uri"
        "--region" "$AWS_REGION"
    )

    # Add server-side encryption
    if [[ -n "${ENCRYPTION_KEY_ID:-}" ]]; then
        aws_opts+=(
            "--sse" "aws:kms"
            "--sse-kms-key-id" "$ENCRYPTION_KEY_ID"
        )
    else
        aws_opts+=("--sse" "AES256")
    fi

    # Add metadata
    aws_opts+=(
        "--metadata" "backup-type=${BACKUP_TYPE},database=${POSTGRES_DB},timestamp=${TIMESTAMP},retention-days=${RETENTION_DAYS}"
    )

    # Add storage class for cost optimization
    aws_opts+=("--storage-class" "STANDARD_IA")

    local start_time
    start_time=$(date +%s)

    if ! aws "${aws_opts[@]}"; then
        log_error "Failed to upload backup to S3"
        exit 1
    fi

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))

    log_info "Upload completed in ${duration}s"

    # Tag object with retention policy
    log_info "Applying retention tag (${RETENTION_DAYS} days)..."
    aws s3api put-object-tagging \
        --bucket "$S3_BUCKET" \
        --key "$s3_key" \
        --tagging "TagSet=[{Key=RetentionDays,Value=${RETENTION_DAYS}},{Key=BackupType,Value=postgres},{Key=Environment,Value=${ENVIRONMENT:-production}}]" \
        --region "$AWS_REGION" || log_warn "Failed to apply tags"
}

# Write backup manifest
write_manifest() {
    local manifest_key="${S3_PREFIX}/latest_${BACKUP_TYPE}.json"
    local manifest_file="${BACKUP_DIR}/manifest.json"

    cat > "$manifest_file" <<EOF
{
    "timestamp": "${TIMESTAMP}",
    "type": "${BACKUP_TYPE}",
    "database": "${POSTGRES_DB}",
    "host": "${POSTGRES_HOST}",
    "backup_file": "${S3_PREFIX}/${BACKUP_FILENAME}",
    "retention_days": ${RETENTION_DAYS},
    "created_at": "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
}
EOF

    aws s3 cp "$manifest_file" "s3://${S3_BUCKET}/${manifest_key}" \
        --region "$AWS_REGION" \
        --content-type "application/json" || log_warn "Failed to write manifest"

    rm -f "$manifest_file"
}

# Cleanup old backups based on retention policy
cleanup_old_backups() {
    log_info "Cleaning up backups older than ${RETENTION_DAYS} days..."

    local cutoff_date
    cutoff_date=$(date -d "-${RETENTION_DAYS} days" '+%Y-%m-%d' 2>/dev/null || \
                  date -v-"${RETENTION_DAYS}"d '+%Y-%m-%d' 2>/dev/null)

    if [[ -z "$cutoff_date" ]]; then
        log_warn "Could not calculate cutoff date, skipping cleanup"
        return
    fi

    # List and delete old backups
    local deleted_count=0
    while IFS= read -r line; do
        local file_date
        file_date=$(echo "$line" | awk '{print $1}')
        local file_key
        file_key=$(echo "$line" | awk '{print $4}')

        if [[ -n "$file_key" && "$file_date" < "$cutoff_date" ]]; then
            if aws s3 rm "s3://${S3_BUCKET}/${file_key}" --region "$AWS_REGION" 2>/dev/null; then
                ((deleted_count++))
            fi
        fi
    done < <(aws s3 ls "s3://${S3_BUCKET}/${S3_PREFIX}/" --region "$AWS_REGION" 2>/dev/null | grep "postgres_${POSTGRES_DB}_" || true)

    if [[ $deleted_count -gt 0 ]]; then
        log_info "Deleted $deleted_count old backup(s)"
    else
        log_info "No old backups to delete"
    fi
}

# Main execution
main() {
    log_info "========================================"
    log_info "PostgreSQL Backup Script"
    log_info "========================================"
    log_info "Host: ${POSTGRES_HOST}:${POSTGRES_PORT}"
    log_info "Database: ${POSTGRES_DB}"
    log_info "Backup Type: ${BACKUP_TYPE}"
    log_info "S3 Bucket: ${S3_BUCKET}"
    log_info "Retention: ${RETENTION_DAYS} days"
    log_info "========================================"

    validate_env
    check_dependencies
    test_db_connection
    perform_backup
    upload_to_s3
    write_manifest
    cleanup_old_backups

    log_info "========================================"
    log_info "Backup completed successfully!"
    log_info "File: s3://${S3_BUCKET}/${S3_PREFIX}/${BACKUP_FILENAME}"
    log_info "========================================"
}

main "$@"
