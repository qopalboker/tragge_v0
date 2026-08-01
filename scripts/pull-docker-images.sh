#!/bin/bash
# Script to pre-pull Docker base images with exponential backoff retry
# This helps work around TLS handshake timeout issues with Docker Hub
#
# Usage: ./scripts/pull-docker-images.sh
#
# The script will attempt to pull all required base images before running
# docker-compose build. This separates the network-sensitive image pull
# from the build process, making it easier to retry on network failures.

set -e

# Configuration
MAX_RETRIES=4
INITIAL_BACKOFF=2  # seconds

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# List of all base images used in Dockerfiles
IMAGES=(
    "node:20-alpine"
    "nginx:1.25-alpine"
    "golang:1.23-alpine"
)

# Function to pull an image with retry and exponential backoff
pull_with_retry() {
    local image=$1
    local attempt=1
    local backoff=$INITIAL_BACKOFF

    echo -e "${YELLOW}Pulling ${image}...${NC}"

    while [ $attempt -le $MAX_RETRIES ]; do
        if docker pull "$image" 2>&1; then
            echo -e "${GREEN}Successfully pulled ${image}${NC}"
            return 0
        else
            if [ $attempt -lt $MAX_RETRIES ]; then
                echo -e "${YELLOW}Attempt $attempt failed for ${image}. Retrying in ${backoff}s...${NC}"
                sleep $backoff
                backoff=$((backoff * 2))
                attempt=$((attempt + 1))
            else
                echo -e "${RED}Failed to pull ${image} after $MAX_RETRIES attempts${NC}"
                return 1
            fi
        fi
    done
}

# Function to check if image already exists locally
image_exists() {
    local image=$1
    docker image inspect "$image" &> /dev/null
}

# Main
echo "========================================"
echo "Docker Image Pre-Pull Script"
echo "========================================"
echo ""
echo "This script pre-pulls base images to avoid TLS timeout issues"
echo "during docker-compose build."
echo ""

# Check if Docker is running
if ! docker info &> /dev/null; then
    echo -e "${RED}Error: Docker is not running or not accessible${NC}"
    echo "Please start Docker and try again."
    exit 1
fi

# Track failures
failed_images=()
skipped_images=()
pulled_images=()

for image in "${IMAGES[@]}"; do
    if image_exists "$image"; then
        echo -e "${GREEN}[SKIP] ${image} already exists locally${NC}"
        skipped_images+=("$image")
    else
        if pull_with_retry "$image"; then
            pulled_images+=("$image")
        else
            failed_images+=("$image")
        fi
    fi
    echo ""
done

# Summary
echo "========================================"
echo "Summary"
echo "========================================"
echo -e "Pulled:  ${GREEN}${#pulled_images[@]}${NC} images"
echo -e "Skipped: ${YELLOW}${#skipped_images[@]}${NC} images (already local)"
echo -e "Failed:  ${RED}${#failed_images[@]}${NC} images"

if [ ${#failed_images[@]} -gt 0 ]; then
    echo ""
    echo -e "${RED}Failed images:${NC}"
    for img in "${failed_images[@]}"; do
        echo "  - $img"
    done
    echo ""
    echo "Suggestions:"
    echo "  1. Check your network connection"
    echo "  2. Run: sudo ./scripts/fix-docker-tls.sh (configures registry mirrors)"
    echo "  3. Try again later"
    exit 1
fi

echo ""
echo -e "${GREEN}All images ready. You can now run: make up${NC}"
