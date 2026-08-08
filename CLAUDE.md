# CLAUDE.md

This file provides guidance for AI assistants working with the tragge repository.

## Project Overview

**tragge** is a Trading Tournament Platform structured as a monorepo with an event-driven architecture.

### Current State

- **Status**: Production-ready platform with 11 Go services, 3 Vue frontends, and full infrastructure
- **Structure**: Monorepo with Go workspaces and pnpm workspaces
- **Backend**: Go 1.24+ services with BFF (Backend-for-Frontend) pattern
- **Frontend**: Vue 3 + Vite + TypeScript with i18n support (English/Farsi)
- **Messaging**: Redpanda (Kafka-compatible) for event-driven communication
- **Database**: PostgreSQL 16 with comprehensive schema (68 migration pairs, 30+ tables)
- **Observability**: Full monitoring stack (Prometheus, Grafana, Loki, Tempo, Alertmanager)
- **Deployment**: Docker Compose (dev) and Kubernetes (production/staging)
- **Codebase**: ~270 Go files, ~185 Vue components, comprehensive integration tests
- **CI/CD**: GitHub Actions pipeline (lint, test, build)
- **Secrets**: Docker secrets (dev) and external secrets managers (production)

### What's Implemented

**Go Services (Fully Operational):**
- `user-bff` - User registration, login, JWT authentication, OAuth (Google), password reset, email verification, profile/avatar management, tournament listing
- `trade-bff` - WebSocket trading interface with real-time updates, compression, tournament feed, contest event subscriptions
- `admin-bff` - Contest management, audit logging, role-based access, tournament templates/schedules, market hours, spread config, calendar, email template versioning
- `market-ingestor` - Multi-provider market data (Massive primary, TwelveData fallback, Nobitex for crypto), candle aggregation, spread management
- `trading-engine` - Order processing, pending orders, TP/SL support, sharded consumption, WAL, position locking, decimal scoring, rate limiting
- `leaderboard-worker` - Leaderboard calculation, payout processing, contest finalization, sharded leaderboards, notification consumer
- `payment-service` - Deposit/withdrawal processing (Jibit, NowPayments), webhook handling, KYC, exchange rates, expiry worker, inquiry worker, cleanup
- `settlement-service` - Contest settlement, prize distribution, wallet integration, stuck settlement detection
- `shard-router` - Request routing with circuit breakers, rate limiting, caching, alerting
- `free-contest-generator` - Automated free practice contest generation on schedule
- `contest-scheduler` - Contest lifecycle state machine (scheduled→running→completed), distributed locking

**Packages (24 shared packages):**
- `auth` - Full authentication suite (Argon2id hashing, JWT with separate refresh secrets, middleware, sessions, RBAC)
- `circuitbreaker` - Circuit breaker pattern implementation
- `config` - Configuration loading and environment variable validation
- `contracts` - 20 versioned event types (Go + TypeScript + JSON schemas)
- `db` - Database migrations and utilities (68 migration pairs)
- `exchangerate` - Currency exchange rate service
- `health` - Health check utilities
- `inapp` - In-app notification support with mark-as-read
- `kyc` - KYC verification service (Jibit identity provider)
- `notification` - Email (Resend) and Discord notification service with async queue, cleanup, circuit breaker
- `observability` - Structured logging (zap), Prometheus metrics, OpenTelemetry tracing
- `prize` - Prize distribution logic with commission handling
- `ratelimit` - Rate limiting middleware
- `redis` - Redis client wrapper (standalone, sentinel, cluster modes)
- `resilience` - Resilience patterns (retry, fallback)
- `scoring` - Scoring calculation with decimal precision
- `secrets` - Centralized secrets management (Docker secrets + env fallback)
- `sms` - SMS provider abstraction (KaveNegar), OTP generation, Redis-backed verification with rate limiting
- `shard` - Sharding utilities
- `statemachine` - State machine implementation for contest lifecycle
- `storage` - S3/MinIO storage abstraction for avatars and KYC documents

- `validation` - Input validation, sanitization, HTTP middleware
- `wallet` - Wallet/balance management with idempotency and ledger
- `wsregistry` - WebSocket registry

**Infrastructure:**
- Docker Compose with full monitoring stack (Prometheus, Grafana, Loki, Tempo, Alertmanager)
- Redis High Availability (Sentinel and Cluster modes)
- PgBouncer connection pooling
- Kubernetes manifests with Kustomize overlays (production + staging)
- HPA, PDB, Network Policies, TLS certificates
- Automated backup system (PostgreSQL and Redis to S3)
- Complete Makefile build automation
- Integration, contract, E2E, and manual test suites
- 9 load testing and chaos engineering tools

**Operations:**
- Incident response runbook
- Database recovery and HA procedures
- Credential rotation procedures
- Scaling guide and deployment procedures
- Rollback procedures
- Redis HA and PostgreSQL HA runbooks
- Circuit breaker troubleshooting

## Repository Structure

