#!/usr/bin/env bash
#
# backup-redis.sh - Redis backup to S3
#
# This script performs a Redis backup by triggering BGSAVE and copying
# the RDB file to S3 with proper encryption and retention tagging.
#
# Usage: ./backup-redis.sh
#
# Environment Variables Required:
#   REDIS_HOST          - Redis host (default: localhost)
#   REDIS_PORT          - Redis port (default: 6379)
#   REDIS_PASSWORD      - Redis password (optional)
#   REDIS_RDB_PATH      - Path to RDB file (default: /data/dump.rdb)
#   S3_BUCKET           - S3 bucket name (required)
#   S3_PREFIX           - S3 key prefix (default: backups/redis)
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
REDIS_HOST="${REDIS_HOST:-localhost}"
REDIS_PORT="${REDIS_PORT:-6379}"
REDIS_RDB_PATH="${REDIS_RDB_PATH:-/data/dump.rdb}"
S3_PREFIX="${S3_PREFIX:-backups/redis}"
AWS_REGION="${AWS_REGION:-us-east-1}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
BGSAVE_TIMEOUT="${BGSAVE_TIMEOUT:-300}"

# Validate required environment variables
validate_env() {
    local missing=()

    [[ -z "${S3_BUCKET:-}" ]] && missing+=("S3_BUCKET")

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required environment variables: ${missing[*]}"
        exit 1
    fi
}

