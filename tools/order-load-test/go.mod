module github.com/Parsaeffatravesh/tragge/tools/order-load-test

go 1.24.0

toolchain go1.24.7

require (
	github.com/Parsaeffatravesh/tragge/packages/contracts v0.0.0
	github.com/gorilla/websocket v1.5.1
	golang.org/x/sync v0.17.0
)

require golang.org/x/net v0.45.0 // indirect

replace github.com/Parsaeffatravesh/tragge/packages/contracts => ../../packages/contracts