```
tragge/
├── apps/                           # Applications
│   ├── user-frontend/              # User panel SPA (port 5173, aud=user)
│   │   ├── src/
│   │   │   ├── modules/user/       # User module (auth, dashboard, contests, profile, wallet)
│   │   │   ├── modules/trade/      # Trade module (real-time trading interface)
│   │   │   ├── stores/             # auth.ts (user-only surface), theme, i18n re-exports
│   │   │   └── i18n/               # i18n (English + Farsi)
│   │   └── e2e/                    # Playwright specs — user/trade suites
│   ├── admin-frontend/             # Admin panel SPA (port 5174, aud=admin)
│   │   ├── src/
│   │   │   ├── modules/admin/      # Admin module (contest + user management, tickets, KYC)
│   │   │   ├── stores/             # auth.ts (admin-only surface), theme, i18n re-exports
│   │   │   └── i18n/               # i18n (English + Farsi)
│   │   └── e2e/                    # Playwright specs — audit/contests/shards
│   ├── user-bff/                   # User Backend-for-Frontend (Go)
│   │   ├── main.go                 # Entry point + handlers
│   │   ├── internal/               # Internal packages
│   │   ├── tournament_handlers.go  # Tournament listing/calendar
│   │   ├── contest_prizes.go       # Prize distribution
│   │   ├── encryption.go           # Encryption utilities
│   │   ├── circuits.go             # Circuit breaker config
│   │   └── Dockerfile
│   ├── trade-bff/                  # Trade Backend-for-Frontend (Go)
│   │   ├── main.go                 # Entry point + WebSocket handlers
│   │   ├── batcher.go              # Message batching
│   │   ├── leaderboard.go          # Leaderboard broadcast
│   │   ├── tournament_feed.go      # Tournament feed WebSocket
│   │   ├── market_status.go        # Market hours/status
│   │   ├── circuits.go             # Circuit breaker config
│   │   ├── alerts.go               # Alert definitions
│   │   └── Dockerfile
│   ├── admin-bff/                  # Admin Backend-for-Frontend (Go)
│   │   ├── main.go                 # Entry point + core handlers
│   │   ├── handlers_*.go           # Modular handler files
│   │   │   ├── handlers_calendar.go
│   │   │   ├── handlers_email_versions.go
│   │   │   ├── handlers_market.go
│   │   │   ├── handlers_market_hours.go
│   │   │   ├── handlers_schedules.go
│   │   │   ├── handlers_spreads.go
│   │   │   ├── handlers_statemachine.go
│   │   │   ├── handlers_templates.go
│   │   │   └── handlers_tournament_stats.go
│   │   ├── circuits.go             # Circuit breaker config
│   │   ├── market_hours.json       # Market hours data
│   │   └── Dockerfile
│   ├── market-ingestor/            # Market data ingestion service (Go)
│   │   ├── main.go                 # Entry point
│   │   ├── nobitex_provider.go     # Nobitex crypto provider
│   │   ├── candle_aggregator.go    # OHLCV candle aggregation
│   │   ├── spread_manager.go       # Spread configuration
│   │   ├── symbol_registry.go      # Symbol management
│   │   ├── alerts.go               # Alert definitions
│   │   └── Dockerfile
│   ├── trading-engine/             # Core trading engine (Go)
│   │   ├── main.go                 # Entry point
│   │   ├── engine.go               # Core engine logic
│   │   ├── state.go                # State management
│   │   ├── state_sharded.go        # Sharded state
│   │   ├── pending.go              # Pending order evaluation
│   │   ├── pricebook.go            # Price book with bid/ask
│   │   ├── decimal_scoring.go      # Decimal precision scoring
│   │   ├── wal.go                  # Write-ahead log
│   │   ├── consumer_sharded.go     # Sharded Kafka consumer
│   │   ├── contest_cache.go        # Contest config cache
│   │   ├── position_lock.go        # Position locking
│   │   ├── rate_limiter.go         # Per-user rate limiting
│   │   ├── market_hours.go         # Market hours logic
│   │   ├── price_freshness_monitor.go # Price staleness detection
│   │   ├── alerts.go / circuits.go
│   │   └── Dockerfile
│   ├── leaderboard-worker/         # Leaderboard computation worker
│   │   ├── main.go                 # Entry point, Kafka consumer
│   │   ├── leaderboard.go          # Redis sorted set operations
│   │   ├── leaderboard_sharded.go  # Sharded leaderboard
│   │   ├── enhanced_leaderboard.go # Enhanced leaderboard features
│   │   ├── payout.go               # Prize pool distribution logic
│   │   ├── finalize.go             # Contest finalization logic
│   │   ├── notification_consumer.go # Notification event consumer
│   │   ├── config.go               # Configuration management
│   │   ├── alerts.go / circuits.go
│   │   ├── prize_distribution.json # Prize distribution config
│   │   └── Dockerfile
│   ├── payment-service/            # Payment processing service (Go)
│   │   ├── main.go                 # Entry point
│   │   ├── handlers/               # Deposit, withdraw, webhook, history
│   │   ├── providers/              # Jibit, NowPayments integrations
│   │   ├── expiry.go               # Payment expiry worker
│   │   ├── inquiry.go              # Jibit inquiry worker
│   │   ├── cleanup.go              # Cleanup logic
│   │   ├── circuits.go             # Circuit breaker config
│   │   ├── config.go               # Configuration
│   │   └── Dockerfile
│   ├── settlement-service/         # Contest settlement service (Go)
│   │   ├── main.go                 # Entry point
│   │   ├── settlement.go           # Settlement logic
│   │   ├── stuck_detector.go       # Stuck settlement detection
│   │   ├── db.go                   # Database operations
│   │   ├── config.go               # Configuration
│   │   └── Dockerfile
│   ├── shard-router/               # Request routing service (Go)
│   │   ├── main.go                 # Entry point
│   │   ├── router.go               # Routing logic
│   │   ├── handlers.go             # HTTP handlers
│   │   ├── cache.go                # Response caching
│   │   ├── alerts.go / circuits.go
│   │   ├── config.go               # Configuration
│   │   └── Dockerfile
│   ├── free-contest-generator/     # Free contest generator (Go)
│   │   ├── main.go                 # Entry point + generator logic
│   │   └── Dockerfile
│   ├── contest-scheduler/          # Contest lifecycle scheduler (Go)
│   │   ├── main.go                 # Entry point
│   │   ├── internal/               # Scheduler, health check
│   │   ├── config.go               # Configuration
│   │   └── Dockerfile

│   └── gateway/                    # Nginx reverse proxy + API gateway
│       ├── Dockerfile              # Dev container
│       ├── Dockerfile.prod         # Production container
│       ├── nginx.conf              # Dev routing configuration
│       ├── nginx.prod.conf         # Production routing configuration
│       ├── includes/               # Nginx include files
│       │   ├── security-headers.conf
│       │   └── cors-error-headers.conf
│       └── avatars/                # Default avatar images
├── packages/                       # Shared packages (24 packages)
│   ├── auth/                       # Authentication utilities (Go)
│   │   ├── auth.go                 # Password hashing (Argon2id)
│   │   ├── jwt.go                  # JWT token service
│   │   ├── middleware.go           # HTTP middleware with RBAC
│   │   ├── session.go              # Redis session management
│   │   └── password.go             # Password utilities
│   ├── circuitbreaker/             # Circuit breaker pattern
│   ├── config/                     # Configuration & env validation
│   ├── contracts/                  # Versioned trading contracts
│   │   ├── v1/                     # Go types (20 event types)
│   │   ├── ts/                     # TypeScript equivalents
│   │   ├── schemas/                # JSON Schema v1 definitions
│   │   ├── prize_distribution/     # Prize distribution configs
│   │   └── go.mod
│   ├── db/                         # Database utilities
│   │   ├── migrations/             # SQL migration files (68 pairs)
│   │   └── go.mod
│   ├── exchangerate/               # Currency exchange rate service
│   ├── health/                     # Health check utilities
│   ├── inapp/                      # In-app notification support
│   ├── kyc/                        # KYC verification service (Jibit)
│   ├── notification/               # Email (Resend) and Discord notifications
│   ├── observability/              # Logging, metrics, tracing
│   │   ├── observability.go        # Main package entry point
│   │   ├── logger.go               # Structured JSON logging (zap)
│   │   ├── metrics.go              # Prometheus metrics
│   │   ├── tracing.go              # OpenTelemetry tracing
│   │   └── middleware.go           # HTTP middleware
│   ├── prize/                      # Prize distribution with commission
│   ├── ratelimit/                  # Rate limiting middleware
│   ├── redis/                      # Redis client (standalone/sentinel/cluster)
│   ├── resilience/                 # Resilience patterns (retry, fallback)
│   ├── scoring/                    # Scoring calculation (decimal precision)
│   ├── secrets/                    # Centralized secrets management
│   ├── shard/                      # Sharding utilities
│   ├── statemachine/               # State machine implementation
│   ├── storage/                    # S3/MinIO storage abstraction

│   ├── validation/                 # Input validation utilities
│   │   ├── validation.go           # Core validation functions
│   │   ├── middleware.go           # HTTP validation middleware
│   │   ├── sanitize.go             # Input sanitization
│   │   └── validation_test.go      # Validation tests
│   ├── wallet/                     # Wallet/balance management with ledger
│   └── wsregistry/                 # WebSocket registry
├── infra/                          # Infrastructure
│   ├── docker/
│   │   ├── docker-compose.yml      # Core services orchestration
│   │   ├── docker-compose.override.yml # Dev overrides
│   │   ├── docker-compose.redis-sentinel.yml # Redis Sentinel HA
│   │   ├── docker-compose.redis-cluster.yml  # Redis Cluster HA
│   │   ├── secrets/                # Docker secrets directory
│   │   └── daemon.json             # Docker daemon config
│   ├── k8s/                        # Kubernetes manifests
│   │   ├── base/                   # Base manifests (Kustomize)
│   │   │   ├── namespace.yaml
│   │   │   ├── configmap.yaml / secrets.yaml
│   │   │   ├── postgres.yaml / postgres-ha.yaml / postgres-config.yaml
│   │   │   ├── redis.yaml / redis-sentinel.yaml / redis-cluster.yaml
│   │   │   ├── redpanda.yaml
│   │   │   ├── pgbouncer.yaml / pgbouncer-config.yaml / pgbouncer-secrets.yaml
│   │   │   ├── {service}.yaml     # All 11 active services
│   │   │   ├── {frontend}.yaml    # All 3 frontends
│   │   │   ├── gateway.yaml
│   │   │   ├── ingress.yaml
│   │   │   ├── hpa.yaml           # Horizontal Pod Autoscaler
│   │   │   ├── pdb.yaml           # Pod Disruption Budget
│   │   │   ├── network-policies.yaml
│   │   │   ├── certificate.yaml / cluster-issuer.yaml # TLS
│   │   │   ├── external-secrets.yaml
│   │   │   ├── shard-config.yaml
│   │   │   └── kustomization.yaml
│   │   ├── overlays/
│   │   │   ├── production/         # Production overrides
│   │   │   └── staging/            # Staging overrides
│   │   ├── cronjobs/               # Backup CronJobs
│   │   │   ├── daily-backup.yaml
│   │   │   └── backup-verification.yaml
│   │   └── README.md / TLS_SETUP.md
│   ├── alertmanager/               # Alertmanager configuration
│   │   ├── alertmanager.yml
│   │   └── templates/
│   ├── pgbouncer/                  # PgBouncer connection pooling
│   │   └── pgbouncer.ini.template
│   ├── grafana/                    # Grafana configuration
│   │   └── provisioning/
│   │       ├── dashboards/
│   │       │   └── json/           # Pre-built dashboards
│   │       │       ├── system-overview.json
│   │       │       ├── websocket-realtime.json
│   │       │       ├── kafka-redpanda-health.json
│   │       │       └── scheduler-health.json
│   │       └── datasources/
│   ├── prometheus/
│   │   └── prometheus.yml
│   ├── loki/
│   │   └── loki-config.yml
│   ├── tempo/
│   │   └── tempo-config.yml
│   └── promtail/
│       └── promtail-config.yml
├── docs/                           # Documentation
│   ├── DEVELOPMENT.md              # Development guide
│   ├── E2E_TESTING.md              # E2E testing guide
│   ├── PGBOUNCER_MIGRATION.md      # PgBouncer migration guide
│   ├── SECURE_KEY_MANAGEMENT.md    # Secrets management guide
│   ├── SECURITY_HEADERS.md         # Security headers documentation
│   ├── chaos-engineering.md        # Chaos engineering guide
│   ├── go-live-checklist.md        # Go-live checklist
│   ├── health-checks.md            # Health check documentation
│   ├── kafka-client-unification.md # Kafka client guide
│   ├── kafka-partitioning.md       # Kafka partitioning strategy
│   ├── kafka-topics.md             # Kafka topics reference
│   ├── websocket-sticky-sessions.md # WebSocket session affinity
│   ├── runbook/                    # Operational runbooks
│   │   ├── incident-response.md
│   │   ├── database-recovery.md
│   │   ├── credential-rotation.md
│   │   ├── api-key-rotation.md
│   │   ├── scaling-guide.md
│   │   ├── service-restart.md
│   │   ├── deployment-procedures.md
│   │   ├── rollback-procedures.md
│   │   ├── postgres-ha.md
│   │   ├── redis-ha.md
│   │   ├── circuit-breaker.md
│   │   ├── log-troubleshooting.md
│   │   └── notifications.md
│   └── testing/
│       └── close-position-test-plan.md
├── scripts/                        # Operational scripts
│   ├── backup/                     # Backup scripts
│   │   ├── backup-postgres.sh
│   │   ├── restore-postgres.sh
│   │   ├── backup-redis.sh
│   │   └── restore-redis.sh
│   ├── secrets/                    # Secrets management
│   │   ├── init-secrets.sh
│   │   ├── generate-db-credentials.sh
│   │   ├── generate-ssl-certs.sh
│   │   ├── migrate-from-env.sh
│   │   └── verify-db-credentials.sh
│   ├── kafka/                      # Kafka management
│   │   └── create-topics.sh
│   ├── security/                   # Security testing
│   │   └── test-security-headers.sh
│   ├── check-health.sh             # Service health checks
│   ├── health-check.sh             # Health check utility
│   ├── kafka-setup.sh              # Kafka setup
│   ├── pull-docker-images.sh       # Docker image pre-pull
│   ├── fix-docker-tls.sh           # Docker TLS fix
│   ├── redis-failover-test.sh      # Redis failover testing
│   └── create_test_contest.sql     # Test contest SQL
├── tests/                          # Test suites
│   ├── integration/                # Integration tests (testcontainers)
│   │   ├── testhelpers.go
│   │   ├── auth_flow_test.go
│   │   ├── websocket_test.go
│   │   ├── trading_flow_test.go
│   │   └── go.mod
│   ├── contract/                   # API contract tests
│   │   └── api_routes_test.go
│   ├── e2e/                        # Go E2E tests (full-stack)
│   │   └── go.mod
│   └── manual/                     # Manual test scripts
│       └── close-position-test.sh
├── tools/                          # Development & testing tools
│   ├── ws-load-test/               # WebSocket load testing
│   ├── order-load-test/            # Order latency testing
│   ├── leaderboard-load-test/      # Leaderboard load testing
│   ├── order-burst-test/           # Order burst testing
│   ├── finalization-load-test/     # Contest finalization testing
│   ├── shard-load-test/            # Shard router load testing
│   └── chaos-test/                 # Chaos engineering scenarios
├── e2e/                            # Root E2E test fixtures
│   ├── fixtures.ts
│   ├── global-setup.ts
│   ├── global-teardown.ts
│   └── test-data.ts
├── go.work                         # Go workspace (Go 1.24.7)
├── pnpm-workspace.yaml             # pnpm workspace configuration
├── package.json                    # Root package.json
├── playwright.config.ts            # Playwright E2E config
├── .golangci.yml                   # golangci-lint configuration
├── Makefile                        # Build automation
├── .env.example                    # Environment variables template
├── .github/workflows/ci.yml        # CI/CD pipeline
└── README.md                       # Project documentation
```