# Check required tools
check_dependencies() {
    local deps=("redis-cli" "aws" "gzip")
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

# Build redis-cli command with auth if needed
redis_cmd() {
    local cmd=("redis-cli" "-h" "$REDIS_HOST" "-p" "$REDIS_PORT")
    if [[ -n "${REDIS_PASSWORD:-}" ]]; then
        cmd+=("-a" "$REDIS_PASSWORD" "--no-auth-warning")
    fi
    "${cmd[@]}" "$@"
}

# Test Redis connectivity
test_redis_connection() {
    log_info "Testing Redis connection..."

    if ! redis_cmd PING | grep -q "PONG"; then
        log_error "Cannot connect to Redis at $REDIS_HOST:$REDIS_PORT"
        exit 1
    fi

    log_info "Redis connection successful"
}

# Get Redis info for logging
get_redis_info() {
    local info
    info=$(redis_cmd INFO memory 2>/dev/null | grep -E "^used_memory_human:" | cut -d: -f2 | tr -d '\r')
    echo "${info:-unknown}"
}

# Get last save time
get_last_save() {
    redis_cmd LASTSAVE 2>/dev/null | tr -d '\r'
}

# Trigger BGSAVE and wait for completion
trigger_bgsave() {
    log_info "Triggering Redis BGSAVE..."

    local last_save_before
    last_save_before=$(get_last_save)

    # Check if BGSAVE is already in progress
    local bgsave_status
    bgsave_status=$(redis_cmd LASTSAVE 2>/dev/null)

    # Trigger BGSAVE
    if ! redis_cmd BGSAVE 2>/dev/null | grep -qE "(Background saving started|Background saving scheduled)"; then
        # BGSAVE might already be running, check status
        local info
        info=$(redis_cmd INFO persistence 2>/dev/null | grep "rdb_bgsave_in_progress" | cut -d: -f2 | tr -d '\r')
        if [[ "$info" != "1" ]]; then
            log_warn "BGSAVE command returned unexpected response, continuing..."
        fi
    fi

    # Wait for BGSAVE to complete
    log_info "Waiting for BGSAVE to complete (timeout: ${BGSAVE_TIMEOUT}s)..."
    local waited=0
    local check_interval=5

    while [[ $waited -lt $BGSAVE_TIMEOUT ]]; do
        local last_save_after
        last_save_after=$(get_last_save)

        if [[ "$last_save_after" -gt "$last_save_before" ]]; then
            log_info "BGSAVE completed successfully"
            return 0
        fi

        # Check for errors
        local last_bgsave_status
        last_bgsave_status=$(redis_cmd INFO persistence 2>/dev/null | grep "rdb_last_bgsave_status" | cut -d: -f2 | tr -d '\r')

        if [[ "$last_bgsave_status" == "err" ]]; then
            log_error "BGSAVE failed with error"
            exit 1
        fi

        sleep $check_interval
        waited=$((waited + check_interval))
        log_info "Still waiting for BGSAVE... (${waited}s elapsed)"
    done

    log_error "BGSAVE timeout after ${BGSAVE_TIMEOUT}s"
    exit 1
}

# Copy RDB file
copy_rdb_file() {
    TIMESTAMP=$(date '+%Y%m%d_%H%M%S')
    BACKUP_DIR=$(mktemp -d)
    BACKUP_FILENAME="redis_${TIMESTAMP}.rdb.gz"
    BACKUP_FILE="${BACKUP_DIR}/${BACKUP_FILENAME}"

    log_info "Copying RDB file from $REDIS_RDB_PATH..."

    # Handle different access methods for RDB file
    if [[ -f "$REDIS_RDB_PATH" ]]; then
        # Direct file access
        if ! gzip -c "$REDIS_RDB_PATH" > "$BACKUP_FILE"; then
            log_error "Failed to copy and compress RDB file"
            exit 1
        fi
    elif command -v kubectl &>/dev/null && [[ -n "${REDIS_POD:-}" ]]; then
        # Kubernetes: copy from pod
        log_info "Copying RDB from Kubernetes pod $REDIS_POD..."
        local temp_rdb="${BACKUP_DIR}/dump.rdb"
        if ! kubectl cp "${REDIS_NAMESPACE:-default}/${REDIS_POD}:${REDIS_RDB_PATH}" "$temp_rdb"; then
            log_error "Failed to copy RDB from pod"
            exit 1
        fi
        gzip -c "$temp_rdb" > "$BACKUP_FILE"
        rm -f "$temp_rdb"
    elif command -v docker &>/dev/null && [[ -n "${REDIS_CONTAINER:-}" ]]; then
        # Docker: copy from container
        log_info "Copying RDB from Docker container $REDIS_CONTAINER..."
        local temp_rdb="${BACKUP_DIR}/dump.rdb"
        if ! docker cp "${REDIS_CONTAINER}:${REDIS_RDB_PATH}" "$temp_rdb"; then
            log_error "Failed to copy RDB from container"
            exit 1
        fi
        gzip -c "$temp_rdb" > "$BACKUP_FILE"
        rm -f "$temp_rdb"
    else
        log_error "Cannot access RDB file at $REDIS_RDB_PATH"
        log_error "Set REDIS_POD or REDIS_CONTAINER for remote access"
        exit 1
    fi

    local backup_size
    backup_size=$(du -h "$BACKUP_FILE" | cut -f1)
    log_info "RDB backup size: ${backup_size}"
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
        "--metadata" "backup-type=redis-rdb,timestamp=${TIMESTAMP},retention-days=${RETENTION_DAYS}"
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
        --tagging "TagSet=[{Key=RetentionDays,Value=${RETENTION_DAYS}},{Key=BackupType,Value=redis},{Key=Environment,Value=${ENVIRONMENT:-production}}]" \
        --region "$AWS_REGION" || log_warn "Failed to apply tags"
}

# Write backup manifest
write_manifest() {
    local manifest_key="${S3_PREFIX}/latest.json"
    local manifest_file="${BACKUP_DIR}/manifest.json"

    local redis_keys
    redis_keys=$(redis_cmd DBSIZE 2>/dev/null | grep -oE '[0-9]+' || echo "unknown")

    cat > "$manifest_file" <<EOF
{
    "timestamp": "${TIMESTAMP}",
    "type": "redis-rdb",
    "host": "${REDIS_HOST}",
    "port": ${REDIS_PORT},
    "backup_file": "${S3_PREFIX}/${BACKUP_FILENAME}",
    "retention_days": ${RETENTION_DAYS},
    "keys_count": "${redis_keys}",
    "memory_usage": "$(get_redis_info)",
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
    done < <(aws s3 ls "s3://${S3_BUCKET}/${S3_PREFIX}/" --region "$AWS_REGION" 2>/dev/null | grep "redis_" || true)

    if [[ $deleted_count -gt 0 ]]; then
        log_info "Deleted $deleted_count old backup(s)"
    else
        log_info "No old backups to delete"
    fi
}

# Main execution
main() {
    log_info "========================================"
    log_info "Redis Backup Script"
    log_info "========================================"
    log_info "Host: ${REDIS_HOST}:${REDIS_PORT}"
    log_info "RDB Path: ${REDIS_RDB_PATH}"
    log_info "S3 Bucket: ${S3_BUCKET}"
    log_info "Retention: ${RETENTION_DAYS} days"
    log_info "========================================"

    validate_env
    check_dependencies
    test_redis_connection

    log_info "Redis memory usage: $(get_redis_info)"

    trigger_bgsave
    copy_rdb_file
    upload_to_s3
    write_manifest
    cleanup_old_backups

    log_info "========================================"
    log_info "Backup completed successfully!"
    log_info "File: s3://${S3_BUCKET}/${S3_PREFIX}/${BACKUP_FILENAME}"
    log_info "========================================"
}

main "$@"
