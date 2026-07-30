// Package observability wires distributed tracing for the backend
// (roadmap 020, Tier 2).
//
// WHAT THIS BUYS OVER TIER 0/1. nginx says WHICH URL is slow. The RED metrics
// in core-be-lib say WHICH ROUTE TEMPLATE and whether we were saturated. Both
// are aggregates: they have already thrown the individual request away. A trace
// keeps one request whole across every process it touched, which is the only
// way an N+1 query is visible at all — in metrics such an endpoint is merely
// "slow", in a waterfall it is unmistakable.
//
// WHY SENTRY AND NOT RAW OTEL. Sentry is already instrumented in the frontend,
// so the browser transaction and the backend transaction join into ONE trace:
// a slow page load shows the SQL that caused it. Raw OTel would mean operating
// a Tempo/Jaeger for the same outcome. (The go.opentelemetry.io entries in
// go.mod are indirect, pulled in by the GCP libraries — there is no existing
// OTel investment to preserve.)
//
// EVERYTHING HERE IS ENV-GATED AND DEGRADES TO A NO-OP. With SENTRY_DSN unset —
// which is the case in local development and in any environment that has not
// been given the variable — Init returns false, no middleware is installed, and
// the pgx tracer does nothing. There is no code path that requires Sentry to be
// configured.
package observability

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

var enabled bool

// Enabled reports whether tracing was successfully initialised.
func Enabled() bool { return enabled }

// Init sets up the Sentry SDK from the environment.
//
// Returns false (and logs at info, not error) when SENTRY_DSN is absent: an
// unconfigured environment is the normal case, not a fault.
//
// Env:
//
//	SENTRY_DSN                 — enables everything. Unset ⇒ no-op.
//	SENTRY_ENVIRONMENT         — defaults to "production".
//	SENTRY_RELEASE             — defaults to unset (Sentry infers nothing).
//	SENTRY_TRACES_SAMPLE_RATE  — 0..1, defaults to 0.2.
func Init() bool {
	dsn := strings.TrimSpace(os.Getenv("SENTRY_DSN"))
	if dsn == "" {
		log.Info().Msg("tracing: SENTRY_DSN unset — distributed tracing disabled")
		return false
	}

	rate := 0.2
	if raw := os.Getenv("SENTRY_TRACES_SAMPLE_RATE"); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && parsed >= 0 && parsed <= 1 {
			rate = parsed
		} else {
			log.Warn().Str("value", raw).Msg("tracing: invalid SENTRY_TRACES_SAMPLE_RATE, using default")
		}
	}

	env := os.Getenv("SENTRY_ENVIRONMENT")
	if env == "" {
		env = "production"
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      env,
		Release:          os.Getenv("SENTRY_RELEASE"),
		EnableTracing:    true,
		TracesSampleRate: rate,
		// The whole point of this tier is latency attribution, not PII. The
		// frontend runs with sendDefaultPii false for the same reason; keeping
		// the backend consistent means a trace never becomes a data-protection
		// question.
		SendDefaultPII: false,
	})
	if err != nil {
		// A bad DSN must not stop the server from booting. Observability is
		// there to explain outages, not to cause them.
		log.Error().Err(err).Msg("tracing: sentry init failed — continuing without tracing")
		return false
	}

	enabled = true
	log.Info().Str("environment", env).Float64("traces_sample_rate", rate).Msg("tracing: enabled")
	return true
}

// Flush drains buffered events. Call it on shutdown, before the process exits,
// or the last few traces before a crash — the interesting ones — are lost.
func Flush(timeout time.Duration) {
	if !enabled {
		return
	}
	sentry.Flush(timeout)
}

// GinMiddleware returns the handlers to install at the OUTERMOST position of
// the API chain, or nil when tracing is disabled.
//
// Two handlers, in order:
//  1. sentrygin — opens the transaction, continuing the browser's trace when
//     the request carries sentry-trace/baggage headers.
//  2. nameTransaction — renames it to the gin route TEMPLATE.
//
// The second is not cosmetic. sentrygin names a transaction after the raw URL
// path, so /api/v1/lms/courses/<uuid> becomes its own transaction — the exact
// cardinality mistake that fragmented one endpoint into ~20 rows in the audit
// that produced this roadmap, making every p95 a fifth as reliable as it looked.
func GinMiddleware() []gin.HandlerFunc {
	if !enabled {
		return nil
	}
	return []gin.HandlerFunc{
		sentrygin.New(sentrygin.Options{
			// Let gin.Recovery() own panic handling; re-panicking here would
			// mean the request never reaches it and the client gets a dead
			// connection instead of a 500.
			Repanic: true,
		}),
		nameTransaction(),
	}
}

