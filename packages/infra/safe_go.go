package infra

import (
	"runtime/debug"
	"sync"

	"go.uber.org/zap"
)

// SafeGo launches a goroutine with panic recovery. If the goroutine panics,
// the panic is logged and the goroutine exits without crashing the process.
func SafeGo(logger *zap.Logger, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if logger != nil {
					logger.Error("goroutine panicked",
						zap.String("goroutine", name),
						zap.Any("panic", r),
						zap.String("stack", string(debug.Stack())),
					)
				}
			}
		}()
		fn()
	}()
}

// SafeGoWg launches a goroutine with panic recovery and WaitGroup support.
// It calls wg.Add(1) before launching and defers wg.Done() inside the goroutine.
func SafeGoWg(wg *sync.WaitGroup, logger *zap.Logger, name string, fn func()) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				if logger != nil {
					logger.Error("goroutine panicked",
						zap.String("goroutine", name),
						zap.Any("panic", r),
						zap.String("stack", string(debug.Stack())),
					)
				}
			}
		}()
		fn()
	}()
}
