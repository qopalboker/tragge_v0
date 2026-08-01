package observability

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewRedactingCore sanitizes immediately before zap encodes an entry.
func NewRedactingCore(core zapcore.Core) zapcore.Core {
	if core == nil {
		return nil
	}
	if _, ok := core.(redactingCore); ok {
		return core
	}
	return redactingCore{Core: core}
}

// WrapLogger protects transitional constructors that cannot yet use NewLogger.
func WrapLogger(logger *zap.Logger) *zap.Logger {
	if logger == nil {
		return nil
	}
	return logger.WithOptions(zap.WrapCore(NewRedactingCore))
}

type redactingCore struct{ zapcore.Core }

func (core redactingCore) With(fields []zap.Field) zapcore.Core {
	return redactingCore{Core: core.Core.With(RedactFields(fields))}
}

func (core redactingCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if !core.Enabled(entry.Level) {
		return checked
	}
	entry.Message = RedactText(entry.Message)
	return checked.AddCore(entry, core)
}

func (core redactingCore) Write(entry zapcore.Entry, fields []zap.Field) error {
	entry.Message = RedactText(entry.Message)
	return core.Core.Write(entry, RedactFields(fields))
}

// RedactFields sanitizes structured values and conservatively replaces opaque
// marshalers whose nested output cannot be inspected safely.
func RedactFields(fields []zap.Field) []zap.Field {
	result := make([]zap.Field, len(fields))
	for i, field := range fields {
		if IsSensitiveKey(field.Key) {
			result[i] = zap.String(field.Key, RedactedValue)
			continue
		}
		switch field.Type {
		case zapcore.StringType:
			field.String = RedactText(field.String)
			result[i] = field
		case zapcore.ByteStringType:
			if value, ok := field.Interface.([]byte); ok {
				result[i] = zap.String(field.Key, RedactText(string(value)))
			} else {
				result[i] = zap.String(field.Key, RedactedValue)
			}
		case zapcore.ErrorType:
			if value, ok := field.Interface.(error); ok {
				result[i] = zap.String(field.Key, RedactText(value.Error()))
			} else {
				result[i] = zap.String(field.Key, RedactedValue)
			}
		case zapcore.ReflectType:
			result[i] = zap.Any(field.Key, RedactValue(field.Interface))
		case zapcore.ObjectMarshalerType, zapcore.ArrayMarshalerType, zapcore.BinaryType:
			result[i] = zap.String(field.Key, RedactedValue)
		default:
			result[i] = field
		}
	}
	return result
}
