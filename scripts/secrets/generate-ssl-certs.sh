#!/bin/bash
# ==============================================================================
# Generate SSL/TLS Certificates for PostgreSQL
# ==============================================================================
# This script generates self-signed certificates for development/testing
# or prepares a CSR for production use with a real CA.
#
# Certificate Types Generated:
#   - CA certificate (root certificate authority)
#   - Server certificate (for PostgreSQL server)
#   - Client certificates (optional, for client authentication)
#
# Usage:
#   ./scripts/secrets/generate-ssl-certs.sh [--output-dir <path>] [--production]
#
# Options:
#   --output-dir    Directory to store certificates (default: infra/docker/ssl)
#   --production    Generate CSRs for production CA signing
#   --cn            Common Name for server certificate (default: postgres)
#   --days          Certificate validity in days (default: 365)
#   --force         Overwrite existing certificates
#
# For Production:
#   1. Run with --production flag to generate CSRs
#   2. Submit CSRs to your Certificate Authority
#   3. Replace the signed certificates in the output directory
#
# ==============================================================================

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
DEFAULT_OUTPUT_DIR="$PROJECT_ROOT/infra/docker/ssl"
OUTPUT_DIR="$DEFAULT_OUTPUT_DIR"
PRODUCTION=false
FORCE=false
SERVER_CN="postgres"
VALIDITY_DAYS=365

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --production)
            PRODUCTION=true
            shift
            ;;
        --cn)
            SERVER_CN="$2"
            shift 2
            ;;
        --days)
            VALIDITY_DAYS="$2"
            shift 2
            ;;
        --force)
            FORCE=true
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
NC='\033[0m'

echo "=============================================="
echo "    PostgreSQL SSL Certificate Generator"
echo "=============================================="
echo ""
echo "Output directory: $OUTPUT_DIR"
echo "Server CN: $SERVER_CN"
echo "Validity: $VALIDITY_DAYS days"
echo "Mode: $([ "$PRODUCTION" = true ] && echo "Production (CSR generation)" || echo "Development (self-signed)")"
echo ""

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Check for existing certificates
if [[ -f "$OUTPUT_DIR/ca.crt" && "$FORCE" != "true" ]]; then
    echo -e "${YELLOW}Certificates already exist. Use --force to regenerate.${NC}"
    exit 0
fi

# ==============================================================================
# Generate CA Certificate
# ==============================================================================
echo -e "${BLUE}Generating CA certificate...${NC}"

# CA private key
openssl genrsa -out "$OUTPUT_DIR/ca.key" 4096

# CA certificate
openssl req -new -x509 \
    -key "$OUTPUT_DIR/ca.key" \
    -out "$OUTPUT_DIR/ca.crt" \
    -days $((VALIDITY_DAYS * 10)) \
    -subj "/C=US/ST=State/L=City/O=Tragge/OU=Database/CN=Tragge PostgreSQL CA"

chmod 600 "$OUTPUT_DIR/ca.key"
echo -e "${GREEN}[CREATED]${NC} ca.key, ca.crt"

# ==============================================================================
# Generate Server Certificate
# ==============================================================================
echo -e "${BLUE}Generating server certificate...${NC}"

# Server private key
openssl genrsa -out "$OUTPUT_DIR/server.key" 2048

# Server CSR configuration
cat > "$OUTPUT_DIR/server.cnf" << EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
req_extensions = req_ext

[dn]
C = US
ST = State
L = City
O = Tragge
OU = Database
CN = $SERVER_CN

[req_ext]
subjectAltName = @alt_names

[alt_names]
DNS.1 = $SERVER_CN
DNS.2 = localhost
DNS.3 = postgres
DNS.4 = postgres.tragge.svc.cluster.local
DNS.5 = pgbouncer
DNS.6 = pgbouncer.tragge.svc.cluster.local
IP.1 = 127.0.0.1
IP.2 = ::1
EOF

# Generate server CSR
openssl req -new \
    -key "$OUTPUT_DIR/server.key" \
    -out "$OUTPUT_DIR/server.csr" \
    -config "$OUTPUT_DIR/server.cnf"

if [[ "$PRODUCTION" = true ]]; then
    echo -e "${GREEN}[CREATED]${NC} server.key, server.csr (for CA signing)"
    echo ""
    echo -e "${YELLOW}PRODUCTION MODE:${NC}"
    echo "Submit server.csr to your Certificate Authority"
    echo "Place the signed certificate as server.crt"
else
    # Self-sign server certificate with CA
    openssl x509 -req \
        -in "$OUTPUT_DIR/server.csr" \
        -CA "$OUTPUT_DIR/ca.crt" \
        -CAkey "$OUTPUT_DIR/ca.key" \
        -CAcreateserial \
        -out "$OUTPUT_DIR/server.crt" \
        -days "$VALIDITY_DAYS" \
        -extensions req_ext \
        -extfile "$OUTPUT_DIR/server.cnf"

    echo -e "${GREEN}[CREATED]${NC} server.key, server.crt (self-signed)"
fi

