# Database Package

This package contains database migrations and utilities for the tragge trading platform.
## Legacy chain and target foundation

The 99 top-level migration pairs reproduce the disposable pre-launch schema (98 from FND-004 plus the SEC-004 canonical-role bridge).
They are legacy evidence, not the approved target database. Their complete
classification is in the
[migration inventory](../../docs/architecture/migration-inventory.md).

The isolated `migrations/target` chain establishes only the Platform, Trading
Engine, and Market Data schema/role boundary approved by ADR-0001. It does not
yet contain domain tables and must not be mixed with the top-level chain. The
[reset strategy](../../docs/architecture/database-migration-reset-strategy.md)
defines guards, migration policy, future task ownership, and the exact
fresh-install command.

Static/dry-run validation does not require PostgreSQL:

```bash
TRAGGE_TARGET_DATABASE_URL='postgresql://local_admin@localhost/tragge_fnd004_test?sslmode=disable' \
  node scripts/database-reset.mjs --dry-run --environment test
node scripts/database-migration-reset.test.mjs
```

Destructive execution additionally requires the exact database confirmation,
the destructive-confirmation environment value, an approved database name,
and a positively identified non-production connection. Do not use the legacy
`make migrate-up` command for a clean target database.

## Prerequisites

- Docker and Docker Compose (for local development)
- [golang-migrate](https://github.com/golang-migrate/migrate) CLI tool

### Installing golang-migrate

**macOS (Homebrew):**
```bash
brew install golang-migrate
```

**Linux:**
```bash
curl -L https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | tar xvz
sudo mv migrate /usr/local/bin/
```

**Go Install:**
```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

## Running Migrations

### Development (Docker)

1. Start the infrastructure:
   ```bash
   # From repository root
   make up
   ```

2. Run migrations:
   ```bash
   # Apply all pending migrations
   make migrate-up

   # Rollback the last migration
   make migrate-down
   ```

### Connection Details

Default development database connection:
- **Host:** localhost
- **Port:** 5432
- **Database:** app
- **User:** app
- **Password:** app

Connection string:
```
postgres://app:app@localhost:5432/app?sslmode=disable
```

### Manual Migration Commands

If you need to run migrations manually:

```bash
# Apply all migrations
migrate -path packages/db/migrations \
        -database "postgres://app:app@localhost:5432/app?sslmode=disable" \
        up

# Rollback last migration
migrate -path packages/db/migrations \
        -database "postgres://app:app@localhost:5432/app?sslmode=disable" \
        down 1

# Go to a specific version
migrate -path packages/db/migrations \
        -database "postgres://app:app@localhost:5432/app?sslmode=disable" \
        goto 1

# Check current version
migrate -path packages/db/migrations \
        -database "postgres://app:app@localhost:5432/app?sslmode=disable" \
        version

# Force set version (use with caution)
migrate -path packages/db/migrations \
        -database "postgres://app:app@localhost:5432/app?sslmode=disable" \
        force 1
```

## Creating New Migrations

Use the migrate CLI to create new migration files:

```bash
migrate create -ext sql -dir packages/db/migrations -seq <migration_name>
```

This creates two files:
- `NNNN_<migration_name>.up.sql` - Apply changes
- `NNNN_<migration_name>.down.sql` - Rollback changes

### Migration Guidelines

1. **Always create both up and down migrations**
2. **Test rollbacks locally before committing**
3. **Keep migrations small and focused**
4. **Never modify existing migrations that have been deployed**
5. **Use transactions when possible** (PostgreSQL auto-wraps DDL in transactions)

## Schema Overview

The initial migration (`0001_init`) creates the following tables:

### Users & Authentication
- `users` - User accounts (id, email, password_hash)
- `roles` - Available roles (user, admin, moderator)
- `user_roles` - User-to-role assignments

### Contests
- `contests` - Trading contests/tournaments
- `contest_symbols` - Tradeable symbols per contest
- `contest_participants` - User participation in contests

### Trading
- `orders` - Trading orders (market, limit, stop, stop_limit)
- `fills` - Order execution records
- `positions` - Open and closed positions

### Analytics & Audit
- `leaderboard_snapshots` - Point-in-time leaderboard data
- `audit_logs` - System-wide audit trail

## Troubleshooting

### Migration is in a dirty state

If a migration fails partway through, you may need to fix the dirty state:

```bash
# Check current state
migrate -path packages/db/migrations \
        -database "postgres://app:app@localhost:5432/app?sslmode=disable" \
        version

# Force to a clean version (after manually fixing the database)
migrate -path packages/db/migrations \
        -database "postgres://app:app@localhost:5432/app?sslmode=disable" \
        force <version>
```

### Reset database completely

The old volume-removal sequence was unguarded and applied the legacy chain. It
is intentionally no longer an approved reset. Use the guarded command in the
reset-strategy document. It refuses production, unknown hosts, unapproved
database names, conflicting environment signals, and missing confirmation.
FND-004 did not execute a real reset; the later
[Phase 0 PostgreSQL remediation report](../../docs/codex/reports/phase-0-postgresql-fresh-install-remediation.md)
records the isolated PostgreSQL execution evidence.
