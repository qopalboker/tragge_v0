#!/bin/bash

# =============================================================================
# Staging vs Production Comparison Script
# =============================================================================
# This script helps visualize differences between staging and production
# configurations.
#
# Usage:
#   ./compare.sh [resource-type]
#
# Examples:
#   ./compare.sh              # Compare all resources
#   ./compare.sh deployment   # Compare only deployments
#   ./compare.sh ingress      # Compare only ingress
# =============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
STAGING_DIR="infra/k8s/overlays/staging"
PRODUCTION_DIR="infra/k8s/overlays/production"
RESOURCE_TYPE="${1:-all}"

echo -e "${BLUE}==============================================================================${NC}"
echo -e "${BLUE}Tragge Platform: Staging vs Production Comparison${NC}"
echo -e "${BLUE}==============================================================================${NC}"
echo

# Check if kustomize is available
if ! command -v kustomize &> /dev/null; then
    echo -e "${YELLOW}WARNING: kustomize not found. Using kubectl kustomize instead.${NC}"
    KUSTOMIZE_CMD="kubectl kustomize"
else
    KUSTOMIZE_CMD="kustomize build"
fi

# Function to extract specific resource type
extract_resource() {
    local manifest="$1"
    local resource="$2"

    if [ "$resource" = "all" ]; then
        echo "$manifest"
    else
        echo "$manifest" | grep -A 100 "kind: $resource" || echo ""
    fi
}

# Generate manifests
echo -e "${YELLOW}Generating manifests...${NC}"
PROD_MANIFEST=$(${KUSTOMIZE_CMD} ${PRODUCTION_DIR} 2>/dev/null || echo "ERROR: Failed to generate production manifest")
STAGING_MANIFEST=$(${KUSTOMIZE_CMD} ${STAGING_DIR} 2>/dev/null || echo "ERROR: Failed to generate staging manifest")

if [[ "$PROD_MANIFEST" == ERROR* ]] || [[ "$STAGING_MANIFEST" == ERROR* ]]; then
    echo -e "${RED}Failed to generate manifests. Check kustomize configuration.${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Manifests generated successfully${NC}"
echo

# =============================================================================
# NAMESPACE COMPARISON
# =============================================================================
echo -e "${BLUE}--- Namespace Comparison ---${NC}"
PROD_NS=$(echo "$PROD_MANIFEST" | grep "namespace:" | head -1 | awk '{print $2}')
STAGING_NS=$(echo "$STAGING_MANIFEST" | grep "namespace:" | head -1 | awk '{print $2}')

echo -e "Production: ${GREEN}$PROD_NS${NC}"
echo -e "Staging:    ${YELLOW}$STAGING_NS${NC}"
echo

# =============================================================================
# REPLICA COUNT COMPARISON
# =============================================================================
if [ "$RESOURCE_TYPE" = "all" ] || [ "$RESOURCE_TYPE" = "Deployment" ]; then
    echo -e "${BLUE}--- Replica Count Comparison ---${NC}"
    echo -e "${BLUE}Service${NC}                  ${GREEN}Production${NC}    ${YELLOW}Staging${NC}"
    echo "--------------------------------------------------------"

    for service in user-bff trade-bff admin-bff gateway frontend trading-engine market-ingestor leaderboard-worker; do
        PROD_REPLICAS=$(echo "$PROD_MANIFEST" | grep -A 5 "name: $service" | grep "replicas:" | head -1 | awk '{print $2}')
        STAGING_REPLICAS=$(echo "$STAGING_MANIFEST" | grep -A 5 "name: $service" | grep "replicas:" | head -1 | awk '{print $2}')

        printf "%-25s ${GREEN}%-13s${NC} ${YELLOW}%-10s${NC}\n" "$service" "$PROD_REPLICAS" "$STAGING_REPLICAS"
    done
    echo
fi

# =============================================================================
# RESOURCE LIMITS COMPARISON
# =============================================================================
if [ "$RESOURCE_TYPE" = "all" ] || [ "$RESOURCE_TYPE" = "Deployment" ]; then
    echo -e "${BLUE}--- Resource Limits Comparison (user-bff example) ---${NC}"
    echo "Production:"
    echo "$PROD_MANIFEST" | grep -A 30 "name: user-bff" | grep -A 10 "resources:" | head -10
    echo
    echo "Staging:"
    echo "$STAGING_MANIFEST" | grep -A 30 "name: user-bff" | grep -A 10 "resources:" | head -10
    echo
fi

# =============================================================================
# IMAGE TAG COMPARISON
# =============================================================================
if [ "$RESOURCE_TYPE" = "all" ] || [ "$RESOURCE_TYPE" = "Deployment" ]; then
    echo -e "${BLUE}--- Image Tag Comparison ---${NC}"
    echo -e "${BLUE}Service${NC}                  ${GREEN}Production${NC}         ${YELLOW}Staging${NC}"
    echo "--------------------------------------------------------"

    for service in user-bff trade-bff admin-bff; do
        PROD_TAG=$(echo "$PROD_MANIFEST" | grep "image: tragge/$service" | head -1 | sed 's/.*://' | xargs)
        STAGING_TAG=$(echo "$STAGING_MANIFEST" | grep "image: tragge/$service" | head -1 | sed 's/.*://' | xargs)

        printf "%-25s ${GREEN}%-18s${NC} ${YELLOW}%-15s${NC}\n" "$service" "$PROD_TAG" "$STAGING_TAG"
    done
    echo
