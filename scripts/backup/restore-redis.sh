#!/usr/bin/env bash
#
# restore-redis.sh - Redis restore from S3
#
# This script restores Redis from an RDB backup stored in S3.
# It includes safety checks and handles the restore process carefully.
#
# Usage: ./restore-redis.sh [--backup-file FILE] [--latest] [--dry-run]
#
# Environment Variables Required:
#   REDIS_HOST          - Redis host (default: localhost)
#   REDIS_PORT          - Redis port (default: 6379)
#   REDIS_PASSWORD      - Redis password (optional)
#   REDIS_RDB_PATH      - Path to RDB file (default: /data/dump.rdb)
#   S3_BUCKET           - S3 bucket name (required)
#   S3_PREFIX           - S3 key prefix (default: backups/redis)
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
REDIS_HOST="${REDIS_HOST:-localhost}"
REDIS_PORT="${REDIS_PORT:-6379}"
REDIS_RDB_PATH="${REDIS_RDB_PATH:-/data/dump.rdb}"
S3_PREFIX="${S3_PREFIX:-backups/redis}"
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
    echo "  $0 --backup-file backups/redis/redis_20240115_030000.rdb.gz"
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

    [[ -z "${S3_BUCKET:-}" ]] && missing+=("S3_BUCKET")

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required environment variables: ${missing[*]}"
        exit 1
    fi
}

