package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestTimeAuthMiddlewareLabelsOutcome(t *testing.T) {
	gin.SetMode(gin.TestMode)

	passedBefore := observations(t, authDuration, "passed")
	abortedBefore := observations(t, authDuration, "aborted")

	r := gin.New()
	r.Use(TimeAuthMiddleware(func(c *gin.Context) { c.Next() }))
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })
	do(r, http.MethodGet, "/ok")

	deny := gin.New()
	deny.Use(TimeAuthMiddleware(func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) }))
	deny.GET("/no", func(c *gin.Context) { c.Status(http.StatusOK) })
	do(deny, http.MethodGet, "/no")

	assert.Equal(t, uint64(1), observations(t, authDuration, "passed")-passedBefore)
	assert.Equal(t, uint64(1), observations(t, authDuration, "aborted")-abortedBefore,
		"a rejected request short-circuits before the Kratos call and must not be averaged in with successes")
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
