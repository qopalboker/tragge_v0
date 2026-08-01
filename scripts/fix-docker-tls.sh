#!/bin/bash
# Script to fix Docker TLS timeout issues by adding Iranian registry mirrors
# Run this script with sudo: sudo ./fix-docker-tls.sh

set -e

DAEMON_JSON="/etc/docker/daemon.json"

echo "🔧 Fixing Docker TLS timeout by adding registry mirrors..."

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   echo "❌ This script must be run as root (use sudo)"
   exit 1
fi

# Create /etc/docker directory if it doesn't exist
mkdir -p /etc/docker

# Check if daemon.json exists and has content
if [[ -f "$DAEMON_JSON" && -s "$DAEMON_JSON" ]]; then
    echo "📝 Existing daemon.json found. Creating backup..."
    cp "$DAEMON_JSON" "${DAEMON_JSON}.backup.$(date +%Y%m%d%H%M%S)"

    # Check if it already has registry-mirrors
    if grep -q "registry-mirrors" "$DAEMON_JSON"; then
        echo "⚠️  registry-mirrors already configured in daemon.json"
        echo "Current content:"
        cat "$DAEMON_JSON"
        echo ""
        read -p "Do you want to overwrite? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            echo "Aborted."
            exit 0
        fi
    fi

    # Try to merge with existing config using jq if available
    if command -v jq &> /dev/null; then
        echo "📦 Merging with existing configuration..."
        jq '. + {"registry-mirrors": ["https://docker.arvancloud.ir", "https://registry.docker.ir"]}' "$DAEMON_JSON" > "${DAEMON_JSON}.tmp"
        mv "${DAEMON_JSON}.tmp" "$DAEMON_JSON"
    else
        # Overwrite if jq not available
        echo "⚠️  jq not found. Overwriting daemon.json..."
        cat > "$DAEMON_JSON" << 'EOF'
{
  "registry-mirrors": [
    "https://docker.arvancloud.ir",
    "https://registry.docker.ir"
  ]
}
EOF
    fi
else
    # Create new daemon.json
    echo "📝 Creating new daemon.json..."
    cat > "$DAEMON_JSON" << 'EOF'
{
  "registry-mirrors": [
    "https://docker.arvancloud.ir",
    "https://registry.docker.ir"
  ]
}
EOF
fi

echo "✅ daemon.json configured:"
cat "$DAEMON_JSON"

# Restart Docker
echo ""
echo "🔄 Restarting Docker daemon..."
if command -v systemctl &> /dev/null; then
    systemctl restart docker
elif command -v service &> /dev/null; then
    service docker restart
else
    echo "⚠️  Could not detect init system. Please restart Docker manually:"
    echo "   - For systemd: sudo systemctl restart docker"
    echo "   - For sysvinit: sudo service docker restart"
    exit 1
fi

echo ""
echo "✅ Docker configured successfully!"
echo ""
echo "🧪 Testing Docker pull..."
if docker pull hello-world &> /dev/null; then
    echo "✅ Docker pull test successful!"
else
    echo "⚠️  Docker pull test failed. Mirrors might be slow or unavailable."
    echo "   Try running your build again - it may take a few attempts."
fi

echo ""
echo "You can now retry your docker build command."
