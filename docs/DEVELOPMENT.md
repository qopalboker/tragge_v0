# Development Guide

This guide covers everything needed to set up and run the tragge Trading Tournament Platform locally.

## Prerequisites

Install the following before proceeding:

| Tool | Version | Purpose |
|------|---------|---------|
| **Docker** | 24+ with Compose v2 | Infrastructure services (PostgreSQL, Redis, Redpanda) |
| **Go** | 1.24+ | Backend services |
| **Node.js** | 18+ | Frontend builds |
| **pnpm** | 8+ | Frontend package management |
| **golang-migrate** | latest | Database migrations |

### Installing prerequisites

**Go** - https://go.dev/dl/

**Node.js** - https://nodejs.org/ (LTS recommended)

**pnpm:**
```bash
npm install -g pnpm@8
```

**golang-migrate CLI:**
```bash
# macOS
brew install golang-migrate

# Linux (binary)
curl -L https://github.com/golang-migrate/migrate/releases/latest/download/migrate.linux-amd64.tar.gz | tar xvz
sudo mv migrate /usr/local/bin/

# Or via Go
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

## Environment Setup

### 1. Copy the environment file

```bash
cp .env.example .env
```

The defaults in `.env.example` are sufficient for local development. Review and adjust if needed (e.g., ports, market data provider settings).

### 2. Initialize Docker secrets

The platform uses Docker secrets for sensitive values (database passwords, JWT secret, API keys). Run the initialization scripts to generate them:

```bash
# Generate base secrets (JWT secret, API key placeholders, etc.)
./scripts/secrets/init-secrets.sh

# Generate secure database credentials (admin, app, readonly, pgbouncer users)
./scripts/secrets/generate-db-credentials.sh
```

This creates secret files in `infra/docker/secrets/` with auto-generated passwords. The files are gitignored.

### 3. Configure API keys (optional)

If you need live market data, update these files with real API keys:

```
infra/docker/secrets/twelvedata_api_keys.txt
infra/docker/secrets/massive_api_keys.txt
```

Other optional secrets for notifications and OAuth:
```
infra/docker/secrets/resend_api_key.txt
infra/docker/secrets/discord_webhook_url.txt
infra/docker/secrets/google_client_id.txt
infra/docker/secrets/google_client_secret.txt
```

### 4. Install frontend dependencies

```bash
make install
```

### 5. Sync Go workspace

```bash
make go-sync
```

## Starting Infrastructure

Start all infrastructure services (PostgreSQL, Redis, Redpanda, monitoring stack):

```bash
docker compose -f infra/docker/docker-compose.yml up -d --build
```

Or using the Makefile shortcut:

```bash
make up
```

Verify services are running:

```bash
make ps
```

Expected services:

| Service | Port | Health Check |
|---------|------|-------------|
| PostgreSQL | 5432 | `pg_isready` |
| Redis | 6379 | `redis-cli ping` |
| Redpanda (Kafka) | 9092 | `rpk cluster health` |
| Redpanda Console | 8089 | Web UI |
| Prometheus | 9090 | Web UI |
| Grafana | 3000 | Web UI (admin/admin) |
| Loki | 3100 | API |
| Tempo | 3200 | API |

## Database Initialization

Once PostgreSQL is running, apply all migrations:

```bash
make migrate-up
```

This runs all migration files from `packages/db/migrations/` (currently 0001 through 0035). The init scripts in `packages/db/init/` are automatically executed by PostgreSQL on first start to create the role-based database users.

### Verify migration version

```bash
make migrate-version
```

### Rollback the last migration

```bash
make migrate-down
```

### Create a new migration

```bash
make migrate-create NAME=your_migration_name
```

## Running Services

### All services (Docker)

To build and run all services in Docker:

```bash
docker compose -f infra/docker/docker-compose.yml up -d --build
```

### Individual services (local development)

For active development, run services locally against the Dockerized infrastructure:

```bash
# Backend services
make dev-user-bff           # port 8081
make dev-trade-bff          # port 8082
make dev-admin-bff          # port 8083
make dev-trading-engine     # port 8084
make dev-market-ingestor    # port 8085
make dev-leaderboard-worker # port 8086