## Service Documentation

### user-bff

User-facing backend for authentication and account management.

**Endpoints:**
| Method | Path | Description |
|--------|------|-------------|
| POST | `/register` | User registration with email validation |
| POST | `/login` | Login with email/password, returns JWT |
| POST | `/refresh` | Refresh access token |
| GET | `/me` | Get current user details (protected) |
| PUT | `/me/profile` | Update user profile |
| POST | `/me/avatar` | Upload user avatar (S3/MinIO) |
| POST | `/auth/google` | Google OAuth login |
| GET | `/auth/google/callback` | OAuth callback |
| POST | `/forgot-password` | Password reset request |
| POST | `/reset-password` | Password reset with token |
| POST | `/verify-email` | Email verification |
| POST | `/auth/send-otp` | Request OTP code for phone number |
| POST | `/auth/verify-otp` | Verify OTP code |
| POST | `/auth/register-phone` | Register/login via phone + OTP |
| GET | `/tournaments` | List tournaments |
| GET | `/tournaments/calendar` | Tournament calendar |
| GET | `/contests/:id/prizes` | Contest prize distribution |
| POST | `/contests/:id/leave` | Leave a contest |
| GET | `/healthz` | Liveness probe |
| GET | `/readyz` | Readiness probe |

**Features:**
- Password hashing with Argon2id
- JWT access/refresh token pairs with separate signing secrets
- OAuth (Google) social login
- Default "user" role assignment on registration
- Profile and avatar management via S3/MinIO
- Tournament listing and calendar APIs
- Email verification and password reset flows

### trade-bff

Real-time trading interface backend with WebSocket support.

**Endpoints:**
| Method | Path | Description |
|--------|------|-------------|
| GET | `/ws/trade` | WebSocket connection for trading |
| GET | `/ws/tournaments` | WebSocket for tournament feed |
| GET | `/healthz` | Liveness probe |
| GET | `/readyz` | Readiness probe |

**WebSocket Protocol:**
- JWT authentication via `Authorization` header
- Contest context via `X-Contest-ID` header
- Ping/pong keepalive mechanism (54s server ping interval)
- Optional compression (RFC 7692)
- Message batching for efficiency
- Connection tracking with graceful disconnection

**Features:**
- Real-time leaderboard broadcasting
- Market status and hours tracking
- Tournament feed subscription
- Contest event consumption (join/leave/state changes)
- Delta-based PnL updates

### admin-bff

Administrative backend for contest management and platform oversight.

**Endpoints:**
| Method | Path | Description |
|--------|------|-------------|
| POST | `/contests` | Create new contest |
| GET | `/contests` | List all contests |
| PUT | `/contests/:id` | Update contest |
| POST | `/contests/:id/freeze` | Pause a running contest |
| POST | `/contests/:id/transition` | Trigger state transition |
| GET | `/audit-logs` | Query audit logs with filters |
| GET/POST | `/templates` | Tournament template CRUD |
| GET/POST | `/schedules` | Tournament schedule CRUD |
| GET/PUT | `/market-hours` | Market hours configuration |
| GET/PUT | `/spreads` | Spread configuration |
| GET/POST | `/calendar` | Calendar entry management |
| GET/POST | `/email-templates` | Email template versioning |
| GET | `/tournament-stats` | Tournament statistics |
| GET | `/healthz` | Liveness probe |
| GET | `/readyz` | Readiness probe |

