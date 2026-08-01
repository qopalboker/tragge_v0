module github.com/Parsaeffatravesh/tragge/tools/tick-simulator

go 1.24.0

toolchain go1.24.7

require (
	github.com/Parsaeffatravesh/tragge/packages/contracts v0.0.0
	github.com/twmb/franz-go v1.18.1
)

require (
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/pierrec/lz4/v4 v4.1.22 // indirect
	github.com/twmb/franz-go/pkg/kmsg v1.9.0 // indirect
	golang.org/x/crypto v0.43.0 // indirect
)

replace github.com/Parsaeffatravesh/tragge/packages/contracts => ../../packages/contracts
