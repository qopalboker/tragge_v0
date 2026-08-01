# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git ca-certificates tzdata

# Copy go.work and all go.mod/go.sum for dependency caching
COPY go.work go.work.sum ./

# -- Apps --
COPY apps/admin-bff/go.mod apps/admin-bff/go.sum* ./apps/admin-bff/
COPY apps/contest-scheduler/go.mod apps/contest-scheduler/go.sum* ./apps/contest-scheduler/
COPY apps/free-contest-generator/go.mod apps/free-contest-generator/go.sum* ./apps/free-contest-generator/
COPY apps/leaderboard-worker/go.mod apps/leaderboard-worker/go.sum* ./apps/leaderboard-worker/
COPY apps/market-ingestor/go.mod apps/market-ingestor/go.sum* ./apps/market-ingestor/
COPY apps/payment-service/go.mod apps/payment-service/go.sum* ./apps/payment-service/
COPY apps/settlement-service/go.mod apps/settlement-service/go.sum* ./apps/settlement-service/
COPY apps/shard-router/go.mod apps/shard-router/go.sum* ./apps/shard-router/
COPY apps/trade-bff/go.mod apps/trade-bff/go.sum* ./apps/trade-bff/
COPY apps/trading-engine/go.mod apps/trading-engine/go.sum* ./apps/trading-engine/
COPY apps/user-bff/go.mod apps/user-bff/go.sum* ./apps/user-bff/

# -- Packages --
COPY packages/auth/go.mod packages/auth/go.sum* ./packages/auth/
# circuitbreaker moved into packages/resilience as a sub-package — no
# separate go.mod, pulled in via the resilience COPY below and the
# packages/ source COPY further down.
COPY packages/contracts/go.mod packages/contracts/go.sum* ./packages/contracts/
COPY packages/db/go.mod packages/db/go.sum* ./packages/db/
COPY packages/health/go.mod packages/health/go.sum* ./packages/health/
COPY packages/inapp/go.mod packages/inapp/go.sum* ./packages/inapp/
COPY packages/kyc/go.mod packages/kyc/go.sum* ./packages/kyc/
COPY packages/notification/go.mod packages/notification/go.sum* ./packages/notification/
COPY packages/observability/go.mod packages/observability/go.sum* ./packages/observability/
COPY packages/ratelimit/go.mod packages/ratelimit/go.sum* ./packages/ratelimit/
COPY packages/redis/go.mod packages/redis/go.sum* ./packages/redis/
COPY packages/resilience/go.mod packages/resilience/go.sum* ./packages/resilience/
COPY packages/scoring/go.mod packages/scoring/go.sum* ./packages/scoring/
COPY packages/secrets/go.mod packages/secrets/go.sum* ./packages/secrets/
COPY packages/shard/go.mod packages/shard/go.sum* ./packages/shard/
COPY packages/statemachine/go.mod packages/statemachine/go.sum* ./packages/statemachine/
COPY packages/traggepoint/go.mod packages/traggepoint/go.sum* ./packages/traggepoint/
COPY packages/validation/go.mod packages/validation/go.sum* ./packages/validation/
COPY packages/wallet/go.mod packages/wallet/go.sum* ./packages/wallet/
COPY packages/wsregistry/go.mod packages/wsregistry/go.sum* ./packages/wsregistry/
COPY packages/config/go.mod packages/config/go.sum* ./packages/config/
COPY packages/exchangerate/go.mod packages/exchangerate/go.sum* ./packages/exchangerate/
COPY packages/prize/go.mod packages/prize/go.sum* ./packages/prize/
COPY packages/prize-distribution/go.mod packages/prize-distribution/go.sum* ./packages/prize-distribution/
COPY packages/storage/go.mod packages/storage/go.sum* ./packages/storage/

# -- Scripts --
COPY scripts/create-admin-users/go.mod scripts/create-admin-users/go.sum* ./scripts/create-admin-users/

# -- Tests --
COPY tests/integration/go.mod tests/integration/go.sum* ./tests/integration/
COPY tests/contract/go.mod tests/contract/go.sum* ./tests/contract/
COPY tests/e2e/go.mod tests/e2e/go.sum* ./tests/e2e/

# -- Tools --
COPY tools/chaos-test/go.mod tools/chaos-test/go.sum* ./tools/chaos-test/
COPY tools/finalization-load-test/go.mod tools/finalization-load-test/go.sum* ./tools/finalization-load-test/
COPY tools/leaderboard-load-test/go.mod tools/leaderboard-load-test/go.sum* ./tools/leaderboard-load-test/
COPY tools/order-burst-test/go.mod tools/order-burst-test/go.sum* ./tools/order-burst-test/
COPY tools/order-load-test/go.mod tools/order-load-test/go.sum* ./tools/order-load-test/
COPY tools/shard-load-test/go.mod tools/shard-load-test/go.sum* ./tools/shard-load-test/
COPY tools/tick-simulator/go.mod tools/tick-simulator/go.sum* ./tools/tick-simulator/
COPY tools/ws-load-test/go.mod tools/ws-load-test/go.sum* ./tools/ws-load-test/

# Download all workspace dependencies
RUN go mod download

# Copy source code
COPY apps/shard-router/ ./apps/shard-router/
COPY packages/ ./packages/

# Build the application
WORKDIR /build/apps/shard-router
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /shard-router .

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

RUN adduser -D -u 65534 nonroot

COPY --from=builder /shard-router /shard-router

USER nonroot
EXPOSE 8090

ENTRYPOINT ["/shard-router"]
