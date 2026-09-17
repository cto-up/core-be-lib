package kratos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	ory "github.com/ory/kratos-client-go"
)

// Kratos can answer with an active session that carries no identity. Every site
// below guarded that pointer somewhere and then dereferenced it anyway --
// VerifyIDToken three lines after its own guard closed -- so the authentication
// hot path panicked instead of refusing the session.
//
// A 401 is the right answer. A panic recovered into a 500 is not, and a token
// with an empty UID would be worse than either: it authenticates as nobody.

// identityLessWhoami answers /sessions/whoami with an active, identity-less
// session. Everything else is a test failure: these tests must not pass because
// the stub quietly 404'd.
func identityLessWhoami(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions/whoami" {
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1e2d3c4b-0000-0000-0000-000000000000","active":true}`))
	})
}

// newProviderWithStub builds a provider whose public and admin clients both
// point at the stub. multitenantService stays nil: none of the paths under test
// reach it before refusing the session, which is itself part of the contract.
func newProviderWithStub(t *testing.T, h http.Handler) *KratosAuthProvider {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	cfg := ory.NewConfiguration()
	cfg.Servers = ory.ServerConfigurations{{URL: srv.URL}}
	client := ory.NewAPIClient(cfg)

	return &KratosAuthProvider{adminClient: client, publicClient: client}
}

// testContextWithCookie carries a Cookie header, so these tests exercise the
// identity-nil path rather than the no-credential early return.
func testContextWithCookie() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Cookie", "ory_kratos_session=irrelevant-to-the-stub")
	return c
}

func TestVerifyIDTokenRefusesSessionWithoutIdentity(t *testing.T) {
	client := newTestClient(t, identityLessWhoami(t))

	tok, err := client.VerifyIDToken(context.Background(), "any-credential")
	if err == nil {
		t.Fatalf("expected an error for a session with no identity, got token %+v", tok)
	}
	if tok != nil {
		t.Errorf("expected no token alongside the error, got %+v", tok)
	}
}

func TestGetSessionAALInfoRefusesSessionWithoutIdentity(t *testing.T) {
	p := newProviderWithStub(t, identityLessWhoami(t))

	info, err := p.GetSessionAALInfo(testContextWithCookie())
	if err == nil {
		t.Fatalf("expected an error for a session with no identity, got %+v", info)
	}
}

func TestGetMFAStatusRefusesSessionWithoutIdentity(t *testing.T) {
	p := newProviderWithStub(t, identityLessWhoami(t))

	if _, err := p.GetMFAStatus(testContextWithCookie()); err == nil {
		t.Fatal("expected an error for a session with no identity, got nil")
	}
}

func TestDisableWebAuthnRefusesSessionWithoutIdentity(t *testing.T) {
	p := newProviderWithStub(t, identityLessWhoami(t))

	if err := p.DisableWebAuthn(testContextWithCookie()); err == nil {
		t.Fatal("expected an error for a session with no identity, got nil")
	}
}
