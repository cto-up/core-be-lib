package service

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Define a custom type for the context key to avoid collisions
type contextKey string

const (
	RequestIDKey contextKey = "requestID"
	LoggerKey    contextKey = "logger" // Key for storing the zerolog logger in the context
)

// RequestIDMiddleware is a Gin middleware to add a unique request ID to each request.
// It also creates a request-scoped zerolog instance.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Store the request ID in the Gin context
		c.Set(string(RequestIDKey), requestID)

		// Set the X-Request-ID header in the response for clients/next services
		c.Writer.Header().Set("X-Request-ID", requestID)

		// Create a zerolog instance with the request ID
		requestLogger := log.With().
			Str("request_id", requestID).
			Logger()

		// Store the enriched logger in the Go context (c.Request.Context())
		// This is the idiomatic way to pass request-scoped values in Go.
		ctx := context.WithValue(c.Request.Context(), LoggerKey, requestLogger)
		c.Request = c.Request.WithContext(ctx)

		// Record the start time
		start := time.Now()

		// Process the request
		c.Next()

		// Calculate the time taken
		duration := time.Since(start)

		// Log the details
		requestLogger.Info().
			Str("method", c.Request.Method).
			Str("url", c.Request.URL.String()).
			Int("status", c.Writer.Status()).
			Dur("duration", duration).
			Msg("Request handled")
	}
}

func GetLoggerFromContext(c *gin.Context) zerolog.Logger {
	if logger, ok := c.Request.Context().Value(LoggerKey).(zerolog.Logger); ok {
		return logger
	}
	return log.Logger // Fallback to global logger
}
