package aiutil

import (
	"context"

	"github.com/rs/zerolog"
)

// LoggerFromContext returns the logger from the context if available,
// otherwise falls back to the provided logger.
func LoggerFromContext(ctx context.Context, fallback *zerolog.Logger) *zerolog.Logger {
	if ctx != nil {
		if ctxLog := zerolog.Ctx(ctx); ctxLog != nil && ctxLog.GetLevel() != zerolog.Disabled {
			return ctxLog
		}
	}
	return fallback
}
