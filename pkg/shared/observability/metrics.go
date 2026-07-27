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
	"net/http"
	"strings"
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
	// The "route" label was added after auth_duration proved UNDIAGNOSABLE without
	// it. Production showed auth averaging 89ms inside a request averaging 67ms —
	// impossible, and impossible to attribute, because the only label was
	// "outcome". There was no way to ask WHICH endpoints were slow in auth, so the
	// investigation could only guess. A metric you cannot slice is a metric you
	// cannot act on.
	//
	// Cardinality is bounded and strictly SMALLER than requestDuration's, which
	// already carries route alongside method and status_class. Same template-only
	// rule applies — see routeLabel below.
	// Long-lived connections, counted rather than timed. Their duration is a
	// session length, not a latency, and belongs in no latency histogram.
	streamConnections = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "stream_connections_total",
			Help:      "Long-lived connections (WebSocket/SSE), excluded from the request-duration histogram.",
		},
		[]string{"route"},
	)

	authDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "auth_duration_seconds",
			Help:      "Time spent inside the auth middleware, including the Kratos round-trip.",
			Buckets: []float64{
				0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5,
			},
		},
		[]string{"outcome", "route"},
	)
)

// routeLabel returns the gin route TEMPLATE, never the raw URL.
//
// c.FullPath() is "" for an unmatched request; bucketing those under a literal
// "unmatched" is what keeps a scanner probing /wp-admin/<random> from minting
// unbounded series. Same rule as requestDuration — see the cardinality note at
// the top of this file.
func routeLabel(c *gin.Context) string {
	if r := c.FullPath(); r != "" {
		return r
	}
	return "unmatched"
}

func init() {
	prometheus.MustRegister(requestDuration, requestsInFlight, authDuration, streamConnections)
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
// ── IT MUST BE REGISTERED WITH router.Use(), NOT VIA APIOptions.Middlewares ──
//
// This is not a style preference; putting it in the oapi-codegen middleware list
// silently reduces it to measuring nothing, and that shipped to production.
//
// oapi-codegen's gin generator does NOT run APIOptions.Middlewares through gin's
// handler chain. Each generated wrapper calls them as plain functions:
//
//	for _, middleware := range siw.HandlerMiddlewares {
//	    middleware(c)                 // plain call
//	    if c.IsAborted() { return }
//	}
//	siw.Handler.ListRunsByBatch(c, batchId)   // handler runs AFTER the loop
//
// A middleware that measures by calling c.Next() therefore measures the wrong
// thing: c.Next() advances GIN's chain, which at that point holds only the
// wrapper already executing, so it returns immediately — before the handler has
// run. The observation came back as ~76µs on every generated route while nginx
// measured tens of milliseconds for the same requests.
//
// The failure is invisible in the worst way: the metric exists, has plausible
// labels, and every panel renders. The only route reading correctly was
// /api/v1/assets/:id/file, which asset-lib registers straight onto gin — so the
// "costliest routes" panel showed it as 99.98% of all server time, which looked
// like a finding about assets and was actually a finding about instrumentation.
//
// Registered with router.Use() the middleware sits in gin's real chain, wraps
// the generated wrapper AND its handler, and covers hand-registered routes too.
func HTTPMetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestsInFlight.Inc()

		// Deferred so a panicking handler is still counted. Without this the
		// observation is skipped while the panic unwinds to gin.Recovery(), and
		// 500s from panics — the ones most worth seeing — go unrecorded.
		defer func() {
			requestsInFlight.Dec()

			route := c.FullPath()
			if route == "" {
				route = "unmatched"
			}
			// Prometheus scrapes its own endpoint every interval. Counting them
			// adds a steady stream of fast requests that flatters every
			// aggregate and measures nothing about the product.
			if route == metricsPath {
				return
			}
			// Streams are counted, never timed — see isStream. Counting them
			// keeps them visible: silently dropping the observation would make
			// a flood of WebSocket churn look like no traffic at all.
			if isStream(c) {
				streamConnections.WithLabelValues(route).Inc()
				return
			}
			requestDuration.WithLabelValues(
				c.Request.Method,
				route,
				statusClass(c.Writer.Status()),
			).Observe(time.Since(start).Seconds())
		}()

		c.Next()

	}
}

