.PHONY: help install dev build test lint clean go-sync \
	dev-frontend \
	dev-user-bff dev-trade-bff dev-admin-bff \
	dev-trading-engine dev-market-ingestor dev-leaderboard-worker dev-contest-scheduler dev-free-contest-generator dev-settlement-service \
	build-frontends build-go test-go test-frontends \
	lint-go lint-go-fix lint-go-new lint-frontends \
	pull-images fix-docker-tls up down logs ps build-gateway \
	up-minimal down-minimal up-lite down-lite up-full-lite down-full-lite \
	up-redis-sentinel up-redis-cluster redis-sentinel-status redis-cluster-status \
	migrate-up migrate-down migrate-create migrate-version \
	load-test-ws load-test-ws-storm load-test-leaderboard load-test-order-burst load-test-finalization load-test-all \
	chaos-test chaos-test-list chaos-test-all \
	e2e e2e-user e2e-trade e2e-admin e2e-ui e2e-install e2e-report e2e-go e2e-lifecycle \
	test-notification-unit test-notification-integration test-notification-integration-real test-notification-all \
	baseline-inventory baseline-verify test-baseline \
	check-health

# Default target
help:
	@echo "tragge - Trading Tournament Platform"
	@echo ""
	@echo "Usage:"
	@echo "  make install          Install all dependencies"
	@echo "  make dev              Start all development servers"
	@echo "  make build            Build all applications"
	@echo "  make test             Run all tests"
	@echo "  make lint             Lint all code"
	@echo "  make clean            Clean build artifacts"
	@echo "  make go-sync          Sync Go workspace"
	@echo "  make baseline-inventory  Print the reproducible production-baseline inventory"
	@echo "  make baseline-verify     Verify baseline counts, evidence paths, links, and toolchains"
	@echo "  make test-baseline       Run focused tests for the baseline verifier"
	@echo ""
	@echo "Frontend targets:"
	@echo "  make dev-frontend        Start unified frontend dev server"
	@echo ""
	@echo "Go service targets:"
	@echo "  make dev-user-bff           Start user BFF"
	@echo "  make dev-trade-bff          Start trade BFF"
	@echo "  make dev-admin-bff          Start admin BFF"
	@echo "  make dev-trading-engine     Start trading engine"
	@echo "  make dev-market-ingestor    Start market ingestor"
	@echo "  make dev-leaderboard-worker Start leaderboard worker"
	@echo "  make dev-contest-scheduler  Start contest scheduler"
	@echo "  make dev-free-contest-generator Start free contest generator"
	@echo "  make dev-settlement-service     Start settlement service"
	@echo ""
	@echo "Docker targets:"
	@echo "  make pull-images            Pre-pull Docker images (fixes TLS timeout issues)"
	@echo "  make fix-docker-tls         Configure Docker registry mirrors (requires sudo)"
	@echo "  make up                     Start all services (full mode)"
	@echo "  make down                   Stop all services"
	@echo "  make logs                   View service logs"
	@echo "  make ps                     List running containers"
	@echo "  make build-gateway          Build nginx gateway image"
	@echo "  make check-health           Run service health checks (--verbose, --json)"
	@echo ""
	@echo "Lightweight targets:"
	@echo "  make up-minimal             Infra only (~700MB RAM, run Go services locally)"
	@echo "  make up-lite                Core app + frontends, no monitoring (~1.5GB RAM)"
	@echo "  make up-full-lite           All services with resource limits (~3GB RAM)"
	@echo "  make down-minimal           Stop minimal services"
	@echo "  make down-lite              Stop lite services"
	@echo "  make down-full-lite         Stop full-lite services"
	@echo ""
	@echo "Redis HA targets:"
	@echo "  make up-redis-sentinel      Start with Redis Sentinel HA"
	@echo "  make up-redis-cluster       Start with Redis Cluster HA"
	@echo "  make redis-sentinel-status  Check Sentinel status"
	@echo "  make redis-cluster-status   Check Cluster status"
	@echo "  make redis-sentinel-failover Trigger Sentinel failover (test)"
	@echo ""
	@echo "Database targets:"
	@echo "  make migrate-up             Run all pending migrations"
	@echo "  make migrate-down           Rollback the last migration"
	@echo "  make migrate-version        Show current migration version"
	@echo "  make migrate-create NAME=x  Create a new migration"
	@echo ""
	@echo "Load testing targets:"
	@echo "  make load-test-ws           Run WebSocket load test"
	@echo "  make load-test-ws-storm     Run WebSocket storm test (1000 connections)"
	@echo "  make load-test-leaderboard  Run leaderboard load test (500 concurrent requests)"
	@echo "  make load-test-order-burst  Run order burst test (100 orders/sec across 10 contests)"
	@echo "  make load-test-finalization Run contest finalization test (1000 participants)"
	@echo "  make load-test-all          Run all load tests in sequence"
	@echo ""
	@echo "Chaos engineering targets:
	@echo "  make chaos-test-list        List available chaos scenarios"
	@echo "  make chaos-test             Run a chaos scenario (SCENARIO=pod-kill NAMESPACE=tragge)"
	@echo "  make chaos-test-all         Run all chaos scenarios"
	@echo ""
	@echo "E2E testing targets:"
	@echo "  make e2e                    Run all E2E tests"
	@echo "  make e2e-user               Run user E2E tests"
	@echo "  make e2e-trade              Run trade E2E tests"
	@echo "  make e2e-admin              Run admin E2E tests"
	@echo "  make e2e-ui                 Open Playwright UI mode"
	@echo "  make e2e-install            Install Playwright browsers"
	@echo "  make e2e-report             Show Playwright HTML report"
	@echo "  make e2e-go                 Run Go E2E tests (requires docker-compose)"
	@echo "  make e2e-lifecycle          Run contest lifecycle E2E test"
	@echo ""
	@echo "Notification testing targets:"
	@echo "  make test-notification-unit             Run notification package unit tests"
	@echo "  make test-notification-integration      Run notification integration tests (mocked)"
	@echo "  make test-notification-integration-real Run notification integration tests (real services)"
	@echo "  make test-notification-all              Run all notification tests"

