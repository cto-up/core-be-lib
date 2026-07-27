package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// observations returns how many samples a histogram child has recorded.
//
// testutil.ToFloat64 cannot be used here: HistogramVec.WithLabelValues returns
// a prometheus.Observer, which is not a Collector. The child IS a
// prometheus.Metric though, so its dto form carries the sample count.
func observations(t *testing.T, vec *prometheus.HistogramVec, labels ...string) uint64 {
	t.Helper()
	obs, err := vec.GetMetricWithLabelValues(labels...)
	require.NoError(t, err)
	m, ok := obs.(prometheus.Metric)
	require.True(t, ok, "histogram child should implement prometheus.Metric")
	var out dto.Metric
	require.NoError(t, m.Write(&out))
	return out.GetHistogram().GetSampleCount()
}

func TestStatusClass(t *testing.T) {
	cases := map[int]string{
		100: "1xx", 200: "2xx", 204: "2xx", 301: "3xx", 304: "3xx",
		400: "4xx", 401: "4xx", 404: "4xx", 500: "5xx", 503: "5xx",
	}
	for code, want := range cases {
		assert.Equal(t, want, statusClass(code), "status %d", code)
	}
}

// newTestRouter builds an engine with the metrics middleware and a couple of
// routes, and returns a private registry holding a fresh copy of the histogram
// so assertions do not depend on what other tests observed.
func newTestRouter(t *testing.T) (*gin.Engine, *prometheus.Registry, *prometheus.HistogramVec) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	reg := prometheus.NewRegistry()
	hist := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Name: "test_request_duration_seconds", Buckets: []float64{1}},
		[]string{"method", "route", "status_class"},
	)
	require.NoError(t, reg.Register(hist))

	// Mirror of HTTPMetricsMiddleware against the private histogram. Kept in
	// lockstep with the real one deliberately: the behaviour under test is the
	// LABEL choice (route template, status class, unmatched fallback), which is
	// where the cardinality bugs live.
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		hist.WithLabelValues(c.Request.Method, route, statusClass(c.Writer.Status())).Observe(0)
	})
	r.GET("/api/v1/lms/courses/:courseId", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/api/v1/boom", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	return r, reg, hist
}

func do(r *gin.Engine, method, path string) {
	req := httptest.NewRequest(method, path, nil)
	r.ServeHTTP(httptest.NewRecorder(), req)
}

// The whole point of the route label: many entity ids must collapse to ONE
// series. A raw URL path here would mint one series per course and is the
// single most common way a Prometheus gets taken down.
func TestRouteLabelUsesTemplateNotPath(t *testing.T) {
	r, _, hist := newTestRouter(t)

	for _, id := range []string{
		"9c8c315c-66ba-4b57-b005-72f43a8db9a5",
		"ca69a396-253d-4a12-9352-b2c35df01d20",
		"f8f5a9a5-b4e5-42ef-8c01-1cc2e0e04a89",
	} {
		do(r, http.MethodGet, "/api/v1/lms/courses/"+id)
	}

	assert.Equal(t, 1, testutil.CollectAndCount(hist),
		"three distinct ids must produce exactly one time series")
	assert.Equal(t, uint64(3),
		observations(t, hist, "GET", "/api/v1/lms/courses/:courseId", "2xx"),
		"all three requests must land on the templated series")
}

// A 404 has no matched route, so FullPath() is "". Without the fallback, every
// scanner probing /wp-admin/<random> would create a new series — which is the
// normal state of any public endpoint, not a hypothetical.
func TestUnmatchedRoutesCollapseToOneSeries(t *testing.T) {
	r, _, hist := newTestRouter(t)

	for _, p := range []string{"/wp-admin/x1", "/wp-admin/x2", "/.env", "/nope/nope"} {
		do(r, http.MethodGet, p)
	}

	assert.Equal(t, 1, testutil.CollectAndCount(hist),
		"unmatched paths must all collapse into a single 'unmatched' series")
	assert.Equal(t, uint64(4), observations(t, hist, "GET", "unmatched", "4xx"))
}

// Splitting by outcome is what stops a burst of fast 500s from flattering p50.
func TestSuccessAndErrorAreSeparateSeries(t *testing.T) {
	r, _, hist := newTestRouter(t)

	do(r, http.MethodGet, "/api/v1/lms/courses/abc")
	do(r, http.MethodGet, "/api/v1/boom")

	assert.Equal(t, 2, testutil.CollectAndCount(hist))
	assert.Equal(t, uint64(1), observations(t, hist, "GET", "/api/v1/boom", "5xx"))
}

