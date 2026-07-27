// Package observability holds the application's own view of its request
// latency: RED metrics (Rate, Errors, Duration) plus the saturation signals
// that tell a "we got slower" incident apart from a "we ran out of capacity"
// one.
//
// WHY THIS EXISTS (roadmap 020, Tier 1). The stack had exactly one source of
// response-time data — the browser. nginx measured every request and logged
// none of the numbers; gin's default logger computed a latency per request and
// wrote it to stdout, where nothing scraped it. So when a trace said an
// endpoint took 600 ms, nothing could say whether that was network, auth,
// handler or database.
//
// nginx (Tier 0) answers "which URL is slow" from outside, at 100% sampling.
// This package answers "which ROUTE TEMPLATE, at which status, and were we
// saturated" — with the router's own view, which nginx cannot have.
//
// # Cardinality
//
// Every label here is bounded by construction and must stay that way. `route`
// is the gin route TEMPLATE from c.FullPath() (e.g. /api/v1/lms/courses/:id),
// never c.Request.URL.Path — a raw path mints one time series per entity id,
// which is the single most common way teams take down a Prometheus. `status` is
// deliberately a CLASS (2xx/4xx/5xx) rather than the exact code: the exact code
// belongs in logs, where it is free.
package observability

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "http_server"

var (
	// requestDuration is the RED "Duration" and, with _count, "Rate" and
	// "Errors" too — one histogram answers all three.
	//
	// Buckets are hand-chosen rather than prometheus.DefBuckets, for two reasons:
	// DefBuckets has no boundary between 0.25 and 0.5, so a p95 near the 300ms
	// read SLO would be interpolated across a bucket four times too wide to
	// trust; and its lowest boundary is far above where this app actually lives.
	// See the bucket list below for what production measurement changed.
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "request_duration_seconds",
			Help:      "Latency of HTTP requests handled by this instance, by route template.",
			// Buckets start at 1ms, NOT 10ms. Measured in production after the
			// first rollout: every ordinary read completed inside the old lowest
			// bucket (le=0.01), so histogram_quantile had nothing to interpolate
			// between and reported the bucket's linear fractions as if they were
			// data — p50 5.0ms, p95 9.5ms, p99 9.9ms, identical for every route.
			// Those were artefacts of the bucket edge, not latencies.
			//
			// 0.3 / 0.8 / 30 are retained as exact SLO boundaries (Tier 4 reads
			// ratios straight off these `le` values, so removing one silently
			// empties an alert). The long tail serves the AI/authoring routes.
			Buckets: []float64{
				0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.3, 0.5, 0.8,
				1, 2, 5, 10, 30, 60,
			},
		},
		[]string{"method", "route", "status_class"},
	)

	// requestsInFlight is the saturation signal. A p95 spike with FLAT in-flight
	// means the work itself got slower; a p95 spike that tracks in-flight means
	// requests are queueing and the fix is capacity, not optimisation. The two
	// are indistinguishable on a latency graph alone, which is why this gauge is
	// not optional.
	requestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "requests_in_flight",
			Help:      "Number of HTTP requests currently being served by this instance.",
		},
	)

	// authDuration isolates the cost of the auth middleware — which performs a
	// NETWORK round-trip to Kratos on every authenticated request.
	//
	// This metric exists to settle a specific open question. Roadmap 018 dropped
	// the backend Kratos session cache (T5) with an explicit re-entry condition:
	// "the trigger is a measurement — Kratos's share of p50 on ordinary
	// endpoints." Divide this histogram by request_duration_seconds and that
	// share is on a dashboard instead of in an argument.
	authDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "auth_duration_seconds",
			Help:      "Time spent inside the auth middleware, including the Kratos round-trip.",
			Buckets: []float64{
				0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5,
			},
		},
		[]string{"outcome"},
	)
)

func init() {
	prometheus.MustRegister(requestDuration, requestsInFlight, authDuration)
}

// statusClass collapses an HTTP status into a bounded label.
//
// The exact code is intentionally NOT a label. It adds ~40 possible values to
// every series for information that is already in the access log, and the thing
// dashboards and burn-rate alerts actually need is the class.
func statusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	case code >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}