# =============================================================================
# Main targets
# =============================================================================

install:
	pnpm install

dev:
	@trap 'kill 0' INT TERM; \
	$(MAKE) dev-frontend & \
	$(MAKE) dev-user-bff & \
	$(MAKE) dev-trade-bff & \
	$(MAKE) dev-admin-bff & \
	$(MAKE) dev-trading-engine & \
	$(MAKE) dev-market-ingestor & \
	$(MAKE) dev-leaderboard-worker & \
	$(MAKE) dev-payment-service & \
	$(MAKE) dev-contest-scheduler & \
	$(MAKE) dev-free-contest-generator & \
	$(MAKE) dev-settlement-service & \
	wait

build: build-frontends build-go

test: test-go test-frontends

lint: lint-go lint-frontends

clean:
	@echo "TODO: Clean build artifacts"

go-sync:
	go work sync

baseline-inventory:
	node scripts/production-baseline.mjs inventory

baseline-verify:
	node scripts/production-baseline.mjs verify

test-baseline:
	node scripts/production-baseline.test.mjs

# =============================================================================
# Frontend targets
# =============================================================================

dev-frontend: dev-user-frontend

dev-user-frontend:
	pnpm --filter @tragge/user-frontend dev

dev-admin-frontend:
	pnpm --filter @tragge/admin-frontend dev

build-frontends:
	pnpm -r build

test-frontends:
	pnpm -r test

lint-frontends:
	pnpm -r lint

# =============================================================================
# Go service targets
# =============================================================================

dev-user-bff:
	go run ./apps/user-bff/...

dev-trade-bff:
	go run ./apps/trade-bff/...

dev-admin-bff:
	go run ./apps/admin-bff/...

dev-trading-engine:
	go run ./apps/trading-engine/...

dev-market-ingestor:
	go run ./apps/market-ingestor/...

dev-leaderboard-worker:
	go run ./apps/leaderboard-worker/...

dev-payment-service:
	go run ./apps/payment-service/...

dev-contest-scheduler:
	go run ./apps/contest-scheduler/...

dev-free-contest-generator:
	go run ./apps/free-contest-generator/...

dev-settlement-service:
	go run ./apps/settlement-service/...