**Features:**
- Role-based access control (admin role required)
- Audit logging for all admin actions
- Tournament template and schedule management
- Market hours and spread configuration
- Email template versioning
- State machine transitions for contests

### market-ingestor

Market data ingestion service with multi-provider support.

**Providers:**
1. **Massive** (Primary) - WebSocket market data feed for forex/commodities
2. **TwelveData** (Fallback) - Automatic failover after 30s timeout
3. **Nobitex** (Crypto) - REST polling for crypto prices (24 symbols)

**Features:**
- Intelligent provider failover with auto-switchback
- Real-time tick publishing to Redis and Kafka (`ticks.v1` topic)
- OHLCV candle aggregation
- Configurable spread management
- Symbol registry with per-provider routing
- Control endpoints for provider switching
- Exponential backoff with jitter for reconnection
- Health check shows provider status and fallback state

**Kafka Topics:**
- Publishes to: `ticks.v1`

### trading-engine

Core order processing engine with advanced order type support.

**Order Types:**
| Type | Mode | Trigger Condition |
|------|------|-------------------|
| MARKET | MARKET | Immediate execution at current price |
| BUY_LIMIT | PENDING | Execute when ask ≤ limit_price |
| SELL_LIMIT | PENDING | Execute when bid ≥ limit_price |
| BUY_STOP | PENDING | Execute when ask ≥ stop_price |
| SELL_STOP | PENDING | Execute when bid ≤ stop_price |

**Features:**
- Take Profit / Stop Loss support per position
- Real-time price book with bid/ask synthesis (configurable spread)
- Pending order evaluation on each tick
- Position tracking and fill event generation
- Database persistence for orders, fills, positions
- Sharded Kafka consumption for horizontal scaling
- Write-ahead log (WAL) for crash recovery
- Position locking for concurrent safety
- Decimal precision scoring
- Per-user rate limiting
- Market hours enforcement
- Price freshness monitoring with staleness alerts
- Contest configuration caching

**Kafka Topics:**
- Consumes: `orders.v1`, `ticks.v1`
- Publishes: `fills.v1`, `positions.v1`, `pnl.v1`

### leaderboard-worker

Leaderboard computation and contest finalization service.

**Endpoints:**
| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Liveness probe |
| GET | `/readyz` | Readiness probe (checks DB, Redis, Kafka) |
| GET | `/metrics` | Prometheus metrics endpoint |

**Features:**
- Real-time leaderboard updates via Redis sorted sets
- Sharded leaderboard support for horizontal scaling
- Enhanced leaderboard with additional metrics
- PnL delta consumption from Kafka (`pnl.v1` topic)
- Contest state consumption for finalization (`contests.v1` topic)
- Periodic snapshot writing to PostgreSQL
- Prize pool distribution with configurable payout ratios and commission
- Automatic contest finalization when status changes to completed
- Notification event consumption

**Kafka Topics:**
- Consumes: `pnl.v1`, `contests.v1`

**Redis Keys:**
- `lb:{contest_id}` - Sorted set with user scores

### payment-service

Payment processing service for deposits and withdrawals.

**Providers:**
- **Jibit** - Iranian payment gateway (deposit, withdrawal, refund/reverse)
- **NowPayments** - Cryptocurrency payments

**Endpoints:**
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/payments/deposit/crypto/create` | Create crypto deposit (nowpayments) |
| POST | `/api/payments/deposit/fiat/create` | Create fiat deposit (jibit) |
| GET | `/api/payments/deposit/{id}/status` | Check deposit status |
| GET | `/api/payments/status/{purchaseId}` | Check status by provider payment ID |
| GET | `/api/payments/estimate` | Get crypto conversion estimate (nowpayments only) |
| POST | `/webhooks/nowpayments` | NowPayments IPN webhook |
| POST | `/callback/jibit` | Jibit payment callback (redirect + webhook) |

**Features:**
- Deposit and withdrawal processing with multiple payment providers
- Webhook handling for payment provider callbacks (IP whitelist and amount verification for Jibit; signature, freshness, and replay verification for NOWPayments)
- Transaction history and reporting
- KYC verification integration
- Circuit breaker pattern for provider failover
- Currency exchange rate conversion
- Wallet balance management with idempotency
- Payment expiry worker for stale transactions
- Inquiry worker for status reconciliation
- Cleanup logic for old transactions
- Withdrawal limits (AML compliance)

**Dependencies:**
- PostgreSQL, Redis, auth, wallet, kyc, exchangerate, circuitbreaker, notification packages

### settlement-service

Contest settlement and prize distribution service.

**Features:**
- Automated contest settlement when contests complete
- Prize pool calculation and distribution via wallet service
- Advisory lock-based idempotent prize credits
- Email notifications for winners
- Stuck settlement detection and recovery
- Kafka consumer for contest state events (`contests.v1`)

**Kafka Topics:**
- Consumes: `contests.v1`

**Dependencies:**
- PostgreSQL, Redis, Kafka, wallet, notification, secrets packages

### shard-router

Request routing service with resilience patterns.

**Features:**
- Request routing to appropriate service shards
- Circuit breaker pattern for backend protection
- Rate limiting per client/endpoint
- Response caching
- Alerting on circuit breaker state changes

**Dependencies:**
- Redis, auth, circuitbreaker, ratelimit, notification packages

### free-contest-generator

Automated free practice contest generation service.

**Features:**
- Generates free contests on configurable intervals (default: hourly)
- Asset class filtering (forex, crypto)
- Weekday-only and time-of-day restrictions
- Automatic contest symbol insertion
- Old contest cleanup
- Publishes contest creation events to Kafka (`contests.v1`)

**Kafka Topics:**
- Publishes to: `contests.v1`

### contest-scheduler

Contest lifecycle state machine and scheduler.

**Features:**
- Manages contest state transitions (scheduled → registration_open → running → completed)
- State machine pattern for valid transitions
- Distributed locking via Redis to prevent duplicate processing
- Email notifications on state changes (contest starting, ending reminders)
- Commission rate propagation
- Health check with dependency status

**Dependencies:**
- PostgreSQL, Redis, Kafka, statemachine, notification packages

## Database Schema

The PostgreSQL schema is managed via 68 migration pairs in `packages/db/migrations/`.

### Core Tables

| Table | Purpose |
|-------|---------|
| `users` | User accounts with UUID primary keys, profile, status |
| `roles` | Role definitions (user, admin, moderator) |
| `user_roles` | RBAC junction table |
| `contests` | Trading tournaments with configuration, templates, commission |
| `contest_symbols` | Symbols available per contest |
| `contest_participants` | User enrollment with scoring |
| `orders` | Trading orders with full lifecycle |
| `fills` | Order execution records |
| `positions` | User positions per contest/symbol |
| `leaderboard_snapshots` | Point-in-time leaderboard data |
| `audit_logs` | Action audit trail |

### Extended Tables (added via migrations)

| Table | Purpose |
|-------|---------|
| `shard_config` | Sharding configuration |
| `wallets` / `wallet_ledger` | Wallet balances and transaction ledger |
| `candles` | OHLCV candle data |
| `user_stats` | Aggregated user statistics |
| `kyc_documents` / `kyc_verifications` | KYC verification records |
| `affiliate_*` | Affiliate program tables |
| `password_reset_tokens` | Password reset flow |
| `email_verification_tokens` | Email verification flow |
| `user_profiles` | Extended user profiles |
| `withdrawal_limits` | Per-user withdrawal limits |
| `symbols_master` | Master symbol registry |
| `email_templates` / `email_template_versions` | Email template versioning |
| `settlement_batches` / `settlement_entries` | Settlement tracking |
| `tournament_templates` | Tournament configuration templates |
| `tournament_schedules` | Automated tournament scheduling |
| `calendar_entries` | Tournament calendar |
| `contest_reminders` | Reminder tracking |
| `finalization_tracking` | Contest finalization audit |
| `oauth_accounts` | OAuth provider accounts |
| `two_factor_auth` | 2FA configuration |

### Key Enums

- **Order Side**: `buy`, `sell`
- **Order Type**: `market`, `limit`, `stop`, `stop_limit`, `buy_limit`, `sell_limit`, `buy_stop`, `sell_stop`
- **Order Mode**: `market`, `pending`
- **Order Status**: `pending`, `open`, `partially_filled`, `filled`, `cancelled`, `rejected`, `expired`
- **Contest Status**: `draft`, `scheduled`, `registration_open`, `running`, `paused`, `completed`, `cancelled`
- **Position Side**: `long`, `short`

## Contracts Package

The `packages/contracts` package defines versioned event types for event-driven communication.

### Event Types (v1) - 20 Types

| Type | Purpose | Key Fields |
|------|---------|------------|
| `TickSnapshot` | Market data | symbol, bid, ask, last, volume, timestamp |
| `OrderRequest` | New order placement | order_id, user_id, contest_id, symbol, side, type, mode, qty, limit_price, stop_price, take_profit, stop_loss |
| `OrderAck` | Order response | order_id, status, reason, timestamp |
| `FillEvent` | Execution record | fill_id, order_id, price, qty, timestamp |
| `PositionUpdate` | Position change | user_id, contest_id, symbol, side, qty, avg_price, unrealized_pnl |
| `PnLDelta` | Score change | user_id, contest_id, delta, total_pnl |
| `ContestState` | Phase transition | contest_id, status, timestamp |
| `ContestEvent` | Contest lifecycle event | contest_id, event_type, payload |
| `ContestConfig` | Contest configuration | contest_id, settings |
| `CancelOrderRequest` | Order cancellation | order_id, user_id, contest_id |
| `ClosePositionRequest` | Position close | user_id, contest_id, symbol |
| `ModifyTPSLRequest` | TP/SL modification | order_id, take_profit, stop_loss |
| `OrderCancelledEvent` | Cancellation confirmation | order_id, reason |
| `PositionClosedEvent` | Position closed | user_id, contest_id, symbol, realized_pnl |
| `MarketStatus` | Market open/close | symbol, status, timestamp |
| `PriceStaleAlert` | Price staleness | symbol, last_update, threshold |
| `Settlement` | Settlement event | contest_id, entries |
| `TournamentFeed` | Tournament listing | tournaments, timestamp |
| Enum types | Shared enumerations | OrderSide, OrderType, OrderMode, etc. |

### Kafka Topics

| Topic | Publisher | Consumer(s) |
|-------|-----------|-------------|
| `ticks.v1` | market-ingestor | trading-engine |
| `orders.v1` | trade-bff | trading-engine |
| `fills.v1` | trading-engine | trade-bff |
| `positions.v1` | trading-engine | trade-bff |
| `order_acks.v1` | trading-engine | trade-bff |
| `pnl.v1` | trading-engine | leaderboard-worker |
| `contests.v1` | admin-bff, free-contest-generator | contest-scheduler, settlement-service, leaderboard-worker |

### Usage

**Go:**
```go
import contracts "github.com/Parsaeffatravesh/tragge/packages/contracts/v1"

