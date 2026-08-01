# tragge

Trading Tournament Platform - A monorepo for trading competition services.

## Production readiness

**Current paid-production status: NO-GO.** The reproducible repository
inventory, current runtime topology, test gaps, toolchain baseline, and every
known P0/P1 finding are recorded in the
[current-state production audit](docs/architecture/current-state-audit.md).
This status must not be interpreted as a test-suite or deployment pass.

## Repository Structure

```
tragge/
├── apps/                        # Applications
│   ├── user-frontend/          # User panel SPA (Vue 3 + Vite, port 5173)
│   ├── admin-frontend/         # Admin panel SPA (Vue 3 + Vite, port 5174)
│   ├── user-bff/               # User Backend-for-Frontend (Go)
│   ├── trade-bff/              # Trade Backend-for-Frontend (Go)
│   ├── admin-bff/              # Admin Backend-for-Frontend (Go)
│   ├── api-server/             # Merged user-bff + admin-bff + payment-service
│   ├── market-ingestor/        # Market data ingestion service (Go)
│   ├── trading-engine/         # Core trading engine (Go)
│   ├── trading-core/           # Merged trading-engine + market-ingestor + trade-bff
│   ├── leaderboard-worker/     # Leaderboard computation worker (Go)
│   └── gateway/                # Nginx gateway — :8080 user panel, :8081 admin
├── packages/                    # Shared packages
│   ├── contracts/              # Shared event contracts (Go + TS)
│   ├── auth/                   # Authentication utilities (Go)
│   ├── frontend-shared/        # Shared Vue primitives (auth bootstrap, API client, i18n)
│   ├── observability/          # Logging, metrics, tracing (Go)
│   ├── db/                     # Database utilities (Go)
│   └── ...                     # See packages/ directory for full list
├── infra/                       # Infrastructure
│   └── docker/                 # Docker configurations
├── go.work                      # Go workspace configuration
├── pnpm-workspace.yaml          # pnpm workspace configuration
└── package.json                 # Root package.json
```

## Panel split

Since the April 2026 split the user and admin panels are fully
separated:

- **User panel** — `apps/user-frontend`, served on gateway :8080, talks
  to `/api/user/*`, `/api/trade/*`, `/api/payments/*`, `/api/wallet/*`,
  `/ws/trade`, `/ws/tournaments`. Access and refresh tokens use separate
  User keys, issuer `tragge-user-auth`, audience `user`, and context `user`.
  Cookies are `refresh_token_user` and `tragge_session_hint_user`; Redis
  session and revocation keys use explicit User namespaces.

- **Admin panel** — `apps/admin-frontend`, served on gateway :8081,
  talks to `/api/admin/*` only. Access and refresh tokens use separate
  Admin keys, issuer `tragge-admin-auth`, audience `admin`, and context
  `admin`. Cookies are `refresh_token_admin` and
  `tragge_session_hint_admin`; Redis session and revocation keys use
  explicit Admin namespaces.

The merged API constructs two authentication objects and rejects a token
from the other trust domain before role authorization. The complete
configuration and operational contract is in the
[User/Admin authentication isolation guide](docs/security/user-admin-authentication-isolation.md).
## Prerequisites

- Go 1.24.7
- Node.js 20.19.0 for the repeatable baseline; supported alternatives are
  Node.js 22.13+ or 24+
- pnpm 8.15.0
- Docker and Docker Compose for dependency-backed integration work
- Optional: a Make implementation for the convenience targets

The local baseline is declared in [`.tool-versions`](.tool-versions). CI
currently selects compatible Go 1.24, Node 20, and pnpm 8 major lines rather
than exact patches; see the audit for this known reproducibility gap.

## Production baseline evidence

The inventory and verifier use Node built-ins only and do not install
dependencies:

```bash
pnpm baseline:inventory
pnpm test:baseline
pnpm baseline:verify
```

Without pnpm or Make, run the underlying commands directly:

```bash
node scripts/production-baseline.mjs inventory
node scripts/production-baseline.test.mjs
node scripts/production-baseline.mjs verify
```

## Getting Started

### Install Dependencies

```bash
# Install frontend dependencies
make install

# Or manually
pnpm install
```

### Development

```bash
# Start all services (placeholder)
make dev

# Start the user panel (port 5173)
make dev-user-frontend

# Start the admin panel (port 5174)
make dev-admin-frontend

# Start specific Go service
make dev-gateway
make dev-user-bff
make dev-trade-bff
make dev-admin-bff
make dev-trading-engine
make dev-market-ingestor
make dev-leaderboard-worker
```

### Build

```bash
# Build all
make build

# Build frontends only
make build-frontends

# Build Go services only
make build-go
```

### Test

```bash
# Run all tests
make test

# Run Go tests
make test-go

# Run frontend tests
make test-frontends
```

### Lint

```bash
# Lint all
make lint

# Lint Go code
make lint-go

# Lint frontends
make lint-frontends
```

### Go Workspace

```bash
# Sync Go workspace
make go-sync

# Or manually
go work sync
```

## Development Workflow

1. Clone the repository
2. Run `make install` to install dependencies
3. Run `make dev` to start development servers
4. Make changes and run `make test` before committing

## License

MIT