build-go:
	@echo "Building Go services (parallel)..."
	@go build ./apps/api-server/... & \
	go build ./apps/trading-core/... & \
	go build ./apps/worker/... & \
	go build ./apps/user-bff/... & \
	go build ./apps/trade-bff/... & \
	go build ./apps/admin-bff/... & \
	go build ./apps/trading-engine/... & \
	go build ./apps/market-ingestor/... & \
	go build ./apps/leaderboard-worker/... & \
	go build ./apps/payment-service/... & \
	go build ./apps/contest-scheduler/... & \
	go build ./apps/free-contest-generator/... & \
	go build ./apps/settlement-service/... & \
	wait
	@echo "All services built"

test-go:
	@echo "Running Go tests across workspace modules..."
	go test ./packages/auth/...
	go test ./packages/validation/...
	go test ./packages/secrets/...
	go test ./packages/contracts/...
	go test ./packages/config/...
	go test ./packages/resilience/...
	go test ./packages/scoring/...
	go test ./packages/domain/...
	go test ./packages/observability/...
	go test ./packages/infra/...
	go test ./packages/redis/...
	go test ./packages/notification/...
	go test ./apps/user-bff/...
	go test ./apps/trade-bff/...
	go test ./apps/admin-bff/...
	go test ./apps/trading-engine/...
	go test ./apps/market-ingestor/...
	go test ./apps/leaderboard-worker/...
	go test ./apps/payment-service/...
	go test ./apps/settlement-service/...
	go test ./apps/contest-scheduler/...
	go test ./apps/free-contest-generator/...

# Run golangci-lint with configured linters
# Iterates over each module in go.work because golangci-lint doesn't support Go workspaces natively
# Requires: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
GO_WORK_MODULES := $(shell grep '^\s*\./' go.work | tr -d '\t ')

lint-go:
	@for dir in $(GO_WORK_MODULES); do \
		echo "Linting $$dir..."; \
		(cd $$dir && golangci-lint run --timeout 5m ./...) || exit 1; \
	done

# Run golangci-lint with auto-fix for supported linters (gofmt, goimports)
lint-go-fix:
	@for dir in $(GO_WORK_MODULES); do \
		echo "Linting (fix) $$dir..."; \
		(cd $$dir && golangci-lint run --fix --timeout 5m ./...) || exit 1; \
	done

# Run golangci-lint showing only new issues (for CI on PRs)
lint-go-new:
	@for dir in $(GO_WORK_MODULES); do \
		echo "Linting (new) $$dir..."; \
		(cd $$dir && golangci-lint run --timeout 5m --new-from-rev=origin/main ./...) || exit 1; \
	done

# =============================================================================
# Secrets management
# =============================================================================

verify-secrets:
	@./scripts/secrets/verify-db-credentials.sh

init-secrets:
	@./scripts/secrets/init-secrets.sh

generate-db-credentials:
	@./scripts/secrets/generate-db-credentials.sh

# =============================================================================
# Docker targets
# =============================================================================

COMPOSE_FILE := infra/docker/docker-compose.yml
COMPOSE_LITE := infra/docker/docker-compose.lite.yml

# Pre-pull Docker base images with retry logic (helps with TLS timeout issues)
pull-images:
	@./scripts/pull-docker-images.sh

# Fix Docker TLS issues by configuring registry mirrors (requires sudo)
fix-docker-tls:
	@echo "This command requires sudo to modify Docker daemon configuration"
	sudo ./scripts/fix-docker-tls.sh

# Start all services (full mode — all 29 containers)
up:
	docker compose -f $(COMPOSE_FILE) --profile full up -d

down:
	docker compose -f $(COMPOSE_FILE) --profile full down

logs:
	docker compose -f $(COMPOSE_FILE) --profile full logs -f

ps:
	docker compose -f $(COMPOSE_FILE) --profile full ps

# Minimal mode: infrastructure only (~700MB RAM)
# Starts: postgres, redis, redpanda, kafka-init, minio
# Run Go services locally with 'make dev-*' commands
up-minimal:
	docker compose -f $(COMPOSE_FILE) -f $(COMPOSE_LITE) up -d

