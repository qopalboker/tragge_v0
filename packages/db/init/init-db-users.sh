#!/bin/bash
# ==============================================================================
# Initialize PostgreSQL Database Users
# ==============================================================================
# DISABLED: User creation is handled by 01-create-users.sql
# Passwords are managed via Docker secrets directly in docker-compose.yml
#
# This script is kept for manual execution if needed:
#   ./init-db-users.sh
# ==============================================================================

echo "User initialization skipped - handled by SQL init scripts"
exit 0