chmod 600 "$OUTPUT_DIR/server.key"

# ==============================================================================
# Generate Client Certificate (optional, for client certificate auth)
# ==============================================================================
echo -e "${BLUE}Generating client certificate...${NC}"

# Client private key
openssl genrsa -out "$OUTPUT_DIR/client.key" 2048

# Client certificate configuration
cat > "$OUTPUT_DIR/client.cnf" << EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn

[dn]
C = US
ST = State
L = City
O = Tragge
OU = Application
CN = tragge_app
EOF

# Generate client CSR
openssl req -new \
    -key "$OUTPUT_DIR/client.key" \
    -out "$OUTPUT_DIR/client.csr" \
    -config "$OUTPUT_DIR/client.cnf"

if [[ "$PRODUCTION" != true ]]; then
    # Self-sign client certificate with CA
    openssl x509 -req \
        -in "$OUTPUT_DIR/client.csr" \
        -CA "$OUTPUT_DIR/ca.crt" \
        -CAkey "$OUTPUT_DIR/ca.key" \
        -CAcreateserial \
        -out "$OUTPUT_DIR/client.crt" \
        -days "$VALIDITY_DAYS"

    echo -e "${GREEN}[CREATED]${NC} client.key, client.crt (self-signed)"
fi

chmod 600 "$OUTPUT_DIR/client.key"

# ==============================================================================
# Create Kubernetes Secret Manifest (optional)
# ==============================================================================
echo -e "${BLUE}Creating Kubernetes secret manifest...${NC}"

K8S_SECRET_FILE="$OUTPUT_DIR/postgres-ssl-secret.yaml"

# Base64 encode certificates
CA_CRT_B64=$(base64 -w0 < "$OUTPUT_DIR/ca.crt")
SERVER_CRT_B64=$(base64 -w0 < "$OUTPUT_DIR/server.crt" 2>/dev/null || echo "")
SERVER_KEY_B64=$(base64 -w0 < "$OUTPUT_DIR/server.key")

if [[ -n "$SERVER_CRT_B64" ]]; then
    cat > "$K8S_SECRET_FILE" << EOF
# Generated by scripts/secrets/generate-ssl-certs.sh
# WARNING: Contains private keys - do not commit to version control!
apiVersion: v1
kind: Secret
metadata:
  name: postgres-ssl-certs
  namespace: tragge
  labels:
    app.kubernetes.io/name: postgres
    app.kubernetes.io/part-of: tragge-platform
type: Opaque
data:
  ca.crt: $CA_CRT_B64
  server.crt: $SERVER_CRT_B64
  server.key: $SERVER_KEY_B64
EOF
    echo -e "${GREEN}[CREATED]${NC} postgres-ssl-secret.yaml"
else
    echo -e "${YELLOW}[SKIPPED]${NC} postgres-ssl-secret.yaml (server.crt not available)"
fi

# ==============================================================================
# Cleanup temporary files
# ==============================================================================
rm -f "$OUTPUT_DIR/server.cnf" "$OUTPUT_DIR/client.cnf"
rm -f "$OUTPUT_DIR/server.csr" "$OUTPUT_DIR/client.csr"
rm -f "$OUTPUT_DIR/ca.srl"

echo ""
echo "=============================================="
echo "    Certificate Generation Complete"
echo "=============================================="
echo ""
echo "Generated files in $OUTPUT_DIR:"
echo "  - ca.crt         CA certificate (distribute to clients)"
echo "  - ca.key         CA private key (keep secure!)"
echo "  - server.crt     Server certificate"
echo "  - server.key     Server private key"
echo "  - client.crt     Client certificate (optional)"
echo "  - client.key     Client private key (optional)"
echo ""
echo -e "${YELLOW}NEXT STEPS:${NC}"
echo ""
echo "1. For Docker Compose:"
echo "   Add to docker-compose.yml volumes:"
echo "     - ./ssl/server.crt:/var/lib/postgresql/ssl/server.crt:ro"
echo "     - ./ssl/server.key:/var/lib/postgresql/ssl/server.key:ro"
echo "     - ./ssl/ca.crt:/var/lib/postgresql/ssl/ca.crt:ro"
echo ""
echo "2. For Kubernetes:"
echo "   kubectl apply -f $OUTPUT_DIR/postgres-ssl-secret.yaml"
echo ""
echo "3. Update PostgreSQL configuration:"
echo "   ssl = on"
echo "   ssl_cert_file = '/var/lib/postgresql/ssl/server.crt'"
echo "   ssl_key_file = '/var/lib/postgresql/ssl/server.key'"
echo "   ssl_ca_file = '/var/lib/postgresql/ssl/ca.crt'"
echo ""
echo "4. Update client connection strings:"
echo "   sslmode=verify-full"
echo "   sslrootcert=/path/to/ca.crt"
echo ""
echo -e "${RED}SECURITY REMINDER:${NC}"
echo "- Keep ca.key and server.key files secure"
echo "- Never commit private keys to version control"
echo "- Rotate certificates before expiration"
echo ""
