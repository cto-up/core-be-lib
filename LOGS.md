## Guide: Request ID & Structured Logging with Zerolog

Here's a quick guide on how to leverage the **`request_id`** and structured logging (using `zerolog`) in ctoup's Go Gin applications. This feature is crucial for debugging, monitoring, and tracing requests through our services, especially in a distributed environment.

### What is a Request ID?

A **`request_id`** (also known as a Correlation ID or Trace ID) is a unique identifier assigned to each incoming HTTP request. This ID is propagated throughout the entire lifecycle of the request, even if it spans multiple services.

**Benefits:**

- **Log Correlation:** Easily group all log messages related to a single request.
- **Distributed Tracing:** Track a request's journey across different microservices.
- **Debugging:** Quickly pinpoint issues by filtering logs with a specific `request_id`.
- **Observability:** Improved insights into application behavior and performance.

### How It's Implemented

We use a Gin middleware (`RequestIDMiddleware`) to:

1.  Check for an existing `X-Request-ID` header from the client or upstream service.
2.  If not present, generate a new UUID for the `request_id`.
3.  Store this `request_id` in the `gin.Context` for easy access.
4.  Crucially, it creates a **request-scoped `zerolog.Logger` instance** that automatically includes the `request_id` in every log message. This enriched logger is then stored in the `context.Context` associated with the request.
5.  The `X-Request-ID` header is also set in the response, so clients or downstream services can continue the trace.

---

### How to Use It (For Developers)

#### 1. Getting the Request-Scoped Logger

In any of your Gin handlers or functions called within a request's lifecycle, retrieve the logger using **`GetLoggerFromContext`**:

```go
import (
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	// ... other imports
)

// In your handler:
func MyHandler(c *gin.Context) {
    logger := GetLoggerFromContext(c)
    // Now use 'logger' for all your logging within this request
    logger.Info().
        Str("user_id", "123").
        Msg("Processing user request")

    // ... your handler logic
}

// In a helper function (that accepts context.Context):
func performDatabaseOperation(ctx context.Context, data string) error {
    // You'll need to retrieve the zerolog.Logger from the plain context.Context
    // This assumes the context passed to this function originated from c.Request.Context()
    var logger zerolog.Logger
    if l, ok := ctx.Value(LoggerKey).(zerolog.Logger); ok { // LoggerKey is defined in constants
        logger = l
    } else {
        logger = zerolog.Nop() // Fallback to a "no-op" logger if not found, or global logger
        // For production, you might want to log a warning here or always ensure it's present.
    }

    logger.Debug().
        Str("data_to_save", data).
        Msg("Attempting to save data to database")

    // ... database logic
    return nil
}
```

#### 2. Logging Best Practices with Zerolog

Always use the request-scoped logger (`logger` from `GetLoggerFromContext`) and leverage `zerolog`'s structured logging capabilities:

- **Info Level:** For general information about the request flow.
  ```go
  logger.Info().Msg("User successfully logged in")
  ```
- **Debug Level:** For detailed information useful during development or troubleshooting.
  ```go
  logger.Debug().
      Int("item_count", len(items)).
      Float64("total_price", totalPrice).
      Msg("Calculated shopping cart totals")
  ```
- **Warn Level:** For non-critical issues that might indicate a problem.
  ```go
  logger.Warn().
      Str("feature", "legacy_api").
      Msg("Deprecated API endpoint accessed")
  ```
- **Error Level:** For errors that prevent a request from completing successfully. Always include the error object using **`.Err(err)`**.
  ```go
  if err := someFailingOperation(); err != nil {
      logger.Error().Err(err).Msg("Failed to perform critical operation")
  }
  ```
- **Fatal/Panic Level:** Use with caution. These will exit the application after logging. Often used for unrecoverable errors during startup.
  ```go
  // logger.Fatal().Err(err).Msg("Database connection failed, exiting")
  ```
- **Adding Custom Fields:** Attach any relevant data to your log messages using methods like `.Str()`, `.Int()`, `.Bool()`, `.Float64()`, `.Time()`, `.Dur()`, `.Any()`.
  ```go
  logger.Info().
      Str("user_agent", c.Request.UserAgent()).
      Str("endpoint", c.FullPath()).
      Int("status_code", http.StatusOK).
      Dur("latency", time.Since(start)). // For custom timing
      Msg("Request completed")
  ```

#### 3. Propagating the Request ID to Downstream Services

When your service makes an outgoing HTTP call to another internal or external service, ensure you propagate the `request_id` by setting the **`X-Request-ID`** header:

```go
import (
	"context"
	"net/http"
	// ...
)

func callAnotherService(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return nil, err
    }

    // Retrieve request ID from context and set it in the outgoing request header
    if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
        req.Header.Set("X-Request-ID", requestID)
        // You can also log here that the ID is being propagated:
        if logger, ok := ctx.Value(LoggerKey).(zerolog.Logger); ok {
            logger.Debug().Str("X-Request-ID", requestID).Msg("Propagating request ID to downstream service")
        }
    }

    return client.Do(req)
}
```

**Always pass `context.Context` down to your helper functions and service layers!** This is how the request-scoped logger and `request_id` are propagated.

---

### Example Logging in Console

When running locally with `zerolog.ConsoleWriter`, your logs will look something like this:

```
8:45AM INF Ping endpoint hit request_id=e7b3a4c5-d6e7-4f01-8b2c-9a0d1e2f3g4h
8:45AM INF Fetching data from an external service... request_id=e7b3a4c5-d6e7-4f01-8b2c-9a0d1e2f3g4h
8:45AM DBG Propagating request ID to downstream service X-Request-ID=e7b3a4c5-d6e7-4f01-8b2c-9a0d1e2f3g4h request_id=e7b3a4c5-d6e7-4f01-8b2c-9a0d1e2f3g4h
8:45AM INF External service responded status_code=200 request_id=e7b3a4c5-d6e7-4f01-8b2c-9a0d1e2f3g4h
```

Notice how **`request_id` is consistently present in all log lines** for a given request.

---

### In Production

- `zerolog` will automatically output **JSON formatted logs**, which are ideal for consumption by log aggregation systems (e.g., ELK Stack, Grafana Loki, Splunk).
- Ensure your log aggregators are configured to parse the `request_id` field (and other structured fields) for efficient searching and filtering.