// The real middleware, end to end, against the package's own registered
// collectors — proves the exported wiring works, not just the mirror above.
func TestHTTPMetricsMiddlewareRecordsAgainstRealCollectors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(HTTPMetricsMiddleware())
	r.GET("/probe/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	before := observations(t, requestDuration, "GET", "/probe/:id", "2xx")
	do(r, http.MethodGet, "/probe/one")
	do(r, http.MethodGet, "/probe/two")
	after := observations(t, requestDuration, "GET", "/probe/:id", "2xx")

	assert.Equal(t, uint64(2), after-before)
}

// In-flight must return to its starting value even when a handler panics —
// otherwise the saturation gauge ratchets upward forever after the first panic
// and the "are we queueing?" signal becomes permanently useless.
func TestInFlightIsReleasedOnPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(HTTPMetricsMiddleware())
	r.GET("/panic", func(c *gin.Context) { panic("boom") })

	before := testutil.ToFloat64(requestsInFlight)
	do(r, http.MethodGet, "/panic")
	assert.Equal(t, before, testutil.ToFloat64(requestsInFlight),
		"in-flight gauge leaked after a handler panic")
}

// THE REGRESSION THIS PAIR EXISTS FOR.
//
// A gin middleware continues the chain from inside itself via c.Next(). A single
// wrapping middleware therefore measures auth PLUS everything downstream. The
// pair must measure auth alone — proven here with a deliberately slow handler:
// if the handler's time leaks in, the assertion fails.
func TestAuthTimerMeasuresAuthOnlyNotDownstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	before := observations(t, authDuration, "passed", "/x")

	r := gin.New()
	r.Use(AuthTimerStart())
	r.Use(func(c *gin.Context) { // stands in for the auth middleware
		time.Sleep(20 * time.Millisecond)
		c.Next()
	})
	r.Use(AuthTimerEnd())
	r.GET("/x", func(c *gin.Context) {
		time.Sleep(200 * time.Millisecond) // handler + DB work
		c.Status(http.StatusOK)
	})
	do(r, http.MethodGet, "/x")

	require.Equal(t, uint64(1), observations(t, authDuration, "passed", "/x")-before)

	var m dto.Metric
	obs, err := authDuration.GetMetricWithLabelValues("passed", "/x")
	require.NoError(t, err)
	require.NoError(t, obs.(prometheus.Metric).Write(&m))
	// Cumulative sum across the suite, so assert the increment is auth-shaped:
	// ~20ms, and nowhere near the 220ms a wrapper would have recorded.
	assert.Less(t, m.GetHistogram().GetSampleSum(), 0.15,
		"auth timing absorbed downstream handler time — the c.Next() bug is back")
}

func TestAuthTimerRecordsAnAbort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	before := observations(t, authDuration, "aborted", "/x")

	r := gin.New()
	r.Use(AuthTimerStart())
	r.Use(func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
	r.Use(AuthTimerEnd())
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	// This package's do() returns nothing; assert the status directly.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, uint64(1), observations(t, authDuration, "aborted", "/x")-before,
		"an aborted auth never reaches AuthTimerEnd; AuthTimerStart must record it")
}

func TestMetricsEndpointServesPrometheusText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	MountMetricsEndpoint(r)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "http_server_request_duration_seconds")
	assert.Contains(t, w.Body.String(), "http_server_requests_in_flight")
	assert.Contains(t, w.Body.String(), "http_server_auth_duration_seconds")
}

func TestRegisterPgxPoolCollectorTolerantOfNilPool(t *testing.T) {
	// A consumer that builds a ServerConfig without a pool must not panic.
	assert.NotPanics(t, func() { RegisterPgxPoolCollector(nil) })
}

func TestRouteAndStatusLabelHelpers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	var got string
	r.GET("/x/:id", func(c *gin.Context) { got = RouteLabel(c) })
	do(r, http.MethodGet, "/x/7")
	assert.Equal(t, "/x/:id", got)

	assert.Equal(t, "5xx", StatusLabel(502))
}

// The route label must be the gin TEMPLATE, never the concrete URL. Getting this
// wrong turns one series into one-per-id and takes Prometheus down — the single
// hard rule in infra/OBSERVABILITY.md § 7.
func TestAuthTimerRouteLabelIsTemplateNotRawPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	before := observations(t, authDuration, "passed", "/u/:id")

	r := gin.New()
	r.Use(AuthTimerStart())
	r.Use(func(c *gin.Context) { c.Next() })
	r.Use(AuthTimerEnd())
	r.GET("/u/:id", func(c *gin.Context) { c.Status(http.StatusOK) })

	// Three DIFFERENT ids must collapse into the SAME series.
	for _, id := range []string{"a1b2", "c3d4", "e5f6"} {
		do(r, http.MethodGet, "/u/"+id)
	}

	assert.Equal(t, uint64(3), observations(t, authDuration, "passed", "/u/:id")-before,
		"all ids must share one series; a raw-path label would split them")
}

