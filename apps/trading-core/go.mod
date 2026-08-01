module github.com/Parsaeffatravesh/tragge/apps/trading-core

go 1.24.0

toolchain go1.24.7

require (
	github.com/Parsaeffatravesh/tragge/apps/market-ingestor v0.0.0
	github.com/Parsaeffatravesh/tragge/apps/trade-bff v0.0.0
	github.com/Parsaeffatravesh/tragge/apps/trading-engine v0.0.0
	github.com/Parsaeffatravesh/tragge/packages/db v0.0.0
	github.com/Parsaeffatravesh/tragge/packages/infra v0.0.0
	github.com/Parsaeffatravesh/tragge/packages/observability v0.0.0
	github.com/Parsaeffatravesh/tragge/packages/redis v0.0.0
	github.com/Parsaeffatravesh/tragge/packages/secrets v0.0.0
	go.uber.org/zap v1.27.0
)

require (
	github.com/Parsaeffatravesh/tragge/packages/auth v0.0.0 // indirect
	github.com/Parsaeffatravesh/tragge/packages/config v0.0.0 // indirect
	github.com/Parsaeffatravesh/tragge/packages/contracts v0.0.0 // indirect
	github.com/Parsaeffatravesh/tragge/packages/notification v0.0.0 // indirect
	github.com/Parsaeffatravesh/tragge/packages/scoring v0.0.0 // indirect
	github.com/Parsaeffatravesh/tragge/packages/validation v0.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/getsentry/sentry-go v0.27.0 // indirect
	github.com/go-chi/chi/v5 v5.1.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.1 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.19.0 // indirect
	github.com/gtuk/discordwebhook v1.2.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgx/v5 v5.6.0 // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/lib/pq v1.12.0 // indirect
	github.com/prometheus/client_golang v1.19.0 // indirect
	github.com/prometheus/client_model v0.6.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/redis/go-redis/v9 v9.7.3 // indirect
	github.com/resend/resend-go/v2 v2.13.0 // indirect
	github.com/shopspring/decimal v1.4.0 // indirect
	github.com/twmb/franz-go v1.18.1 // indirect
	github.com/twmb/franz-go/pkg/kadm v1.11.0 // indirect
	github.com/twmb/franz-go/pkg/kmsg v1.9.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel v1.35.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.24.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.24.0 // indirect
	go.opentelemetry.io/otel/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	go.opentelemetry.io/proto/otlp v1.1.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.43.0 // indirect
	golang.org/x/net v0.45.0 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240814211410-ddb44dafa142 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240903143218-8af14fe29dc1 // indirect
	google.golang.org/grpc v1.67.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)

replace (
	github.com/Parsaeffatravesh/tragge/apps/market-ingestor => ../market-ingestor
	github.com/Parsaeffatravesh/tragge/apps/trade-bff => ../trade-bff
	github.com/Parsaeffatravesh/tragge/apps/trading-engine => ../trading-engine
	github.com/Parsaeffatravesh/tragge/packages/auth => ../../packages/auth
	github.com/Parsaeffatravesh/tragge/packages/config => ../../packages/config
	github.com/Parsaeffatravesh/tragge/packages/contracts => ../../packages/contracts
	github.com/Parsaeffatravesh/tragge/packages/db => ../../packages/db
	github.com/Parsaeffatravesh/tragge/packages/kyc => ../../packages/kyc
	github.com/Parsaeffatravesh/tragge/packages/notification => ../../packages/notification
	github.com/Parsaeffatravesh/tragge/packages/observability => ../../packages/observability
	github.com/Parsaeffatravesh/tragge/packages/redis => ../../packages/redis
	github.com/Parsaeffatravesh/tragge/packages/resilience => ../../packages/resilience
	github.com/Parsaeffatravesh/tragge/packages/scoring => ../../packages/scoring
	github.com/Parsaeffatravesh/tragge/packages/secrets => ../../packages/secrets
	github.com/Parsaeffatravesh/tragge/packages/storage => ../../packages/storage
	github.com/Parsaeffatravesh/tragge/packages/validation => ../../packages/validation
	github.com/Parsaeffatravesh/tragge/packages/wallet => ../../packages/wallet
)

replace github.com/Parsaeffatravesh/tragge/packages/domain => ../../packages/domain

replace github.com/Parsaeffatravesh/tragge/packages/infra => ../../packages/infra