down-minimal:
	docker compose -f $(COMPOSE_FILE) -f $(COMPOSE_LITE) down

# Lite mode: core app + frontends, no monitoring (~1.5GB RAM)
# Starts: infrastructure + BFFs + trading-engine + market-ingestor + leaderboard + scheduler + frontends + gateway
up-lite:
	docker compose -f $(COMPOSE_FILE) -f $(COMPOSE_LITE) --profile app --profile frontend up -d

down-lite:
	docker compose -f $(COMPOSE_FILE) -f $(COMPOSE_LITE) --profile app --profile frontend down

# Full mode with resource limits (~3GB RAM)
# Starts all 29 services but with memory/CPU caps
up-full-lite:
	docker compose -f $(COMPOSE_FILE) -f $(COMPOSE_LITE) --profile full up -d

down-full-lite:
	docker compose -f $(COMPOSE_FILE) -f $(COMPOSE_LITE) --profile full down

build-gateway:
	docker build -t tragge-gateway apps/gateway

# Run service health checks across all platform components
# Optional: HEALTH_FLAGS (e.g., --verbose, --json)
check-health:
	@./scripts/check-health.sh $(HEALTH_FLAGS)

# =============================================================================
# Redis High Availability targets
# =============================================================================

COMPOSE_SENTINEL := infra/docker/docker-compose.yml -f infra/docker/docker-compose.redis-sentinel.yml
COMPOSE_CLUSTER := infra/docker/docker-compose.yml -f infra/docker/docker-compose.redis-cluster.yml

# Start infrastructure with Redis Sentinel HA
up-redis-sentinel:
	docker compose -f $(COMPOSE_SENTINEL) up -d
	@echo ""
	@echo "Redis Sentinel HA started!"
	@echo "Master: redis-master:6379"
	@echo "Sentinels: redis-sentinel-1:26379, redis-sentinel-2:26379, redis-sentinel-3:26379"
	@echo ""
	@echo "Check status: make redis-sentinel-status"

# Start infrastructure with Redis Cluster HA
up-redis-cluster:
	docker compose -f $(COMPOSE_CLUSTER) up -d
	@echo ""
	@echo "Redis Cluster HA started!"
	@echo "Nodes: redis-node-1:6379 through redis-node-6:6379"
	@echo ""
	@echo "Cluster init will run automatically. Check status: make redis-cluster-status"

# Stop Redis Sentinel HA
down-redis-sentinel:
	docker compose -f $(COMPOSE_SENTINEL) down

# Stop Redis Cluster HA
down-redis-cluster:
	docker compose -f $(COMPOSE_CLUSTER) down

# Check Redis Sentinel status
redis-sentinel-status:
	@echo "=== Sentinel Master Info ==="
	@docker exec tragge_sentinel_1 redis-cli -p 26379 sentinel master mymaster 2>/dev/null || echo "Sentinels not running"
	@echo ""
	@echo "=== Current Master ==="
	@docker exec tragge_sentinel_1 redis-cli -p 26379 sentinel get-master-addr-by-name mymaster 2>/dev/null || echo "Not available"

# Check Redis Cluster status
redis-cluster-status:
	@echo "=== Cluster Info ==="
	@docker exec tragge_redis_node_1 redis-cli cluster info 2>/dev/null || echo "Cluster not running"
	@echo ""
	@echo "=== Cluster Nodes ==="
	@docker exec tragge_redis_node_1 redis-cli cluster nodes 2>/dev/null || echo "Not available"

# Trigger Sentinel failover (for testing)
redis-sentinel-failover:
	@echo "Triggering Sentinel failover..."
	@docker exec tragge_sentinel_1 redis-cli -p 26379 sentinel failover mymaster
	@sleep 2
	@make redis-sentinel-status

# =============================================================================
# Database migration targets
# =============================================================================

