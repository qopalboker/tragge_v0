module github.com/Parsaeffatravesh/tragge/packages/resilience

go 1.24.0

toolchain go1.24.7

require (
	github.com/Parsaeffatravesh/tragge/packages/observability v0.0.0
	github.com/prometheus/client_golang v1.19.0
	github.com/redis/go-redis/v9 v9.7.3
	go.uber.org/zap v1.27.0
)

replace github.com/Parsaeffatravesh/tragge/packages/observability => ../observability

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/prometheus/client_model v0.6.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)
