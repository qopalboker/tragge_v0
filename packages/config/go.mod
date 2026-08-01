module github.com/Parsaeffatravesh/tragge/packages/config

go 1.24.0

toolchain go1.24.7

require (
	github.com/Parsaeffatravesh/tragge/packages/observability v0.0.0
	github.com/prometheus/client_golang v1.19.0
)

replace github.com/Parsaeffatravesh/tragge/packages/observability => ../observability

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/prometheus/client_model v0.6.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)