// ── Auth timing: a PAIR of middlewares, not a wrapper ───────────────────────
//
// WHY A PAIR. The obvious implementation is wrong, and was shipped before being
// caught in production:
//
//	func TimeAuthMiddleware(next gin.HandlerFunc) gin.HandlerFunc {
//	    return func(c *gin.Context) { t := time.Now(); next(c); observe(t) }  // WRONG
//	}
//
// A gin middleware continues the chain by calling c.Next() from INSIDE itself.
// The coreapp auth middleware does exactly that (three call sites). So `next(c)`
// above does not return when auth finishes — it returns when the entire
// remaining chain, handler and database work included, has finished. The metric
// therefore measured ~the whole request and the derived "Kratos share of request
// time" read 16% one hour and 41,331% the next. Both were noise.
//
// The fix is to observe the moment auth HANDS OFF, which is the moment the next
// middleware in the chain starts running. AuthTimerStart stamps the clock;
// AuthTimerEnd, placed immediately after auth, reads it before any downstream
// work has happened.
//
//	middlewares = [..., AuthTimerStart(), authSlot.handle, AuthTimerEnd(), ...]
//
// Both must be adjacent to auth. Anything placed between them is counted as
// auth, which is the one way to get this wrong again.
// metricsPath is the scrape endpoint, excluded from its own histogram.
const metricsPath = "/metrics"

// isStream reports whether this request is a long-lived connection rather than
// a request/response exchange.
//
// THIS EXCLUSION IS NOT OPTIONAL. For a stream, "duration" is how long the
// client stayed connected — this vhost allows proxy_read_timeout 86400 — so a
// single WebSocket open for five minutes contributes 300 SECONDS to a latency
// histogram. Observed immediately after HTTPMetricsMiddleware moved to
// router.Use(): 5 requests summing to 299.3s, while the app was in fact
// answering in microseconds.
//
// The same exclusion already existed one layer down, in the nginx pipeline's
// `stream` label (infra/config.alloy). Moving this middleware outermost put the
// app on the same footing as nginx — and therefore exposed it to the same
// artefact. Two independent signals, because they catch different things:
//
//	Upgrade / 101  — WebSocket, which announces itself in the REQUEST.
//	text/event-stream — SSE, an ordinary GET that only the RESPONSE identifies.
//
// Keying on the request alone misses SSE completely; there are 11 SSE handlers.
func isStream(c *gin.Context) bool {
	if c.Request.Header.Get("Upgrade") != "" || c.Writer.Status() == http.StatusSwitchingProtocols {
		return true
	}
	return strings.Contains(c.Writer.Header().Get("Content-Type"), "text/event-stream")
}

const (
	authStartKey    = "coreapp_auth_timer_start"
	authRecordedKey = "coreapp_auth_timer_recorded"
)

// AuthTimerStart stamps the clock. Place IMMEDIATELY BEFORE the auth middleware.
//
// It also covers the abort path: when auth rejects a request it never calls
// c.Next(), so AuthTimerEnd never runs. In that case nothing downstream ran
// either, so measuring here after c.Next() returns is still auth-only time.
func AuthTimerStart() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(authStartKey, time.Now())
		c.Next()

		if _, recorded := c.Get(authRecordedKey); recorded {
			return // AuthTimerEnd already observed it
		}

		// c.IsAborted() IS REQUIRED, not a belt-and-braces extra.
		//
		// "AuthTimerEnd has not recorded yet" does NOT imply "auth rejected the
		// request". Under oapi-codegen's sequential middleware loop this
		// function's c.Next() returns immediately — before auth has even run —
		// so authRecordedKey is legitimately unset at this point on EVERY
		// request. Without this guard the fallback fired every time and
		// outcome="aborted" came to mean "a request happened".
		//
		// Production made that unmistakable: aborted and passed counts were
		// exactly equal on both instances (59/59 and 47/47).
		//
		// KNOWN LIMITATION: on an oapi-codegen route a genuine auth rejection is
		// not observed at all — the generated wrapper breaks its loop on
		// c.IsAborted() and AuthTimerEnd never runs. Under-counting rejections is
		// far better than labelling every request as one, and outcome="passed"
		// (the series the SLO actually uses) is unaffected either way.
		if !c.IsAborted() {
			return
		}
		if v, ok := c.Get(authStartKey); ok {
			if start, ok := v.(time.Time); ok {
				authDuration.WithLabelValues("aborted", routeLabel(c)).Observe(time.Since(start).Seconds())
			}
		}
	}
}

// AuthTimerEnd observes auth's own duration. Place IMMEDIATELY AFTER the auth
// middleware.
//
// It runs at the instant auth calls c.Next() — before the handler, before any
// query — so time.Since(start) is auth's work alone.
func AuthTimerEnd() gin.HandlerFunc {
	return func(c *gin.Context) {
		if v, ok := c.Get(authStartKey); ok {
			if start, ok := v.(time.Time); ok {
				authDuration.WithLabelValues("passed", routeLabel(c)).Observe(time.Since(start).Seconds())
				c.Set(authRecordedKey, true)
			}
		}
		c.Next()
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
	r.GET(metricsPath, gin.WrapH(promhttp.Handler()))
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