fi

# =============================================================================
# INGRESS COMPARISON
# =============================================================================
if [ "$RESOURCE_TYPE" = "all" ] || [ "$RESOURCE_TYPE" = "Ingress" ]; then
    echo -e "${BLUE}--- Ingress Host Comparison ---${NC}"
    echo "Production Hosts:"
    echo "$PROD_MANIFEST" | grep -A 2 "kind: Ingress" | grep "host:" | awk '{print "  - " $2}'
    echo
    echo "Staging Hosts:"
    echo "$STAGING_MANIFEST" | grep -A 2 "kind: Ingress" | grep "host:" | awk '{print "  - " $2}'
    echo

    echo -e "${BLUE}--- TLS Issuer Comparison ---${NC}"
    PROD_ISSUER=$(echo "$PROD_MANIFEST" | grep "cert-manager.io/cluster-issuer" | head -1 | awk '{print $2}' | tr -d '"')
    STAGING_ISSUER=$(echo "$STAGING_MANIFEST" | grep "cert-manager.io/cluster-issuer" | head -1 | awk '{print $2}' | tr -d '"')

    echo -e "Production: ${GREEN}$PROD_ISSUER${NC}"
    echo -e "Staging:    ${YELLOW}$STAGING_ISSUER${NC}"
    echo
fi

# =============================================================================
# LABEL COMPARISON
# =============================================================================
echo -e "${BLUE}--- Common Labels Comparison ---${NC}"
echo "Production Labels:"
echo "$PROD_MANIFEST" | grep -A 5 "commonLabels:" | head -6
echo
echo "Staging Labels:"
echo "$STAGING_MANIFEST" | grep -A 5 "commonLabels:" | head -6
echo

# =============================================================================
# FULL DIFF
# =============================================================================
if [ "$RESOURCE_TYPE" = "all" ]; then
    echo -e "${BLUE}--- Full Manifest Diff ---${NC}"
    echo "Writing diff to staging-vs-production.diff..."

    diff <(echo "$PROD_MANIFEST") <(echo "$STAGING_MANIFEST") > staging-vs-production.diff || true

    DIFF_LINES=$(wc -l < staging-vs-production.diff)
    echo -e "${GREEN}✓ Diff written to staging-vs-production.diff ($DIFF_LINES lines)${NC}"
    echo
fi

# =============================================================================
# SUMMARY
# =============================================================================
echo -e "${BLUE}==============================================================================${NC}"
echo -e "${BLUE}Summary${NC}"
echo -e "${BLUE}==============================================================================${NC}"
echo

# Count resources
PROD_DEPLOYMENTS=$(echo "$PROD_MANIFEST" | grep -c "kind: Deployment" || echo "0")
STAGING_DEPLOYMENTS=$(echo "$STAGING_MANIFEST" | grep -c "kind: Deployment" || echo "0")

PROD_SERVICES=$(echo "$PROD_MANIFEST" | grep -c "kind: Service" || echo "0")
STAGING_SERVICES=$(echo "$STAGING_MANIFEST" | grep -c "kind: Service" || echo "0")

PROD_INGRESS=$(echo "$PROD_MANIFEST" | grep -c "kind: Ingress" || echo "0")
STAGING_INGRESS=$(echo "$STAGING_MANIFEST" | grep -c "kind: Ingress" || echo "0")

echo -e "Resource Counts:"
echo -e "  Deployments:  Production: ${GREEN}$PROD_DEPLOYMENTS${NC}, Staging: ${YELLOW}$STAGING_DEPLOYMENTS${NC}"
echo -e "  Services:     Production: ${GREEN}$PROD_SERVICES${NC}, Staging: ${YELLOW}$STAGING_SERVICES${NC}"
echo -e "  Ingress:      Production: ${GREEN}$PROD_INGRESS${NC}, Staging: ${YELLOW}$STAGING_INGRESS${NC}"
echo

# Key differences
echo -e "${YELLOW}Key Differences:${NC}"
echo "  1. Namespace: tragge (prod) vs tragge-staging (staging)"
echo "  2. Replicas: 3 (prod) vs 1-2 (staging)"
echo "  3. Resources: 100% (prod) vs 50% (staging)"
echo "  4. Domain: tragge.example.com (prod) vs staging.tragge.example.com (staging)"
echo "  5. TLS Issuer: letsencrypt-prod vs letsencrypt-staging"
echo "  6. Image Tags: latest (prod) vs staging (staging)"
echo

echo -e "${GREEN}✓ Comparison complete!${NC}"