# Database connection string for local development
# IMPORTANT: Set POSTGRES_PASSWORD environment variable or use .env file
# Do not use weak default credentials in production
DATABASE_URL ?= $(if $(POSTGRES_DSN),$(POSTGRES_DSN),postgres://$(or $(POSTGRES_USER),app):$(POSTGRES_PASSWORD)@localhost:5432/$(or $(POSTGRES_DB),app)?sslmode=$(or $(POSTGRES_SSLMODE),disable))
MIGRATIONS_PATH := packages/db/migrations

migrate-up:
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" down 1

migrate-version:
	migrate -path $(MIGRATIONS_PATH) -database "$(DATABASE_URL)" version

migrate-create:
ifndef NAME
	$(error NAME is required. Usage: make migrate-create NAME=your_migration_name)
endif
	migrate create -ext sql -dir $(MIGRATIONS_PATH) -seq $(NAME)

# =============================================================================
# Load testing targets
# =============================================================================

# WebSocket load test
# Required: EMAIL, PASSWORD, CONTEST_ID
# Optional: N (connections, default 100), DURATION (default 60s)
load-test-ws:
ifndef EMAIL
	$(error EMAIL is required. Usage: make load-test-ws EMAIL=... PASSWORD=... CONTEST_ID=...)
endif
ifndef PASSWORD
	$(error PASSWORD is required. Usage: make load-test-ws EMAIL=... PASSWORD=... CONTEST_ID=...)
endif
ifndef CONTEST_ID
	$(error CONTEST_ID is required. Usage: make load-test-ws EMAIL=... PASSWORD=... CONTEST_ID=...)
endif
	cd tools/ws-load-test && go run . \
		-n $(or $(N),100) \
		-email $(EMAIL) \
		-password $(PASSWORD) \
		-contest-id $(CONTEST_ID) \
		-duration $(or $(DURATION),60s) \
		-ramp-up $(or $(RAMP_UP),10s)

# WebSocket storm test (1000 concurrent connections)
# Required: EMAIL, PASSWORD, CONTEST_ID
load-test-ws-storm:
ifndef EMAIL
	$(error EMAIL is required. Usage: make load-test-ws-storm EMAIL=... PASSWORD=... CONTEST_ID=...)
endif
ifndef PASSWORD
	$(error PASSWORD is required. Usage: make load-test-ws-storm EMAIL=... PASSWORD=... CONTEST_ID=...)
endif
ifndef CONTEST_ID
	$(error CONTEST_ID is required. Usage: make load-test-ws-storm EMAIL=... PASSWORD=... CONTEST_ID=...)
endif
	cd tools/ws-load-test && go run . \
		-n 1000 \
		-email $(EMAIL) \
		-password $(PASSWORD) \
		-contest-id $(CONTEST_ID) \
		-duration $(or $(DURATION),120s) \
		-ramp-up 30s \
		-compression true

# Leaderboard load test (500 simultaneous requests)
# Required: EMAIL, PASSWORD, CONTEST_IDS
# Optional: CONCURRENT (default 500), DURATION (default 60s)
load-test-leaderboard:
ifndef EMAIL
	$(error EMAIL is required. Usage: make load-test-leaderboard EMAIL=... PASSWORD=... CONTEST_IDS=...)
endif
ifndef PASSWORD
	$(error PASSWORD is required. Usage: make load-test-leaderboard EMAIL=... PASSWORD=... CONTEST_IDS=...)
endif
ifndef CONTEST_IDS
	$(error CONTEST_IDS is required. Usage: make load-test-leaderboard EMAIL=... PASSWORD=... CONTEST_IDS=id1,id2,...)
endif
	cd tools/leaderboard-load-test && go run . \
		-concurrent $(or $(CONCURRENT),500) \
		-email $(EMAIL) \
		-password $(PASSWORD) \
		-contest-ids "$(CONTEST_IDS)" \
		-duration $(or $(DURATION),60s)

# Order burst test (100 orders/second across 10 contests)
# Required: CONTEST_IDS (10 contest IDs comma-separated)
# Optional: OPS (orders per second, default 100), NUM_CONTESTS (default 10)
load-test-order-burst:
ifndef CONTEST_IDS
	$(error CONTEST_IDS is required. Usage: make load-test-order-burst CONTEST_IDS=id1,id2,...id10)
endif
	cd tools/order-burst-test && go run . \
		-orders-per-second $(or $(OPS),100) \
		-num-contests $(or $(NUM_CONTESTS),10) \
		-contest-ids "$(CONTEST_IDS)" \
		-duration $(or $(DURATION),60s) \
		-users-per-contest $(or $(USERS_PER_CONTEST),5)

# Contest finalization test (prize distribution for 1000 participants)
# Optional: PARTICIPANTS (default 1000), TRIGGER (default kafka)
load-test-finalization:
	cd tools/finalization-load-test && go run . \
		-participants $(or $(PARTICIPANTS),1000) \
		-trigger $(or $(TRIGGER),kafka) \
		-entry-fee $(or $(ENTRY_FEE),1000) \
		-platform-fee-bps $(or $(PLATFORM_FEE_BPS),1000)

# Run all load tests in sequence (requires all parameters)
# Required: EMAIL, PASSWORD, CONTEST_ID, CONTEST_IDS
load-test-all:
ifndef EMAIL
	$(error EMAIL is required for load-test-all)
endif
ifndef PASSWORD
	$(error PASSWORD is required for load-test-all)
endif
ifndef CONTEST_ID
	$(error CONTEST_ID is required for load-test-all)
endif
ifndef CONTEST_IDS
	$(error CONTEST_IDS is required for load-test-all)
endif
	@echo "=== Running All Load Tests ==="
	@echo ""
	@echo "1/4: WebSocket Storm Test (1000 connections)"
	@make load-test-ws-storm EMAIL=$(EMAIL) PASSWORD=$(PASSWORD) CONTEST_ID=$(CONTEST_ID) DURATION=60s
	@echo ""
	@echo "2/4: Order Burst Test (100 orders/sec across 10 contests)"
	@make load-test-order-burst CONTEST_IDS="$(CONTEST_IDS)" DURATION=60s
	@echo ""
	@echo "3/4: Leaderboard Load Test (500 simultaneous requests)"
	@make load-test-leaderboard EMAIL=$(EMAIL) PASSWORD=$(PASSWORD) CONTEST_IDS="$(CONTEST_IDS)" DURATION=60s
	@echo ""
	@echo "4/4: Contest Finalization Test (1000 participants)"
	@make load-test-finalization PARTICIPANTS=1000
	@echo ""
	@echo "=== All Load Tests Complete ==="

# =============================================================================
# Chaos engineering targets
# =============================================================================

# List available chaos test scenarios
chaos-test-list:
	cd tools/chaos-test && go run . -list

# Run a specific chaos test scenario
# Required: SCENARIO (e.g., pod-kill, network-partition)
# Optional: NAMESPACE (default: tragge), OUTPUT (default: text)
chaos-test:
ifndef SCENARIO
	$(error SCENARIO is required. Usage: make chaos-test SCENARIO=pod-kill [NAMESPACE=tragge])
endif
	cd tools/chaos-test && go run . \
		-scenario=$(SCENARIO) \
		-namespace=$(or $(NAMESPACE),tragge) \
		-output=$(or $(OUTPUT),text)

# Run all chaos test scenarios
chaos-test-all:
	cd tools/chaos-test && go run . \
		-scenario=all \
		-namespace=$(or $(NAMESPACE),tragge) \
		-output=$(or $(OUTPUT),text)

# Run chaos test with load (requires additional parameters)
# Required: SCENARIO, EMAIL, PASSWORD, CONTEST_ID
# Optional: NAMESPACE, LOAD_USERS, LOAD_DURATION
chaos-test-with-load:
ifndef SCENARIO
	$(error SCENARIO is required)
endif
ifndef EMAIL
	$(error EMAIL is required for load testing)
endif
ifndef PASSWORD
	$(error PASSWORD is required for load testing)
endif
ifndef CONTEST_ID
	$(error CONTEST_ID is required for load testing)
endif
	cd tools/chaos-test && go run . \
		-scenario=$(SCENARIO) \
		-namespace=$(or $(NAMESPACE),tragge) \
		-with-load \
		-load-users=$(or $(LOAD_USERS),50) \
		-load-duration=$(or $(LOAD_DURATION),5m) \
		-base-url=$(or $(BASE_URL),http://localhost:8080) \
		-email=$(EMAIL) \
		-password=$(PASSWORD) \
		-contest-id=$(CONTEST_ID) \
		-output=$(or $(OUTPUT),text)

# =============================================================================
# E2E testing targets (Playwright)
# =============================================================================

# Run all E2E tests
e2e:
	pnpm e2e

# Run user E2E tests only
e2e-user:
	pnpm e2e:user

# Run trade E2E tests only
e2e-trade:
	pnpm e2e:trade

# Run admin E2E tests only
e2e-admin:
	pnpm e2e:admin

# Open Playwright UI mode for interactive debugging
e2e-ui:
	pnpm e2e:ui

# Install Playwright browsers
e2e-install:
	pnpm e2e:install

# Show Playwright HTML test report
e2e-report:
	pnpm e2e:report

# Run E2E tests in headed mode (visible browser)
e2e-headed:
	pnpm e2e:headed

# Run E2E tests in debug mode
e2e-debug:
	pnpm e2e:debug

# =============================================================================
# Go E2E tests (contest lifecycle, full-stack integration)
# =============================================================================

# Run Go E2E tests against running docker-compose environment
e2e-go:
	@echo "Running Go E2E tests (requires docker-compose services)..."
	go test -v -timeout 5m ./tests/e2e/...

# Run Go E2E contest lifecycle test
e2e-lifecycle:
	@echo "Running contest lifecycle E2E test..."
	go test -v -timeout 5m -run TestContestLifecycle ./tests/e2e/...

# =============================================================================
# Notification testing targets
# =============================================================================

# Run notification package unit tests
test-notification-unit:
	@echo "Running notification package unit tests..."
	go test -v -race -coverprofile=notification-coverage.out ./packages/notification/...

# Run notification integration tests with mocked services
# These tests use mock servers and don't require external services
test-notification-integration:
	@echo "Running notification integration tests (mocked services)..."
	cd tests/integration && go test -v -tags=integration \
		-run 'TestNotification_(ServiceIntegration|UnderLoad|Fixtures|CircuitBreaker|GracefulShutdown|NoopNotifier)' \
		./...

# Run notification integration tests with real services
# Requires environment variables:
#   DISCORD_TEST_WEBHOOK_URL - Discord webhook URL for test channel
#   RESEND_TEST_API_KEY - Resend API key
#   RESEND_TEST_EMAIL - Email address to send test emails to
test-notification-integration-real:
	@echo "Running notification integration tests (real services)..."
	@if [ -z "$(DISCORD_TEST_WEBHOOK_URL)" ] && [ -z "$(RESEND_TEST_API_KEY)" ]; then \
		echo "WARNING: No real service credentials provided. Set DISCORD_TEST_WEBHOOK_URL or RESEND_TEST_API_KEY."; \
		echo "Skipping real integration tests."; \
		exit 0; \
	fi
	cd tests/integration && go test -v -tags=integration \
		-run 'TestNotification_(Discord_RealWebhook|Resend_RealEmail)' \
		./...

# Run all notification tests
test-notification-all: test-notification-unit test-notification-integration
	@echo ""
	@echo "=== All notification tests completed ==="

# =============================================================================
# Development Profiles (start only what you need)
# =============================================================================

# User flows: auth, dashboard, contests, wallet
dev-user:
	@trap 'kill 0' INT TERM; \
	$(MAKE) dev-user-bff & \
	$(MAKE) dev-frontend & \
	wait

# Trading flow: engine + ingestor + trade BFF + leaderboard
dev-trade:
	@trap 'kill 0' INT TERM; \
	$(MAKE) dev-trade-bff & \
	$(MAKE) dev-trading-engine & \
	$(MAKE) dev-market-ingestor & \
	$(MAKE) dev-leaderboard-worker & \
	$(MAKE) dev-frontend & \
	wait

# Admin panel only
dev-admin:
	@trap 'kill 0' INT TERM; \
	$(MAKE) dev-admin-bff & \
	$(MAKE) dev-frontend & \
	wait

# All backend services (no frontends)
dev-backend:
	@trap 'kill 0' INT TERM; \
	$(MAKE) dev-user-bff & \
	$(MAKE) dev-trade-bff & \
	$(MAKE) dev-admin-bff & \
	$(MAKE) dev-trading-engine & \
	$(MAKE) dev-market-ingestor & \
	$(MAKE) dev-leaderboard-worker & \
	wait
