module github.com/Parsaeffatravesh/tragge/packages/infra

go 1.24.0

toolchain go1.24.7

require (
	github.com/Parsaeffatravesh/tragge/packages/config v0.0.0
	github.com/go-chi/chi/v5 v5.1.0
	github.com/redis/go-redis/v9 v9.7.3
	github.com/twmb/franz-go v1.18.1
	go.uber.org/zap v1.27.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	github.com/twmb/franz-go/pkg/kmsg v1.9.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.43.0 // indirect
)

replace github.com/Parsaeffatravesh/tragge/packages/config => ../config
