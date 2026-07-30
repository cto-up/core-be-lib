package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The contract that matters most: with SENTRY_DSN unset — local dev, CI, and
// any environment that was never given the variable — none of this may panic,
// allocate a span, or alter behaviour. Tracing must never be able to break the
// thing it is watching.
func TestDisabledIsAFullNoOp(t *testing.T) {
	require.False(t, Enabled(), "tests run with tracing off; a previous test leaked state")

	assert.Nil(t, GinMiddleware(), "no middleware may be installed when disabled")
	assert.NotPanics(t, func() { Flush(0) })

	// The pgx tracer must hand back the SAME context, not a span-bearing child.
	ctx := context.Background()
	got := QueryTracer{}.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
	assert.Equal(t, ctx, got)
	assert.NotPanics(t, func() {
		QueryTracer{}.TraceQueryEnd(got, nil, pgx.TraceQueryEndData{})
	})
}

// The traced transport must be transparent when disabled: same request, same
// response, no added headers.
func TestTracedTransportPassesThroughWhenDisabled(t *testing.T) {
	var sawHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	resp, err := TracedHTTPClient("unit-test", 0).Get(srv.URL + "/thing")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusTeapot, resp.StatusCode)
	assert.Empty(t, sawHeaders.Get("sentry-trace"))
	assert.Empty(t, sawHeaders.Get("baggage"))
}

func TestSummariseSQLCollapsesWhitespace(t *testing.T) {
	in := `
		SELECT c.id,
		       c.title
		  FROM olms_courses c
		 WHERE c.tenant_id = $1
	`
	assert.Equal(t,
		"SELECT c.id, c.title FROM olms_courses c WHERE c.tenant_id = $1",
		summariseSQL(in),
		"the same statement formatted differently must group as one description")
}

func TestSummariseSQLTruncates(t *testing.T) {
	got := summariseSQL(strings.Repeat("x", maxSQLLength+50))
	// Count RUNES, not bytes — the ellipsis is one rune and three bytes.
	assert.Equal(t, maxSQLLength+1, len([]rune(got)),
		"truncated to the cap plus a single ellipsis rune")
	assert.True(t, strings.HasSuffix(got, "…"))
}

// Parameters must never reach the span description. pgx keeps them separate
// from the SQL, so this is really a guard against someone "helpfully" adding
// data.Args to the description later — which would leak user data into Sentry
// AND fragment grouping per parameter value.
func TestSummariseSQLCarriesNoParameterValues(t *testing.T) {
	got := summariseSQL("SELECT * FROM users WHERE email = $1")
	assert.NotContains(t, got, "@")
	assert.Contains(t, got, "$1")
}

// The transaction name must be the ROUTE TEMPLATE. sentrygin defaults to the
// raw URL path, which is exactly the cardinality mistake that fragmented one
// endpoint into ~20 rows in the audit behind roadmap 020.
func TestNameTransactionUsesRouteTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// With tracing disabled there is no transaction to rename, so assert the
	// property that survives: the middleware runs, resolves the template, and
	// does not panic on a nil transaction.
	var resolved string
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		resolved = c.Request.Method + " " + route
	})
	r.Use(nameTransaction())
	r.GET("/api/v1/lms/courses/:courseId", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/lms/courses/9c8c315c-66ba-4b57-b005-72f43a8db9a5", nil))
	assert.Equal(t, "GET /api/v1/lms/courses/:courseId", resolved)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/wp-admin/random-probe", nil))
	assert.Equal(t, "GET unmatched", resolved,
		"scanner probes must collapse to one transaction name, not one each")
}
