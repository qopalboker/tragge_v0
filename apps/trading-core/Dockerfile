# ---- Build ----
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /build

# Cache dependencies (changes rarely)
COPY go.work go.work.sum ./
COPY packages/ packages/
COPY apps/admin-bff/go.mod apps/admin-bff/go.sum* apps/admin-bff/
COPY apps/api-server/go.mod apps/api-server/go.sum* apps/api-server/
COPY apps/contest-scheduler/go.mod apps/contest-scheduler/go.sum* apps/contest-scheduler/
COPY apps/free-contest-generator/go.mod apps/free-contest-generator/go.sum* apps/free-contest-generator/
COPY apps/leaderboard-worker/go.mod apps/leaderboard-worker/go.sum* apps/leaderboard-worker/
COPY apps/market-ingestor/go.mod apps/market-ingestor/go.sum* apps/market-ingestor/
COPY apps/payment-service/go.mod apps/payment-service/go.sum* apps/payment-service/
COPY apps/settlement-service/go.mod apps/settlement-service/go.sum* apps/settlement-service/
COPY apps/trade-bff/go.mod apps/trade-bff/go.sum* apps/trade-bff/
COPY apps/trading-core/go.mod apps/trading-core/go.sum* apps/trading-core/
COPY apps/trading-engine/go.mod apps/trading-engine/go.sum* apps/trading-engine/
COPY apps/user-bff/go.mod apps/user-bff/go.sum* apps/user-bff/
COPY apps/worker/go.mod apps/worker/go.sum* apps/worker/
RUN cd apps/trading-core && go mod download

# Build
COPY apps/ apps/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -trimpath \
    -o /server ./apps/trading-core/...

# Create non-root user for scratch image
RUN echo "nonroot:x:65534:65534:nonroot:/:/sbin/nologin" > /etc/passwd.minimal

# ---- Runtime (busybox = ~1 MB, has wget for healthchecks) ----
FROM busybox:stable-musl
COPY --from=builder /etc/passwd.minimal /etc/passwd
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /server /server
USER 65534:65534
EXPOSE 8082 8084 8085
ENTRYPOINT ["/server"]
