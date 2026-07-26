package core

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ctoup.com/coreapp/pkg/shared/observability"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exist because roadmap 020 Tier 1 changed the auth entry in the API
// middleware chain from
//
//	core.MiddlewareFunc(authSlot.handle)
//
// to
//
//	core.MiddlewareFunc(observability.TimeAuthMiddleware(authSlot.handle))
//
// so the Kratos round-trip could be timed separately (roadmap 018 T5 asked for
// exactly that number before the backend session cache could be reconsidered).
//
// The auth middleware is NOT removed — it is passed as `next`. But the wrapping
// puts a closure between the chain and the slot, and the slot's whole purpose is
// LATE MUTATION: WrapAuthMiddleware rewrites authSlot.inner after every route
// has already been registered, and relies on the chain holding a stable method
// value that re-reads `inner` on each call. If the wrapper had captured
// `authSlot.inner` instead of `authSlot.handle`, auth would silently freeze at
// its boot-time value and every later WrapAuthMiddleware call would be a no-op —
// a security regression with no error and no failing build.
//
// So: prove auth still runs, prove it can still be replaced late, and prove it
// can still reject.

// chain mirrors the auth position of the real chain built in
// initializeServerConfig: the timing wrapper around the slot's bound method.
func chain(t *testing.T, slot *authMiddlewareSlot, handler gin.HandlerFunc) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.HandlerFunc(observability.TimeAuthMiddleware(slot.handle)))
	r.GET("/protected", handler)
	return r
}

func do(r *gin.Engine) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))
	return w
}

// The auth middleware still executes — it was wrapped, not dropped.
func TestTimedAuthStillInvokesTheAuthMiddleware(t *testing.T) {
	called := 0
	slot := &authMiddlewareSlot{inner: func(c *gin.Context) {
		called++
		c.Set("authed", true)
		c.Next()
	}}

	var sawAuthed bool
	r := chain(t, slot, func(c *gin.Context) {
		sawAuthed = c.GetBool("authed")
		c.Status(http.StatusOK)
	})

	require.Equal(t, http.StatusOK, do(r).Code)
	assert.Equal(t, 1, called, "auth middleware must still run")
	assert.True(t, sawAuthed, "values auth sets must still reach the handler")
}

// Auth can still REJECT. A wrapper that swallowed the abort would turn every
// unauthenticated request into a 200.
func TestTimedAuthCanStillAbort(t *testing.T) {
	slot := &authMiddlewareSlot{inner: func(c *gin.Context) {
		c.AbortWithStatus(http.StatusUnauthorized)
	}}

	handlerRan := false
	r := chain(t, slot, func(c *gin.Context) {
		handlerRan = true
		c.Status(http.StatusOK)
	})

	assert.Equal(t, http.StatusUnauthorized, do(r).Code)
	assert.False(t, handlerRan, "an aborted auth must not reach the handler")
}

// THE ONE THAT MATTERS. WrapAuthMiddleware mutates authSlot.inner long after
// routes are registered. The timing wrapper must not have frozen the old value.
func TestWrapAuthMiddlewareStillTakesEffectThroughTheTimingWrapper(t *testing.T) {
	slot := &authMiddlewareSlot{inner: func(c *gin.Context) { c.Next() }}
	sc := &ServerConfig{authSlot: slot}

	order := []string{}
	r := chain(t, slot, func(c *gin.Context) {
		order = append(order, "handler")
		c.Status(http.StatusOK)
	})

	// Routes are already registered at this point — exactly the real situation.
	sc.WrapAuthMiddleware(func(next gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			order = append(order, "wrap-1")
			next(c)
		}
	})

	require.Equal(t, http.StatusOK, do(r).Code)
	assert.Equal(t, []string{"wrap-1", "handler"}, order,
		"a WrapAuthMiddleware applied after route registration must still run")
}

// Wrappers compose, last-registered outermost, as documented.
func TestWrapAuthMiddlewareStillComposes(t *testing.T) {
	slot := &authMiddlewareSlot{inner: func(c *gin.Context) { c.Next() }}
	sc := &ServerConfig{authSlot: slot}

	order := []string{}
	r := chain(t, slot, func(c *gin.Context) {
		order = append(order, "handler")
		c.Status(http.StatusOK)
	})

	for _, name := range []string{"first", "second"} {
		n := name
		sc.WrapAuthMiddleware(func(next gin.HandlerFunc) gin.HandlerFunc {
			return func(c *gin.Context) {
				order = append(order, n)
				next(c)
			}
		})
	}

	require.Equal(t, http.StatusOK, do(r).Code)
	assert.Equal(t, []string{"second", "first", "handler"}, order,
		"the last WrapAuthMiddleware call must run outermost")
}

// A late wrapper that short-circuits must still be able to deny.
func TestLateWrapCanStillShortCircuitAuth(t *testing.T) {
	slot := &authMiddlewareSlot{inner: func(c *gin.Context) { c.Next() }}
	sc := &ServerConfig{authSlot: slot}

	innerRan := false
	r := chain(t, slot, func(c *gin.Context) {
		innerRan = true
		c.Status(http.StatusOK)
	})

	sc.WrapAuthMiddleware(func(gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) { c.AbortWithStatus(http.StatusForbidden) }
	})

	assert.Equal(t, http.StatusForbidden, do(r).Code)
	assert.False(t, innerRan)
}