# Frontend dev server (unified SPA on port 5173, serves /user, /trade, /admin)
make dev-frontend
```

## Building

```bash
make build            # Build everything
make build-go         # Build all Go services
make build-frontends  # Build all Vue frontends
make build-gateway    # Build the nginx gateway Docker image
```

## Testing

```bash
make test             # Run all tests
make test-go          # Go tests only
make test-frontends   # Frontend tests only
```

### Integration tests

Integration tests use testcontainers to spin up isolated PostgreSQL, Redis, and Redpanda instances:

```bash
cd tests/integration && go test -v ./...
```

### E2E tests (Playwright)

```bash
make e2e-install      # Install Playwright browsers (first time)
make e2e              # Run all E2E tests
make e2e-ui           # Interactive UI mode
```

## Linting

```bash
make lint             # Lint everything
make lint-go          # Go only (requires golangci-lint)
make lint-go-fix      # Go with auto-fix
make lint-frontends   # Frontend only
```

## Common Troubleshooting

### Docker secrets missing

**Symptom:** `docker compose up` fails with errors about missing secret files.

**Fix:** Run the secret initialization scripts:
```bash
./scripts/secrets/init-secrets.sh
./scripts/secrets/generate-db-credentials.sh
```

### PostgreSQL won't start or authentication fails

**Symptom:** `FATAL: password authentication failed` or services can't connect to the database.

**Fix:**
1. Ensure secrets were generated: check that `infra/docker/secrets/postgres_admin_password.txt` exists and is non-empty.
2. If you changed passwords after initial setup, you need to recreate the database volume:
   ```bash
   docker compose -f infra/docker/docker-compose.yml down -v
   ./scripts/secrets/generate-db-credentials.sh --force
   docker compose -f infra/docker/docker-compose.yml up -d --build
   make migrate-up
   ```

### Migrations fail with "dirty database"

**Symptom:** `migrate: Dirty database version X. Fix and force version.`

**Fix:** A previous migration failed partway through. Force the version back:
```bash
migrate -path packages/db/migrations \
  -database "postgres://tragge_admin:<password>@localhost:5432/app?sslmode=disable" \
  force <last_clean_version>
```
Then re-run `make migrate-up`.

### Redpanda / Kafka connection refused

**Symptom:** Services fail to connect to Kafka on port 9092.

**Fix:**
1. Check that Redpanda is healthy: `docker inspect tragge_redpanda | grep -A5 Health`
2. Redpanda maps external port 9092 to internal port 19092. Services running inside Docker should use `redpanda:9092`. Services running on the host should use `localhost:9092`.
3. If Redpanda takes too long to start, restart dependent services after it's healthy.

### Docker image pull failures / TLS timeouts

**Symptom:** `docker compose up` fails with TLS handshake timeouts or image pull errors.

**Fix:**
```bash
# Pre-pull images with retry logic
make pull-images

# Or configure Docker registry mirrors (requires sudo)
make fix-docker-tls
```

### Port conflicts

**Symptom:** `address already in use` when starting services.

**Fix:** Check which process is using the port:
```bash
lsof -i :<port>
# or
ss -tlnp | grep <port>
```

Default ports: PostgreSQL 5432, Redis 6379, Redpanda Kafka 9092, Redpanda Console 8089, Grafana 3000, Prometheus 9090. BFF services use 8081-8086. The unified frontend dev server uses 5173.

### Go workspace out of sync

**Symptom:** Import errors or `go build` failures after pulling changes.

**Fix:**
```bash
make go-sync
```

### Frontend dependency issues

**Symptom:** Build failures or missing modules after pulling changes.

**Fix:**
```bash
make install   # Re-run pnpm install
```

### Services can't find environment variables

**Symptom:** Services crash on startup with missing config errors.

**Fix:** Ensure `.env` exists at the project root:
```bash
cp .env.example .env
```

For services running locally (not in Docker), you may need to export variables or use a tool like `direnv`.

## Stopping Everything

```bash
make down
```

To also remove volumes (database data, Redpanda data, etc.):

```bash
docker compose -f infra/docker/docker-compose.yml down -v
```