order := contracts.OrderRequest{
    OrderID:    "ord-123",
    UserID:     "user-456",
    ContestID:  "contest-789",
    Symbol:     "AAPL",
    Side:       contracts.OrderSideBuy,
    Type:       contracts.OrderTypeBuyLimit,
    Mode:       contracts.OrderModePending,
    Qty:        100,
    LimitPrice: ptr(150.00),
    TakeProfit: ptr(160.00),
    StopLoss:   ptr(145.00),
}
```

**TypeScript:**
```typescript
import { OrderRequest, OrderSide, OrderType, OrderMode } from '@tragge/contracts/v1';

const order: OrderRequest = {
    order_id: 'ord-123',
    user_id: 'user-456',
    contest_id: 'contest-789',
    symbol: 'AAPL',
    side: OrderSide.Buy,
    type: OrderType.BuyLimit,
    mode: OrderMode.Pending,
    qty: 100,
    limit_price: 150.00,
    take_profit: 160.00,
    stop_loss: 145.00,
};
```

## Shared Packages

### Auth Package

The `packages/auth` package provides comprehensive authentication and authorization.

| File | Purpose |
|------|---------|
| `auth.go` | Password hashing with Argon2id |
| `jwt.go` | JWT token service (access + refresh with separate secrets) |
| `middleware.go` | HTTP middleware with role-based access control |
| `session.go` | Redis-based session management (hashed refresh tokens) |
| `password.go` | Password utility functions |

```go
import "github.com/Parsaeffatravesh/tragge/packages/auth"

// Password hashing
hash, _ := auth.HashPassword("password123")
valid := auth.CheckPassword("password123", hash)

// JWT tokens (separate refresh secret)
tokenService := auth.NewJWTService(secret, refreshSecret, accessTTL, refreshTTL)
tokens, _ := tokenService.GenerateTokens(userID, roles)

// Middleware
protected := auth.RequireAuth(jwtService)(handler)
adminOnly := auth.RequireRoles("admin")(handler)

// Context helpers
userID := auth.GetUserID(ctx)
roles := auth.GetRoles(ctx)
```

### Secrets Package

The `packages/secrets` package centralizes secret loading.

```go
import "github.com/Parsaeffatravesh/tragge/packages/secrets"

// Load secret from Docker secret file, falling back to environment variable
jwtSecret := secrets.Load("JWT_SECRET", "/run/secrets/jwt_secret")
dbPassword := secrets.Load("POSTGRES_PASSWORD", "/run/secrets/postgres_password")
```

### Validation Package

The `packages/validation` package provides input validation and sanitization.

```go
import "github.com/Parsaeffatravesh/tragge/packages/validation"

err := validation.ValidateEmail(email)
err := validation.ValidatePassword(password)
clean := validation.SanitizeString(input)
r.Use(validation.RequestValidator)
```

### Observability Package

The `packages/observability` package provides logging, metrics, and tracing.

```go
import "github.com/Parsaeffatravesh/tragge/packages/observability"

obs, err := observability.New(ctx, observability.Config{
    Service:      "my-service",
    Env:          os.Getenv("ENVIRONMENT"),
    Version:      os.Getenv("VERSION"),
    OTLPEndpoint: os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
})
defer obs.Shutdown(context.Background())

obs.Logger.Logger.Info("Service started", zap.String("port", port))
r := chi.NewRouter()
r.Use(obs.Middleware.Middleware)
r.Get("/metrics", obs.MetricsHandler())
```

### Redis Package

The `packages/redis` package supports multiple Redis deployment modes.

```go
import redispkg "github.com/Parsaeffatravesh/tragge/packages/redis"

// Supports standalone, sentinel, and cluster modes
// Mode selected via REDIS_MODE environment variable
client := redispkg.NewClient(redispkg.Config{
    Mode:           "sentinel", // standalone, sentinel, cluster
    Addr:           "localhost:6379",
    SentinelAddrs:  []string{"sentinel-1:26379", "sentinel-2:26379"},
    SentinelMaster: "mymaster",
})
```

## Infrastructure Services

### Docker Compose (Development)

Docker Compose (`infra/docker/docker-compose.yml`) provides:

| Service | Image | Port(s) | Purpose |
|---------|-------|---------|---------|
| `postgres` | postgres:16-alpine | 5432 | Primary database |
| `redis` | redis:7-alpine | 6379 | Cache & sessions |
| `redpanda` | redpandadata/redpanda:v24.1.1 | 9092, 8081, 8082 | Kafka-compatible messaging |
| `redpanda-console` | redpandadata/console:v2.5.2 | 8088 | Redpanda Web UI |
| `gateway` | tragge-gateway | 8080 | Nginx reverse proxy |

**Redis HA Options:**
| Mode | Compose File | Description |
|------|-------------|-------------|
| Standalone | `docker-compose.yml` | Single Redis instance (default) |
| Sentinel | `docker-compose.redis-sentinel.yml` | Master/replica with 3 sentinels |
| Cluster | `docker-compose.redis-cluster.yml` | 6-node Redis cluster |

**Monitoring Stack:**

| Service | Image | Port(s) | Purpose |
|---------|-------|---------|---------|
| `prometheus` | prom/prometheus:v2.48.0 | 9090 | Metrics collection |
| `grafana` | grafana/grafana:10.2.2 | 3000 | Dashboards & visualization |
| `loki` | grafana/loki:2.9.2 | 3100 | Log aggregation |
| `tempo` | grafana/tempo:2.3.1 | 3200, 4317, 4318 | Distributed tracing |
| `promtail` | grafana/promtail:2.9.2 | - | Log collector |

**Pre-built Grafana Dashboards:**
- System Overview - Overall platform health
- WebSocket Real-time - WebSocket connection metrics
- Kafka/Redpanda Health - Message queue monitoring
- Scheduler Health - Contest scheduler and prize monitoring

### Kubernetes (Production)

Kubernetes manifests in `infra/k8s/` provide production deployment:

```bash
# Preview deployment
kubectl kustomize infra/k8s/base

