package service

import (
	"context"
	"time"

	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// infraPaths are polled continuously by machines, never by a user, and a line
// each in Loki buries the traffic anyone actually reads. Prometheus scrapes
// /metrics every few seconds and the container runtime polls /healthz just as
// often; between them they would be the majority of the access log.
//
// Skipping them here rather than by registration order is deliberate: it makes
// the exclusion independent of where router.Use() sits relative to those two
// routes, so the access log can be placed outside gin.Recovery() — the only
// position from which it can observe a recovered panic. HTTPMetricsMiddleware
// excludes /metrics from its own histogram the same way.
var infraPaths = map[string]bool{
	"/metrics": true,
	"/healthz": true,
}

// RequestIDMiddleware is a Gin middleware to add a unique request ID to each request.
// It also creates a request-scoped zerolog instance and emits the application
// access log once the request has been handled.
//
// MUST be installed with router.Use(), NOT via APIOptions.Middlewares — see the
// block in initializeServerConfig, and the regression tests in
// request_id_middleware_test.go that fail if it moves back.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if infraPaths[c.Request.URL.Path] {
			c.Next()
			return
		}

		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Store the request ID in the Gin context
		c.Set(string(util.RequestIDKey), requestID)

		// Set the X-Request-ID header in the response for clients/next services
		c.Writer.Header().Set("X-Request-ID", requestID)

		// Create a zerolog instance with the request ID
		requestLogger := log.With().
			Str("request_id", requestID).
			Logger()

		// Store the enriched logger in the Go context (c.Request.Context())
		// This is the idiomatic way to pass request-scoped values in Go.
		ctx := context.WithValue(c.Request.Context(), util.RequestIDKey, requestID)
		ctx = context.WithValue(ctx, util.LoggerKey, requestLogger)
		c.Request = c.Request.WithContext(ctx)

		// Record the start time
		start := time.Now()

		// Process the request
		c.Next()

		// Calculate the time taken
		duration := time.Since(start)

		// Re-read the context logger so the summary line picks up any fields
		// added downstream (e.g. tenant_id/user_id from LoggerEnrichmentMiddleware).
		summaryLogger := util.GetLoggerFromCtx(c.Request.Context())

		// THIS LINE IS THE APPLICATION ACCESS LOG (roadmap 020, Tier 1.2).
		//
		// It goes to the request-scoped zerolog logger, which the app writes to
		// its lumberjack file — the file promtail already ships to Loki. gin's
		// own default logger computes the same latency and writes it to stdout,
		// where nothing scrapes it; NewServerConfig therefore builds the engine
		// with gin.New() rather than gin.Default() so there is exactly one
		// access log and it lands somewhere queryable.
		//
		// `route` is the gin route TEMPLATE, added so a LogQL query can group by
		// endpoint. `url` is kept alongside it because the raw path (and query)
		// is what you actually want when reading one slow request — but it must
		// never be promoted to a Loki LABEL, for the same cardinality reason
		// that `route` exists.
		//
		// Paired with request_id (set above from nginx's X-Request-ID when
		// present), a slow line in the nginx timing log joins directly to the
		// application log line, the tenant and the user that produced it:
		//   {job="zerolog"} | json | duration_ms > 1000 | tenant_id="..."
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		summaryLogger.Info().
			Str("method", c.Request.Method).
			Str("route", route).
			Str("url", c.Request.URL.String()).
			Int("status", c.Writer.Status()).
			Float64("duration_ms", float64(duration.Microseconds())/1000.0).
			Dur("duration", duration).
			Msg("Request handled")
	}
}

func GetLoggerFromContext(c *gin.Context) zerolog.Logger {
	if logger, ok := c.Request.Context().Value(util.LoggerKey).(zerolog.Logger); ok {
		return logger
	}
	return log.Logger // Fallback to global logger
}