// An unmatched request (404) has no route template. It must collapse to a single
// literal bucket, or a scanner probing random URLs mints unbounded series.
func TestAuthTimerUnmatchedRouteCollapses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	before := observations(t, authDuration, "aborted", "unmatched")

	r := gin.New()
	r.Use(AuthTimerStart())
	r.Use(func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
	r.Use(AuthTimerEnd())
	r.GET("/known", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, path := range []string{"/wp-admin/x", "/.env", "/phpmyadmin"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	}

	assert.Equal(t, uint64(3), observations(t, authDuration, "aborted", "unmatched")-before,
		"probes of random paths must share one series")
}

// THE REGRESSION THIS EXISTS FOR — the bug that made every generated route
// report ~76µs while nginx measured tens of milliseconds for the same requests.
//
// oapi-codegen does NOT run APIOptions.Middlewares through gin's chain. Each
// generated wrapper calls them as plain functions and then calls the handler:
//
//	for _, mw := range siw.HandlerMiddlewares { mw(c); if c.IsAborted() { return } }
//	siw.Handler.Op(c, args...)
//
// oapiWrapper reproduces that convention exactly.
func oapiWrapper(mws []gin.HandlerFunc, handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, mw := range mws {
			mw(c)
			if c.IsAborted() {
				return
			}
		}
		handler(c)
	}
}

// Registered with router.Use(), the metric must capture the handler's time even
// when the handler runs inside an oapi-codegen wrapper.
func TestHTTPMetricsOnRouterUseCapturesHandlerInsideOapiWrapper(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(HTTPMetricsMiddleware()) // gin's real chain — the correct placement
	r.GET("/w", oapiWrapper(
		[]gin.HandlerFunc{func(c *gin.Context) { c.Next() }}, // a list middleware
		func(c *gin.Context) {
			time.Sleep(60 * time.Millisecond) // the handler's real work
			c.Status(http.StatusOK)
		},
	))
	do(r, http.MethodGet, "/w")

	var m dto.Metric
	obs, err := requestDuration.GetMetricWithLabelValues(http.MethodGet, "/w", "2xx")
	require.NoError(t, err)
	require.NoError(t, obs.(prometheus.Metric).Write(&m))

	assert.GreaterOrEqual(t, m.GetHistogram().GetSampleSum(), 0.05,
		"handler time was not captured — HTTPMetricsMiddleware has been moved back "+
			"into APIOptions.Middlewares, where c.Next() returns before the handler runs")
}

// The inverse, pinning WHY the placement matters: inside the oapi list the very
// same middleware measures essentially nothing. If this ever starts recording
// real time, oapi-codegen changed its convention and the comments should follow.
func TestHTTPMetricsInsideOapiListMeasuresNothing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/bad", oapiWrapper(
		[]gin.HandlerFunc{HTTPMetricsMiddleware()}, // the WRONG placement
		func(c *gin.Context) {
			time.Sleep(60 * time.Millisecond)
			c.Status(http.StatusOK)
		},
	))
	do(r, http.MethodGet, "/bad")

	var m dto.Metric
	obs, err := requestDuration.GetMetricWithLabelValues(http.MethodGet, "/bad", "2xx")
	require.NoError(t, err)
	require.NoError(t, obs.(prometheus.Metric).Write(&m))

	assert.Less(t, m.GetHistogram().GetSampleSum(), 0.02,
		"oapi-codegen now chains middlewares properly; revisit the placement note")
}

// A panicking handler must still be counted. The observation is deferred so it
// survives the unwind to gin.Recovery(); 500s from panics are the ones most
// worth seeing, and the pre-defer version dropped them silently.
func TestHTTPMetricsRecordsPanickingRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Metrics OUTSIDE Recovery — the production order. Reversed, Recovery has
	// not yet set the 500 when the deferred observation runs, and the request is
	// misfiled as 2xx.
	r := gin.New()
	r.Use(HTTPMetricsMiddleware())
	r.Use(gin.Recovery())
	r.GET("/boom", func(c *gin.Context) { panic("kaboom") })

	before := observations(t, requestDuration, http.MethodGet, "/boom", "5xx")
	do(r, http.MethodGet, "/boom")

	assert.Equal(t, uint64(1),
		observations(t, requestDuration, http.MethodGet, "/boom", "5xx")-before,
		"a panicking request must still be observed")
}

// The scrape endpoint must not appear in its own histogram: Prometheus polls it
// on every interval, and those fast requests flatter every aggregate.
func TestHTTPMetricsExcludesTheScrapeEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(HTTPMetricsMiddleware())
	MountMetricsEndpoint(r)
	do(r, http.MethodGet, "/metrics")

	obs, err := requestDuration.GetMetricWithLabelValues(http.MethodGet, "/metrics", "2xx")
	require.NoError(t, err)
	var m dto.Metric
	require.NoError(t, obs.(prometheus.Metric).Write(&m))
	assert.Zero(t, m.GetHistogram().GetSampleCount(), "/metrics must not observe itself")
}
