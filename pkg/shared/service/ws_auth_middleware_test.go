package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// The WebSocket path promotes ?token= into X-Session-Token before the
// credential is extracted, so whatever a client puts in that parameter is read
// as a native session token. These pin the three rules that decide what gets
// promoted -- the cookie has to keep winning, or a browser upgrade would start
// authenticating with whatever happened to be in the URL.

func wsContext(t *testing.T, target string, headers map[string]string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range headers {
		c.Request.Header.Set(k, v)
	}
	return c
}

func TestQueryTokenIsPromotedWhenThereIsNoCookie(t *testing.T) {
	c := wsContext(t, "/care_ws?token=native-tok&subdomain=acme", nil)

	promoteQueryToken(c)

	if got := c.GetHeader("X-Session-Token"); got != "native-tok" {
		t.Errorf("X-Session-Token = %q, want the query token promoted", got)
	}
}

// A browser on a same-origin upgrade sends its cookie automatically. If the
// query parameter won, the URL would decide who you are.
func TestCookieBeatsTheQueryToken(t *testing.T) {
	c := wsContext(t, "/care_ws?token=native-tok", map[string]string{
		"Cookie": "ory_kratos_session=cookie-val",
	})

	promoteQueryToken(c)

	if got := c.GetHeader("X-Session-Token"); got != "" {
		t.Errorf("the query token must not be promoted over a cookie, got %q", got)
	}
}

func TestAnExistingHeaderIsNeverOverwritten(t *testing.T) {
	c := wsContext(t, "/care_ws?token=from-url", map[string]string{
		"X-Session-Token": "from-header",
	})

	promoteQueryToken(c)

	if got := c.GetHeader("X-Session-Token"); got != "from-header" {
		t.Errorf("X-Session-Token = %q, want the original header untouched", got)
	}
}

func TestNoQueryTokenLeavesTheRequestAlone(t *testing.T) {
	c := wsContext(t, "/care_ws?subdomain=acme", nil)

	promoteQueryToken(c)

	if got := c.GetHeader("X-Session-Token"); got != "" {
		t.Errorf("X-Session-Token = %q, want nothing invented", got)
	}
}
