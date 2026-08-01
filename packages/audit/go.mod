module github.com/Parsaeffatravesh/tragge/packages/audit

go 1.24.0

toolchain go1.24.7

require (
	github.com/Parsaeffatravesh/tragge/packages/observability v0.0.0
	github.com/Parsaeffatravesh/tragge/packages/validation v0.0.0
	go.uber.org/zap v1.27.0
)

replace github.com/Parsaeffatravesh/tragge/packages/observability => ../observability

require (
	github.com/stretchr/testify v1.11.1 // indirect
	go.uber.org/multierr v1.11.0 // indirect
)
