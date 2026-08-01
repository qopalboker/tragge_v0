module github.com/Parsaeffatravesh/tragge/packages/notification

go 1.24.0

toolchain go1.24.7

require (
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/Parsaeffatravesh/tragge/packages/observability v0.0.0
	github.com/google/uuid v1.6.0
	github.com/gtuk/discordwebhook v1.2.0
	github.com/lib/pq v1.12.0
	github.com/prometheus/client_golang v1.19.0
	github.com/resend/resend-go/v2 v2.13.0
	github.com/stretchr/testify v1.11.1
	go.uber.org/zap v1.27.0
)

replace github.com/Parsaeffatravesh/tragge/packages/observability => ../observability

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.6.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