func nameTransaction() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// FullPath() is only resolved once the route has been matched, hence
		// after c.Next(). Unmatched requests (404s, scanner probes) collapse to
		// one name rather than minting a transaction per random path.
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		if tx := sentry.TransactionFromContext(c.Request.Context()); tx != nil {
			tx.Name = c.Request.Method + " " + route
			tx.SetTag("route", route)
		}
	}
}

// ── pgx query tracing ────────────────────────────────────────────────────────

// QueryTracer emits one span per SQL statement, so a slow endpoint's trace
// shows exactly which query — and how many times — it ran.
//
// This is where the answers live. Roadmap 018 documented ~6 queries per lesson
// request, and the admin course list's p95 of 4.5 s against a 604 ms average is
// the signature of a fan-out that grows with the data. A waterfall settles that
// in one screenshot; no aggregate metric ever will.
//
// Implements pgx.QueryTracer. Attach via pgxpool.Config.ConnConfig.Tracer.
type QueryTracer struct{}

// TraceQueryStart opens the span.
func (QueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if !enabled {
		return ctx
	}
	span := sentry.StartSpan(ctx, "db.query")
	if span == nil {
		return ctx
	}
	// The SQL text is the span description. It is the query TEMPLATE — pgx
	// passes parameters separately and they are deliberately not attached, so
	// no user data reaches Sentry and every execution of a statement groups
	// under one description instead of fragmenting per parameter value.
	span.Description = summariseSQL(data.SQL)
	return span.Context()
}

// TraceQueryEnd closes it.
func (QueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if !enabled {
		return
	}
	span := sentry.SpanFromContext(ctx)
	if span == nil {
		return
	}
	if data.Err != nil {
		span.Status = sentry.SpanStatusInternalError
	} else {
		span.Status = sentry.SpanStatusOK
		span.SetData("rows_affected", data.CommandTag.RowsAffected())
	}
	span.Finish()
}

const maxSQLLength = 400

// summariseSQL collapses whitespace and truncates.
//
// Sentry truncates long descriptions anyway, and an unbounded multi-line query
// makes the waterfall unreadable. Whitespace collapsing also means the same
// statement formatted differently in two places still groups as one.
func summariseSQL(sql string) string {
	compact := strings.Join(strings.Fields(sql), " ")
	if len(compact) > maxSQLLength {
		return compact[:maxSQLLength] + "…"
	}
	return compact
}

var _ pgx.QueryTracer = QueryTracer{}

// ── outbound calls ───────────────────────────────────────────────────────────

// TracedTransport instruments outbound HTTP so third-party time is attributed
// to the third party.
//
// An authoring request that "takes 20 seconds" is usually waiting on a model
// provider, the TTS vendor, Tika or the video-render sidecar. Without a span
// per outbound call that shows up as 20 seconds inside our handler, and someone
// spends a week optimising code that was idle the whole time.
type TracedTransport struct {
	// Base is the underlying RoundTripper. nil means http.DefaultTransport.
	Base http.RoundTripper
	// Service names the callee in the span ("mistral", "tika", "video-render").
	// It is a fixed string per client, never derived from the URL — a URL would
	// put ids and query parameters into span descriptions.
	Service string
}

func (t TracedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if !enabled {
		return base.RoundTrip(req)
	}

	span := sentry.StartSpan(req.Context(), "http.client")
	if span == nil {
		return base.RoundTrip(req)
	}
	// Method + host + path, deliberately WITHOUT the query string: query
	// parameters carry ids and secrets and would fragment the grouping.
	span.Description = req.Method + " " + t.Service + " " + req.URL.Path
	span.SetTag("service", t.Service)
	defer span.Finish()

	resp, err := base.RoundTrip(req.WithContext(span.Context()))
	if err != nil {
		span.Status = sentry.SpanStatusInternalError
		return resp, err
	}
	span.SetData("http.status_code", resp.StatusCode)
	span.Status = sentry.HTTPtoSpanStatus(resp.StatusCode)
	return resp, nil
}

// TracedHTTPClient returns an http.Client whose outbound calls appear as spans.
// Safe to use when tracing is disabled — the transport falls straight through.
func TracedHTTPClient(service string, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: TracedTransport{Service: service},
	}
}
