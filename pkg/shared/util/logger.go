package util

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type ContextKey string

const (
	RequestIDKey ContextKey = "requestID"
	LoggerKey    ContextKey = "logger"
) // Key for storing the zerolog logger in the context

func GetLoggerFromCtx(ctx context.Context) zerolog.Logger {
	if logger, ok := ctx.Value(LoggerKey).(zerolog.Logger); ok {
		return logger
	}
	return log.Logger // fallback to global logger
}
