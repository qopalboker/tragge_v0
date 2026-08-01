package notification

import (
	"fmt"

	"github.com/Parsaeffatravesh/tragge/packages/observability"
	"go.uber.org/zap"
)

func ensureRedactingLogger(logger *zap.Logger) *zap.Logger {
	return observability.WrapLogger(logger)
}

func redactedPanicError(value any) error {
	return fmt.Errorf("panic: %s", observability.RedactPanic(value))
}
