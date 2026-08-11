package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oapiWrapper reproduces exactly what oapi-codegen's generated gin server does
// with GinServerOptions.Middlewares: it calls each entry as a plain function,
// then the handler. Crucially the middleware is NOT part of gin's handler
// chain, so a c.Next() inside it advances past the end and returns immediately
// — everything the middleware does after c.Next() therefore happens BEFORE the
// handler runs.
//
// Every generated route in every consumer is mounted this way, which is why the
// tests below assert against this shape rather than a plain router.Use() chain.
func oapiWrapper(mws []gin.HandlerFunc, h gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, mw := range mws {
			mw(c)
			if c.IsAborted() {
				return
			}
		}
		h(c)
	}
}

type accessLine struct {
	Status   int     `json:"status"`
	Route    string  `json:"route"`
	Duration float64 `json:"duration_ms"`
	TenantID string  `json:"tenant_id"`
	UserID   string  `json:"user_id"`
	Message  string  `json:"message"`
}

// captureAccessLog swaps the global zerolog sink for the duration of a test and
// returns the decoded "Request handled" line.
func captureAccessLog(t *testing.T, run func()) accessLine {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf)
	defer func() { log.Logger = prev }()

	run()

	var out accessLine
	for _, raw := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		if len(raw) == 0 {
			continue
		}
		var line accessLine
		require.NoError(t, json.Unmarshal(raw, &line), "log line: %s", raw)
		if line.Message == "Request handled" {
			out = line
		}
	}
	require.Equal(t, "Request handled", out.Message, "no access log line emitted")
	return out
}

// The access log must observe the status the handler actually returned.
//
// REGRESSION GUARD (roadmap 020): RequestIDMiddleware used to sit in
// APIOptions.Middlewares, where its post-c.Next() logging ran before the
// handler and recorded the default 200 with a ~0 ms duration for every
// generated route. A 500 was logged as a success, so backend errors were
// invisible in Loki while every panel still rendered plausible numbers. Moving
// it back into the middleware list makes this test fail — that is its job.
func TestAccessLogRecordsHandlerStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	line := captureAccessLog(t, func() {
		r := gin.New()
		r.Use(RequestIDMiddleware())
		r.GET("/gen", oapiWrapper(nil, func(c *gin.Context) {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "boom"})
		}))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/gen", nil))
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	assert.Equal(t, http.StatusInternalServerError, line.Status,
		"access log must report the handler's status, not the default 200")
	assert.Equal(t, "/gen", line.Route)
}

// The access log must carry the identity stamped on by LoggerEnrichmentMiddleware,
// which runs later, inside the generated wrapper. Same root cause as above: while
// RequestIDMiddleware shared that list it finished first and re-read a logger
// nothing had enriched yet, so tenant_id/user_id were absent from every line.
func TestAccessLogCarriesEnrichedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	line := captureAccessLog(t, func() {
		r := gin.New()
		r.Use(RequestIDMiddleware())

		stubAuth := func(c *gin.Context) {
			c.Set("auth_tenant_id", "tenant-123")
			c.Set("auth_user_id", "user-456")
			c.Next()
		}
		r.GET("/gen", oapiWrapper(
			[]gin.HandlerFunc{stubAuth, LoggerEnrichmentMiddleware()},
			func(c *gin.Context) { c.Status(http.StatusOK) },
		))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/gen", nil))
		require.Equal(t, http.StatusOK, w.Code)
	})

	assert.Equal(t, "tenant-123", line.TenantID)
	assert.Equal(t, "user-456", line.UserID)
}

// A panic must be logged as the 500 gin.Recovery() turns it into.
//
// This pins RequestIDMiddleware OUTSIDE Recovery. Inside it, the panic unwinds
// through this middleware and the logging after c.Next() is skipped altogether,
// so the request that most needs a log line produces none — verified below by
// the inverted case.
func TestAccessLogRecordsRecoveredPanicAs500(t *testing.T) {
	gin.SetMode(gin.TestMode)

	line := captureAccessLog(t, func() {
		r := gin.New()
		r.Use(RequestIDMiddleware())
		r.Use(gin.RecoveryWithWriter(nil))
		r.GET("/gen", oapiWrapper(nil, func(c *gin.Context) { panic("handler exploded") }))

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/gen", nil))
		require.Equal(t, http.StatusInternalServerError, w.Code)
	})

	assert.Equal(t, http.StatusInternalServerError, line.Status)
}

// Documents why the ordering above is not arbitrary: with Recovery outermost,
// a panicking request is logged not incorrectly but not at all.
func TestAccessLogInsideRecoveryLosesPanicEntirely(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	prev := log.Logger
	log.Logger = zerolog.New(&buf)
	defer func() { log.Logger = prev }()

	r := gin.New()
	r.Use(gin.RecoveryWithWriter(nil))
	r.Use(RequestIDMiddleware()) // the wrong way round, on purpose
	r.GET("/gen", oapiWrapper(nil, func(c *gin.Context) { panic("handler exploded") }))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/gen", nil))
	require.Equal(t, http.StatusInternalServerError, w.Code)

	assert.NotContains(t, buf.String(), "Request handled",
		"a panic unwinding through the middleware skips its logging entirely")
}

// Machine-polled infrastructure endpoints stay out of the access log; they are
// continuous and would outnumber real traffic.
func TestAccessLogSkipsInfraPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, path := range []string{"/metrics", "/healthz"} {
		t.Run(path, func(t *testing.T) {
			var buf bytes.Buffer
			prev := log.Logger
			log.Logger = zerolog.New(&buf)
			defer func() { log.Logger = prev }()

			r := gin.New()
			r.Use(RequestIDMiddleware())
			r.GET(path, func(c *gin.Context) { c.Status(http.StatusOK) })

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, http.StatusOK, w.Code)

			assert.NotContains(t, buf.String(), "Request handled")
		})
	}
}