# Check required tools
check_dependencies() {
    local deps=("redis-cli" "aws" "gunzip")
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

# List available backups
list_backups() {
    log_info "Available Redis backups in s3://${S3_BUCKET}/${S3_PREFIX}/:"
    echo ""
    echo "Date/Time            Size       Filename"
    echo "------------------------------------------------------------"

    aws s3 ls "s3://${S3_BUCKET}/${S3_PREFIX}/" --region "$AWS_REGION" 2>/dev/null | \
        grep "redis_" | \
        while read -r date time size file; do
            printf "%-20s %-10s %s\n" "${date} ${time}" "$size" "$file"
        done

    echo ""

    # Show latest manifest if available
    local manifest
    manifest=$(aws s3 cp "s3://${S3_BUCKET}/${S3_PREFIX}/latest.json" - --region "$AWS_REGION" 2>/dev/null || true)
    if [[ -n "$manifest" ]]; then
        log_info "Latest backup manifest:"
        echo "$manifest" | python3 -m json.tool 2>/dev/null || echo "$manifest"
    fi
}

# Get latest backup file
get_latest_backup() {
    log_info "Finding latest backup..."

    # Try to get from manifest first
    local manifest
    manifest=$(aws s3 cp "s3://${S3_BUCKET}/${S3_PREFIX}/latest.json" - --region "$AWS_REGION" 2>/dev/null || true)

    if [[ -n "$manifest" ]]; then
        BACKUP_FILE=$(echo "$manifest" | python3 -c "import sys, json; print(json.load(sys.stdin).get('backup_file', ''))" 2>/dev/null || true)
        if [[ -n "$BACKUP_FILE" ]]; then
            log_info "Found latest backup from manifest: $BACKUP_FILE"
            return
        fi
    fi

    # Fallback: list and get most recent
    BACKUP_FILE=$(aws s3 ls "s3://${S3_BUCKET}/${S3_PREFIX}/" --region "$AWS_REGION" 2>/dev/null | \
        grep "redis_.*\.rdb\.gz" | \
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
    local dbsize
    dbsize=$(redis_cmd DBSIZE 2>/dev/null | grep -oE '[0-9]+' || echo "0")

    local memory
    memory=$(redis_cmd INFO memory 2>/dev/null | grep -E "^used_memory_human:" | cut -d: -f2 | tr -d '\r' || echo "unknown")

    echo "Keys: ${dbsize}, Memory: ${memory}"
}

# Get Redis RDB directory
get_redis_dir() {
    redis_cmd CONFIG GET dir 2>/dev/null | tail -1 | tr -d '\r'
}

# Get Redis RDB filename
get_redis_dbfilename() {
    redis_cmd CONFIG GET dbfilename 2>/dev/null | tail -1 | tr -d '\r'
}

# Confirm restore operation
confirm_restore() {
    if [[ "$FORCE" == true ]]; then
        return
    fi

    echo ""
    echo -e "${RED}╔════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${RED}║                    ⚠️  WARNING ⚠️                           ║${NC}"
    echo -e "${RED}║  This will OVERWRITE all data in Redis!                    ║${NC}"
    echo -e "${RED}║  Redis will need to be restarted during the process.       ║${NC}"
    echo -e "${RED}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "Target Redis: ${REDIS_HOST}:${REDIS_PORT}"
    echo "Current stats: $(get_redis_info)"
    echo "Backup file: ${BACKUP_FILE}"
    echo ""

    read -p "Type 'RESTORE' to confirm: " -r confirmation

    if [[ "$confirmation" != "RESTORE" ]]; then
        log_info "Restore cancelled"
        exit 0
    fi
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

    if [[ -x "${script_dir}/backup-redis.sh" ]]; then
        S3_PREFIX="${S3_PREFIX}/pre-restore" "${script_dir}/backup-redis.sh" || {
            log_error "Failed to create pre-restore backup"
            exit 1
        }
    else
        log_warn "backup-redis.sh not found, skipping pre-restore backup"
    fi
}

# Download backup from S3
download_backup() {
    RESTORE_DIR=$(mktemp -d)
    local local_file="${RESTORE_DIR}/backup.rdb.gz"
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

    # Decompress
    log_info "Decompressing backup..."
    gunzip "$local_file"

    DOWNLOADED_FILE="${RESTORE_DIR}/backup.rdb"
}

# Perform the restore
perform_restore() {
    if [[ "$DRY_RUN" == true ]]; then
        log_info "[DRY RUN] Would restore from: $BACKUP_FILE"
        log_info "[DRY RUN] Target: Redis at $REDIS_HOST:$REDIS_PORT"
        return
    fi

    log_step "Starting Redis restore..."

    local redis_dir
    redis_dir=$(get_redis_dir)
    local redis_dbfilename
    redis_dbfilename=$(get_redis_dbfilename)

    if [[ -z "$redis_dir" || -z "$redis_dbfilename" ]]; then
        redis_dir=$(dirname "$REDIS_RDB_PATH")
        redis_dbfilename=$(basename "$REDIS_RDB_PATH")
        log_warn "Could not get Redis config, using: ${redis_dir}/${redis_dbfilename}"
    fi

    local target_rdb="${redis_dir}/${redis_dbfilename}"

    # Method 1: Direct file copy (if we have access)
    if [[ -d "$redis_dir" && -w "$redis_dir" ]]; then
        log_info "Using direct file copy method..."

        # Stop Redis accepting writes
        log_info "Setting Redis to read-only mode..."
        redis_cmd CONFIG SET stop-writes-on-bgsave-error yes 2>/dev/null || true

        # Wait for any ongoing BGSAVE
        log_info "Waiting for any ongoing save operations..."
        while [[ $(redis_cmd INFO persistence 2>/dev/null | grep "rdb_bgsave_in_progress:1" || true) ]]; do
            sleep 1
        done

        # Shutdown Redis with NOSAVE
        log_info "Shutting down Redis (NOSAVE)..."
        redis_cmd SHUTDOWN NOSAVE 2>/dev/null || true

        # Wait for Redis to stop
        sleep 2

        # Copy RDB file
        log_info "Copying RDB file..."
        cp "$DOWNLOADED_FILE" "$target_rdb"
        chmod 644 "$target_rdb"

        log_info "Please restart Redis manually to complete the restore"
        log_info "RDB file copied to: $target_rdb"

    # Method 2: Kubernetes
    elif command -v kubectl &>/dev/null && [[ -n "${REDIS_POD:-}" ]]; then
        log_info "Using Kubernetes restore method..."

        local namespace="${REDIS_NAMESPACE:-default}"

        # Scale down Redis if using a StatefulSet/Deployment
        if [[ -n "${REDIS_STATEFULSET:-}" ]]; then
            log_info "Scaling down Redis StatefulSet..."
            kubectl scale statefulset "$REDIS_STATEFULSET" --replicas=0 -n "$namespace"
            kubectl wait --for=delete pod/"$REDIS_POD" -n "$namespace" --timeout=60s || true
        fi

        # Copy RDB file to PVC (requires a temporary pod or init container)
        log_warn "Kubernetes restore requires manual intervention"
        log_info "Steps:"
        log_info "1. Copy RDB file to the Redis PVC"
        log_info "2. Scale Redis back up"
        log_info ""
        log_info "RDB file location: $DOWNLOADED_FILE"

    # Method 3: Docker
    elif command -v docker &>/dev/null && [[ -n "${REDIS_CONTAINER:-}" ]]; then
        log_info "Using Docker restore method..."

        # Stop container
        log_info "Stopping Redis container..."
        docker stop "$REDIS_CONTAINER" || true

        # Copy RDB file
        log_info "Copying RDB file to container..."
        docker cp "$DOWNLOADED_FILE" "${REDIS_CONTAINER}:${target_rdb}"

        # Start container
        log_info "Starting Redis container..."
        docker start "$REDIS_CONTAINER"

        # Wait for Redis to be ready
        log_info "Waiting for Redis to start..."
        local retries=30
        while [[ $retries -gt 0 ]]; do
            if redis_cmd PING 2>/dev/null | grep -q "PONG"; then
                break
            fi
            sleep 1
            ((retries--))
        done

    else
        log_error "Cannot determine restore method"
        log_error "Set REDIS_POD (Kubernetes) or REDIS_CONTAINER (Docker) for remote restore"
        log_info "RDB file available at: $DOWNLOADED_FILE"
        exit 1
    fi
}

# Verify restore
verify_restore() {
    log_step "Verifying restore..."

    # Wait for Redis to be ready
    local retries=30
    while [[ $retries -gt 0 ]]; do
        if redis_cmd PING 2>/dev/null | grep -q "PONG"; then
            break
        fi
        sleep 1
        ((retries--))
    done

    if ! redis_cmd PING 2>/dev/null | grep -q "PONG"; then
        log_warn "Redis is not responding - manual verification required"
        return
    fi

    log_info "Post-restore stats: $(get_redis_info)"
}

# Main execution
main() {
    log_info "========================================"
    log_info "Redis Restore Script"
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

    log_info "Target: Redis @ ${REDIS_HOST}:${REDIS_PORT}"
    log_info "Backup: ${BACKUP_FILE}"
    log_info "Dry Run: ${DRY_RUN}"
    log_info "========================================"

    verify_backup_exists
    test_redis_connection
    confirm_restore
    create_pre_restore_backup
    download_backup
    perform_restore
    verify_restore

    log_info "========================================"
    log_info "Restore completed!"
    log_info "========================================"
}

main "$@"