// HTTPMetricsMiddleware records RED metrics for every request it wraps.
//
// Install it FIRST in the middleware chain so the histogram covers the entire
// server-side cost — auth, tenant resolution, handler and all. Anything placed
// before it is time this metric cannot see, and time nobody can then explain.
func HTTPMetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestsInFlight.Inc()
		defer requestsInFlight.Dec()

		c.Next()

		// c.FullPath() must be read AFTER c.Next(): gin only resolves the route
		// once the request has been matched against the tree.
		//
		// It is "" for an unmatched request (404) — bucketing those under a
		// literal "unmatched" is what stops a 404 scan (or a scanner probing
		// /wp-admin/<random>) from minting an unbounded number of series. That
		// is not a hypothetical: it is the normal state of any public endpoint.
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}

		requestDuration.WithLabelValues(
			c.Request.Method,
			route,
			statusClass(c.Writer.Status()),
		).Observe(time.Since(start).Seconds())
	}
}

// TimeAuthMiddleware wraps an auth middleware so its own cost is measured
// separately from the handler's.
//
// "outcome" is aborted/passed rather than an error string: an auth failure and
// an auth success have very different latency profiles (a rejected request may
// short-circuit before the Kratos call), and averaging them together hides
// both.
func TimeAuthMiddleware(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		next(c)
		outcome := "passed"
		if c.IsAborted() {
			outcome = "aborted"
		}
		authDuration.WithLabelValues(outcome).Observe(time.Since(start).Seconds())
	}
}

// MountMetricsEndpoint exposes the Prometheus scrape endpoint.
//
// It is registered on the app's own listener, which is published to 127.0.0.1
// only and is not proxied by nginx, so /metrics is unreachable from the
// internet. Prometheus reaches it over the compose network. Deliberately NOT
// behind the auth middleware: a scraper has no session, and gating it would
// mean either an exception in the auth chain or a second listener.
func MountMetricsEndpoint(r gin.IRoutes) {
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
}

// RegisterPgxPoolCollector exports connection-pool statistics.
//
// EmptyAcquireCount is the one to watch and the reason this exists. When it
// rises, queries are fast but requests are waiting for a CONNECTION — the
// classic invisible bottleneck, because every per-query timing looks healthy
// and nothing else in the stack surfaces the wait. AcquireDuration is its
// magnitude.
//
// Safe to call once per process; a second call with the same pool would panic
// on duplicate registration, so callers pass the single shared pool.
func RegisterPgxPoolCollector(pool *pgxpool.Pool) {
	if pool == nil {
		return
	}

	gauge := func(name, help string, fn func(*pgxpool.Stat) float64) prometheus.Collector {
		return prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: "pgxpool",
				Name:      name,
				Help:      help,
			},
			func() float64 { return fn(pool.Stat()) },
		)
	}

	prometheus.MustRegister(
		gauge("total_conns", "Total connections currently in the pool.",
			func(s *pgxpool.Stat) float64 { return float64(s.TotalConns()) }),
		gauge("idle_conns", "Connections currently idle in the pool.",
			func(s *pgxpool.Stat) float64 { return float64(s.IdleConns()) }),
		gauge("acquired_conns", "Connections currently checked out.",
			func(s *pgxpool.Stat) float64 { return float64(s.AcquiredConns()) }),
		gauge("max_conns", "Configured maximum pool size.",
			func(s *pgxpool.Stat) float64 { return float64(s.MaxConns()) }),
		// Counters, exported as gauges via GaugeFunc because pgx owns the
		// monotonic value and we only sample it. rate() over them behaves
		// correctly regardless of the declared type.
		gauge("acquire_count_total", "Cumulative successful connection acquisitions.",
			func(s *pgxpool.Stat) float64 { return float64(s.AcquireCount()) }),
		gauge("empty_acquire_count_total", "Cumulative acquisitions that had to WAIT for a free connection — the saturation signal.",
			func(s *pgxpool.Stat) float64 { return float64(s.EmptyAcquireCount()) }),
		gauge("canceled_acquire_count_total", "Cumulative acquisitions cancelled before a connection was free.",
			func(s *pgxpool.Stat) float64 { return float64(s.CanceledAcquireCount()) }),
		gauge("acquire_duration_seconds_total", "Cumulative time spent waiting for a connection.",
			func(s *pgxpool.Stat) float64 { return s.AcquireDuration().Seconds() }),
	)
}

// RouteLabel returns the bounded route label for a request, for callers that
// want the same value in a log line as the metric carries.
func RouteLabel(c *gin.Context) string {
	if route := c.FullPath(); route != "" {
		return route
	}
	return "unmatched"
}

// StatusLabel exposes the status class for the same reason.
func StatusLabel(code int) string { return statusClass(code) }
