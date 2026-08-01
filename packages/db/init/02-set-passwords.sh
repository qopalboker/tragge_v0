#!/bin/bash
# ==============================================================================
# Set PostgreSQL User Passwords from Docker Secrets
# ==============================================================================
# Runs after 01-create-users.sql to assign passwords from mounted secrets.
# Docker-entrypoint sorts init files alphabetically, so 02-* runs after 01-*.
# ==============================================================================

set -e

echo "Setting user passwords from Docker secrets..."

# Helper: read password from secret file
read_secret() {
    local file="$1"
    if [ -f "$file" ]; then
        cat "$file" | tr -d '\n'
    else
        echo ""
    fi
}

# Read passwords from Docker secrets
ADMIN_PASS=$(read_secret /run/secrets/postgres_admin_password)
APP_PASS=$(read_secret /run/secrets/postgres_app_password)
READONLY_PASS=$(read_secret /run/secrets/postgres_readonly_password)
PGBOUNCER_PASS=$(read_secret /run/secrets/pgbouncer_auth_password)

# Set passwords using psql (running as postgres superuser during init)
if [ -n "$ADMIN_PASS" ]; then
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
        -c "ALTER USER tragge_admin WITH PASSWORD '$ADMIN_PASS';"
    echo "  Password set for: tragge_admin"
fi

if [ -n "$APP_PASS" ]; then
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
        -c "ALTER USER tragge_app WITH PASSWORD '$APP_PASS';"
    echo "  Password set for: tragge_app"
fi

if [ -n "$READONLY_PASS" ]; then
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
        -c "ALTER USER tragge_readonly WITH PASSWORD '$READONLY_PASS';"
    echo "  Password set for: tragge_readonly"
fi

if [ -n "$PGBOUNCER_PASS" ]; then
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
        -c "ALTER USER pgbouncer_auth WITH PASSWORD '$PGBOUNCER_PASS';"
    echo "  Password set for: pgbouncer_auth"
fi

echo "Password initialization complete."