# Deploy base configuration
kubectl apply -k infra/k8s/base

# Deploy production overlay
kubectl apply -k infra/k8s/overlays/production

# Deploy staging overlay
kubectl apply -k infra/k8s/overlays/staging

# Check deployment status
kubectl get all -n tragge
```

**Features:**
- Kustomize-based configuration with production and staging overlays
- Horizontal Pod Autoscaler (HPA) for dynamic scaling
- Pod Disruption Budgets (PDB) for availability
- Network Policies for service isolation
- PgBouncer for connection pooling
- PostgreSQL HA with streaming replication
- Redis Sentinel/Cluster for high availability
- TLS certificates via cert-manager
- Ingress routing with NGINX
- External secrets integration
- Automated backup CronJobs (PostgreSQL + Redis to S3)

### Gateway Routes

| Path | Target | Description |
|------|--------|-------------|
| `/user/*` | frontend | User module of the unified SPA |
| `/trade/*` | frontend | Trade module of the unified SPA |
| `/admin/*` | frontend | Admin module of the unified SPA |
| `/avatars/*` | static files | Default avatar images |
| `/api/user/*` | user-bff:8081 | User API |
| `/api/trade/*` | trade-bff:8082 | Trading API |
| `/api/admin/*` | admin-bff:8083 | Admin API |
| `/api/tournaments/*` | trade-bff:8082 | Tournament API |
| `/api/payments/*` | payment-service | Payment API |
| `/api/wallet/*` | payment-service | Wallet API |
| `/api/leaderboard/*` | leaderboard-worker | Leaderboard API |
| `/api/shards/*` | shard-router | Shard API |
| `/webhooks/*` | payment-service | Webhook callbacks |
| `/callback/jibit` | payment-service | Jibit payment callbacks |
| `/ws/trade` | trade-bff:8082 | Trading WebSocket |
| `/ws/tournaments` | trade-bff:8082 | Tournament feed WebSocket |
| `/health` | - | Health check |

**Security:**
- Rate limiting per endpoint type (API, auth, admin, WebSocket)
- Security headers (CSP, HSTS, X-Frame-Options, etc.)
- Request size limits
- Connection limits per IP

## Environment Configuration

Copy `.env.example` to `.env` and initialize secrets:

```bash
cp .env.example .env
./scripts/secrets/init-secrets.sh
```

**Key configuration areas:**

```bash
# Database (non-sensitive)
POSTGRES_DB=app
POSTGRES_USER=app
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_SSLMODE=disable

# Redis (supports standalone/sentinel/cluster)
REDIS_MODE=standalone
REDIS_ADDR=localhost:6379

# Kafka/Redpanda
KAFKA_BROKERS=localhost:9092

# Market Data
MARKET_PROVIDER=massive    # massive|twelvedata|auto
NOBITEX_ENABLED=true       # Crypto price feed
SYMBOLS=EUR/USD,GBP/USD,...,BTC/USD,ETH/USD,...  # 47 symbols

# Service Ports
USER_BFF_PORT=8081
TRADE_BFF_PORT=8082
ADMIN_BFF_PORT=8083
MARKET_INGESTOR_PORT=8084
TRADING_ENGINE_PORT=8085
LEADERBOARD_WORKER_PORT=8086

# Observability
ENVIRONMENT=development
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317

# Notifications
NOTIFICATION_ENABLED=false
RESEND_FROM_EMAIL=onboarding@resend.dev

# CORS
ALLOWED_ORIGINS=http://localhost:5173,http://localhost:5174,http://localhost:5175
```

**Sensitive values managed via Docker secrets:**
- `POSTGRES_PASSWORD`, `REDIS_PASSWORD`
- `JWT_SECRET`, `JWT_REFRESH_SECRET`
- `TWELVEDATA_API_KEYS`, `MASSIVE_API_KEYS`
- `RESEND_API_KEY`, `DISCORD_WEBHOOK_URL`
- `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`
- `JIBIT_API_KEY`, `JIBIT_SECRET_KEY`, `JIBIT_KYC_API_KEY`, `JIBIT_KYC_SECRET_KEY`
- `NOWPAYMENTS_API_KEY`, `NOWPAYMENTS_IPN_SECRET`
- `NOBITEX_TOKEN`
- `S3_ACCESS_KEY`, `S3_SECRET_KEY`
- `TOTP_ENCRYPTION_KEY`
- `KAVENEGAR_API_KEY`
- `GRAFANA_ADMIN_PASSWORD`

See `docs/SECURE_KEY_MANAGEMENT.md` for full documentation.

## Commands Reference

### Installation & Setup

```bash
make install          # Install frontend dependencies (pnpm install)
make go-sync          # Sync Go workspace
```

### Development Servers

```bash
make dev                        # Start all development servers
make dev-frontend               # Start unified SPA (serves /user, /trade, /admin on port 5173)
make dev-user-bff               # Start user BFF (port 8081)
make dev-trade-bff              # Start trade BFF (port 8082)
make dev-admin-bff              # Start admin BFF (port 8083)
make dev-trading-engine         # Start trading engine (port 8085)
make dev-market-ingestor        # Start market ingestor (port 8084)
make dev-leaderboard-worker     # Start leaderboard worker (port 8086)
make dev-payment-service        # Start payment service
make dev-contest-scheduler      # Start contest scheduler
make dev-free-contest-generator # Start free contest generator
make dev-settlement-service     # Start settlement service
```

### Build & Test

```bash
make build            # Build all applications
make build-frontends  # Build Vue 3 frontends
make build-go         # Build all Go services
make build-gateway    # Build gateway Docker image
make test             # Run all tests
make test-go          # Run Go tests only
make test-frontends   # Run frontend tests only
make lint             # Lint all code
make lint-go          # Lint Go code (golangci-lint)
make lint-go-fix      # Lint Go with auto-fix
make lint-go-new      # Lint only new issues (for PRs)
```

### Infrastructure Management

```bash
make up               # Start all Docker services
make down             # Stop all Docker services
make logs             # View service logs
make ps               # List running containers
make pull-images      # Pre-pull Docker images (helps with TLS issues)
make fix-docker-tls   # Fix Docker TLS issues (requires sudo)
make check-health     # Run service health checks
```

### Redis High Availability

```bash
make up-redis-sentinel        # Start with Redis Sentinel HA
make up-redis-cluster         # Start with Redis Cluster HA
make redis-sentinel-status    # Check Sentinel status
make redis-cluster-status     # Check Cluster status
make redis-sentinel-failover  # Trigger Sentinel failover (test)
```

### Secrets Management

```bash
make init-secrets              # Initialize Docker secrets
make generate-db-credentials   # Generate database credentials
make verify-secrets            # Verify secret files exist
```

### Database Migrations

```bash
make migrate-up              # Apply all pending migrations
make migrate-down            # Rollback last migration
make migrate-version         # Show current migration version
make migrate-create NAME=x   # Create new migration pair
```

### Load Testing

```bash
# WebSocket load test
make load-test-ws EMAIL=... PASSWORD=... CONTEST_ID=... [N=100] [DURATION=60s]

# WebSocket storm test (1000 connections)
make load-test-ws-storm EMAIL=... PASSWORD=... CONTEST_ID=...

# Leaderboard load test (500 simultaneous requests)
make load-test-leaderboard EMAIL=... PASSWORD=... CONTEST_IDS=id1,id2,...

# Order burst test (100 orders/sec across 10 contests)
make load-test-order-burst CONTEST_IDS=id1,id2,...id10

# Contest finalization test (1000 participants)
make load-test-finalization [PARTICIPANTS=1000]

# Run all load tests
make load-test-all EMAIL=... PASSWORD=... CONTEST_ID=... CONTEST_IDS=...
```

### Chaos Engineering

```bash
make chaos-test-list          # List available chaos scenarios
make chaos-test SCENARIO=pod-kill [NAMESPACE=tragge]
make chaos-test-all           # Run all chaos scenarios
```

### E2E Testing

```bash
# Playwright (frontend)
make e2e              # Run all E2E tests
make e2e-user         # Run user-module E2E tests (Playwright --project=user-chromium)
make e2e-trade        # Run trade-module E2E tests (Playwright --project=trade-chromium)
make e2e-admin        # Run admin-module E2E tests (Playwright --project=admin-chromium)
make e2e-ui           # Open Playwright UI mode
make e2e-install      # Install Playwright browsers
make e2e-report       # Show Playwright HTML report

# Go E2E (full-stack)
make e2e-go           # Run Go E2E tests
make e2e-lifecycle    # Run contest lifecycle E2E test
```

### Notification Testing

```bash
make test-notification-unit             # Unit tests
make test-notification-integration      # Integration tests (mocked)
make test-notification-integration-real # Integration tests (real services)
make test-notification-all              # All notification tests
```

## Testing

### Unit Tests

```bash
make test-go          # Run all Go unit tests
make test-frontends   # Run all frontend tests
```

### Integration Tests

Integration tests use testcontainers for isolated testing:

```bash
cd tests/integration && go test -v ./...
go test -v -run TestAuthFlow ./tests/integration/...
```

The test environment automatically spins up PostgreSQL 16, Redis 7, and Redpanda with auto-created topics.

### Contract Tests

API contract tests verify route definitions:

```bash
cd tests/contract && go test -v ./...
```

### E2E Tests (Playwright)

E2E tests are split across the two panels' own directories and
selected by Playwright `--project=`:

| Module | Playwright project | Location | Tests |
|--------|--------------------|----------|-------|
| user   | `user-chromium`    | `apps/user-frontend/e2e/`  | auth, leaderboard, profile, tournament flows |
| trade  | `trade-chromium`   | `apps/user-frontend/e2e/`  | trading, websocket |
| admin  | `admin-chromium`   | `apps/admin-frontend/e2e/` | audit, contests, shards |

**Features:**
- Page Object pattern for maintainability
- Auth state persistence (runs auth setup once)
- API response mocking
- WebSocket testing helpers
- Cross-browser testing (Chromium, Firefox, WebKit)
- Mobile viewport testing
- Screenshot/video on failure

### Go E2E Tests

Full-stack integration tests that run against docker-compose:

```bash
make e2e-go           # All Go E2E tests
make e2e-lifecycle    # Contest lifecycle test
```

### Load Testing Tools

| Tool | Purpose | Key Parameters |
|------|---------|----------------|
| `ws-load-test` | WebSocket connection stress | N connections, duration, ramp-up |
| `order-load-test` | Order latency measurement | users, duration |
| `leaderboard-load-test` | Leaderboard request stress | 500 concurrent, multiple contests |
| `order-burst-test` | Order throughput testing | 100 orders/sec, 10 contests |
| `finalization-load-test` | Prize distribution testing | 1000 participants |
| `shard-load-test` | Shard router stress testing | concurrent requests |
| `chaos-test` | Chaos engineering scenarios | pod-kill, network-partition, etc. |

## Development Guidelines

### Getting Started

1. Ensure Go 1.24+ is installed
2. Ensure Node.js 18+ and pnpm 8+ are installed
3. Copy `.env.example` to `.env`
4. Run `./scripts/secrets/init-secrets.sh` to initialize secrets
5. Run `make install` to install frontend dependencies
6. Run `make go-sync` to sync Go workspace
7. Run `make up` to start infrastructure services
8. Run `make migrate-up` to apply database migrations
9. Start services with `make dev-*` commands

### Code Conventions

**Go:**
- Follow standard Go conventions and `gofmt`
- Use golangci-lint (config in `.golangci.yml`)
- Use meaningful package and function names
- Add comments for exported functions
- Use the contracts package for event types
- Use the auth package for authentication/authorization
- Use the validation package for input validation
- Use the secrets package for loading sensitive configuration
- Module naming: `github.com/Parsaeffatravesh/tragge/{type}/{name}`
- All services have `main.go` at the app root (not in `cmd/` subdirectory)

**TypeScript/Vue:**
- Use TypeScript strict mode
- Follow Vue 3 Composition API patterns
- Use scoped packages (@tragge/*)
- Import contracts from `@tragge/contracts/v1`
- Use i18n for all user-facing strings

**Database:**
- Use sequential numbering for migrations (0001, 0002, etc.)
- Add appropriate indexes for query performance
- Use ENUM types for fixed value sets
- Include check constraints for data integrity
- Use advisory locks for idempotent operations

**Security:**
- Never hardcode secrets; use the secrets package
- Hash refresh tokens in session storage
- Use separate JWT signing secrets for access and refresh tokens
- Validate webhook callbacks (IP whitelist, amount verification)
- Apply rate limiting on all public endpoints
- Use security headers (CSP, HSTS, etc.)

**Error Handling:**
- Return proper HTTP status codes
- Log errors with context
- Never expose internal errors to clients
- Use circuit breakers for external dependencies

### Git Workflow

- **Branch naming**: Use descriptive branch names (e.g., `feature/add-auth`, `fix/login-bug`)
- **Commits**: Write clear, concise commit messages describing the change
- **Pull requests**: Include a description of changes and testing done

## Architecture Patterns

### Event-Driven Design

Services communicate via Redpanda using the versioned contracts:

```
                          ┌──────────────────┐
                          │  redpanda-console│
                          │    (Web UI)      │
                          └────────┬─────────┘
                                   │
┌─────────────┐  ticks.v1   ┌──────▼──────┐  orders.v1   ┌────────────┐
│   market-   │────────────►│             │◄─────────────│  trade-bff │
│   ingestor  │             │   Redpanda  │              │ (WebSocket)│
│  (Massive + │             │   (Kafka)   │              └────────────┘
│   Nobitex)  │             └──────┬──────┘                     ▲
└─────────────┘                    │                            │
                    ticks.v1       │  fills.v1                  │
                    orders.v1      │  positions.v1              │
                                   ▼                            │
                            ┌──────────────┐                    │
                            │   trading-   │────────────────────┘
                            │    engine    │  (via Redis pub/sub)
                            └──────┬───────┘
                                   │
                              pnl.v1
                                   ▼
                            ┌──────────────┐
                            │ leaderboard- │
                            │    worker    │
                            └──────────────┘

contests.v1 ──► contest-scheduler, settlement-service, leaderboard-worker
```

### BFF Pattern

All three BFFs serve the single `frontend` SPA; the SPA differentiates
audience by module-prefixed routes under `/user/*`, `/trade/*`,
`/admin/*`:
- `user-bff` handles the user module's API surface (account management, auth, tournaments)
- `trade-bff` handles the trade module's API surface (WebSocket trading, real-time updates, tournament feed)
- `admin-bff` handles the admin module's API surface (contest management, templates, audit logs)

### Authentication Flow

```
1. User registers/logs in via user-bff (or OAuth via Google)
2. user-bff returns JWT access + refresh tokens (separate signing secrets)
3. Refresh tokens are hashed before session storage
4. Client includes access token in Authorization header
5. BFFs validate token via auth middleware
6. Protected endpoints extract user from context
7. Refresh flow issues new token pair and invalidates old session
```

### Trading Order Flow

```
1. Client sends order via WebSocket to trade-bff
2. trade-bff publishes OrderRequest to orders.v1
3. trading-engine consumes order (sharded by contest_id)
4. For MARKET orders: immediate execution
5. For PENDING orders: stored and evaluated on each tick
6. On fill: FillEvent published to fills.v1
7. Position updated and PositionUpdate published
8. trade-bff receives events and pushes to WebSocket
9. PnL delta published for leaderboard scoring
```

### Observability Stack

```
Services ──metrics──► Prometheus ──► Grafana (dashboards)
    │                                     ▲
    ├───logs────► Promtail ──► Loki ──────┘
    │                                     │
    └───traces──► Tempo ──────────────────┘

Alertmanager ◄── Prometheus alerts ──► Discord/Email notifications
```

### Database Access

- BFFs and services connect to PostgreSQL (optionally via PgBouncer)
- Use connection pooling (configured per-service)
- Use the `packages/db` utilities for migrations
- Connection string from `POSTGRES_DSN` or individual env vars
- PgBouncer in transaction mode for connection multiplexing

## Key Files to Know

| File | Purpose |
|------|---------|
| `packages/contracts/v1/*.go` | Go event type definitions (20 types) |
| `packages/contracts/ts/v1/*.ts` | TypeScript event types |
| `packages/auth/*.go` | Authentication/authorization utilities |
| `packages/secrets/*.go` | Centralized secrets loading |
| `packages/validation/*.go` | Input validation utilities |
| `packages/observability/*.go` | Logging, metrics, tracing utilities |
| `packages/redis/*.go` | Redis client (standalone/sentinel/cluster) |
| `packages/db/migrations/*.sql` | Database schema migrations (68 pairs) |
| `infra/docker/docker-compose.yml` | Core infrastructure services |
| `infra/docker/docker-compose.redis-sentinel.yml` | Redis Sentinel HA |
| `infra/docker/docker-compose.redis-cluster.yml` | Redis Cluster HA |
| `infra/k8s/` | Kubernetes manifests |
| `infra/pgbouncer/pgbouncer.ini.template` | PgBouncer configuration |
| `infra/alertmanager/alertmanager.yml` | Alertmanager configuration |
| `apps/gateway/nginx.conf` | API routing configuration |
| `apps/trading-engine/engine.go` | Core order processing logic |
| `apps/market-ingestor/main.go` | Market data provider logic |
| `apps/leaderboard-worker/main.go` | Leaderboard processing |
| `apps/leaderboard-worker/payout.go` | Prize pool distribution logic |
| `apps/payment-service/main.go` | Payment processing entry point |
| `apps/payment-service/providers/jibit.go` | Jibit payment provider |
| `apps/settlement-service/settlement.go` | Contest settlement logic |
| `apps/shard-router/router.go` | Request routing logic |
| `apps/contest-scheduler/main.go` | Contest lifecycle scheduler |
| `.github/workflows/ci.yml` | CI/CD pipeline (lint, test, build) |
| `.golangci.yml` | Go linter configuration |
| `playwright.config.ts` | E2E test configuration |
| `tests/integration/testhelpers.go` | Integration test setup |
| `docs/SECURE_KEY_MANAGEMENT.md` | Secrets management guide |
| `docs/runbook/` | Operational runbooks (13 procedures) |
| `.env.example` | Environment variables template |
| `Makefile` | Build automation commands |
| `go.work` | Go workspace modules (Go 1.24.7) |

## Operational Runbooks

The `docs/runbook/` directory contains procedures for common operations:

| Runbook | Purpose |
|---------|---------|
| `incident-response.md` | Incident severity levels, response procedures, escalation paths |
| `database-recovery.md` | PostgreSQL backup/restore, point-in-time recovery |
| `credential-rotation.md` | PostgreSQL/JWT/API credential rotation procedures |
| `api-key-rotation.md` | API key rotation for external services |
| `scaling-guide.md` | Horizontal scaling procedures for services |
| `service-restart.md` | Safe service restart procedures |
| `deployment-procedures.md` | Deployment workflows |
| `rollback-procedures.md` | Rollback strategies and steps |
| `postgres-ha.md` | PostgreSQL high availability management |
| `redis-ha.md` | Redis Sentinel/Cluster management |
| `circuit-breaker.md` | Circuit breaker troubleshooting |
| `log-troubleshooting.md` | Log analysis and debugging |
| `notifications.md` | Notification system operations |

## AI Assistant Guidelines

When working on this repository:

1. **Read before modifying**: Always read files before making changes
2. **Minimal changes**: Make only the requested changes; avoid unnecessary refactoring
3. **Preserve style**: Match existing code style and conventions
4. **Use packages**: Use auth package for authentication, contracts for events, validation for input, secrets for sensitive config
5. **Test changes**: Run `make test` to verify changes work
6. **Update docs**: Update relevant documentation when making significant changes
7. **Security first**: Never introduce security vulnerabilities (XSS, injection, etc.)
8. **Never hardcode secrets**: Always use the secrets package or Docker secrets

### Visual regression caveat

A passing build + passing tests do NOT guarantee a functioning UI. The
design-tokens regression (69c1b72) was introduced in March 2026 during the
frontend consolidation, sat undetected for a month through multiple audits
claiming "production ready", and was only discovered when a human finally
opened the browser.

Before declaring any UI-touching work complete:
1. Open the browser and actually look at the affected pages
2. Screenshot or describe what you see
3. Do not rely on build success as evidence of functional UI

### For New Features

1. Understand the existing architecture and event flow
2. Follow established patterns in the codebase
3. Use the contracts package for new event types
4. Use the auth package for any authentication needs
5. Use the validation package for input validation
6. Use the secrets package for any sensitive configuration
7. Add database migrations for schema changes
8. Add appropriate tests (unit + integration)
9. Update documentation as needed

### For Bug Fixes

1. Reproduce the issue first
2. Identify the root cause
3. Make minimal, targeted fixes
4. Verify the fix resolves the issue
5. Check for similar issues elsewhere
6. Add tests to prevent regression

### Adding New Services

1. Create directory under `apps/` with `main.go` at the root level
2. Add Dockerfile for containerization
3. Add to `go.work` workspace
4. Add Makefile targets for dev/build
5. Add to docker-compose and Kubernetes manifests
6. Configure gateway routing if HTTP/WebSocket exposed
7. Add integration tests
8. Use existing patterns from similar services

### Adding New Contracts

1. Add Go types in `packages/contracts/v1/`
2. Add TypeScript types in `packages/contracts/ts/v1/`
3. Add JSON schema in `packages/contracts/schemas/`
4. Update exports in respective index files

### Adding Database Migrations

1. Run `make migrate-create NAME=descriptive_name`
2. Edit the generated `.up.sql` and `.down.sql` files
3. Test with `make migrate-up` and `make migrate-down`
4. Ensure down migration properly reverses up migration
5. Update integration tests if schema changes affect tests

### Adding i18n Translations

1. Add English translation in `apps/{frontend}/src/i18n/locales/en.ts`
2. Add Farsi translation in `apps/{frontend}/src/i18n/locales/fa.ts`
3. Use the translation key in Vue components via `$t('key')`

### Security findings during unrelated work

Security-critical issues discovered during unrelated work must be:
1. Documented in a separate `docs/SECURITY_ISSUE_*.md` file with severity, impact, and fix outline
2. Flagged to the human for prioritization decision
3. **Not bundled** with the current branch, even if the fix seems small

Rationale: Mixing security fixes with unrelated changes makes PR review harder,
increases rollback risk if anything breaks, and obscures the security fix's history
in `git log` / `git blame`. Each security fix deserves its own branch, PR, and
review gate.

Worked example: `docs/SECURITY_ISSUE_ADMIN_AUTH_WIRING.md` — discovered during
a gateway assets regression fix, properly documented and scoped out.

## Maintenance Notes

This CLAUDE.md should be updated when:

- New services are added or existing ones significantly changed
- New tools or frameworks are added
- Project structure changes significantly
- Development workflows are established
- New conventions are adopted
- Contract versions are added
- Database schema changes significantly
- New operational runbooks are added
- Infrastructure configuration changes
- New packages are added or removed

---

*Last updated: February 2026*
