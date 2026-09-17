package kratos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	ory "github.com/ory/kratos-client-go"
)

// Nothing covered VerifyToken, VerifyIDToken or ToSession -- the path every
// authenticated request in every consumer travels. These fill the gaps left by
// the commits that came with the fixes themselves:
//
//	credential_test.go            the extraction order and every provenance
//	provider_credential_kind_test.go  which slot each kind reaches Kratos in
//	provider_identity_nil_test.go     an active session carrying no identity
//
// What remained unpinned, and is pinned here: a bearer credential end to end,
// the no-credential case, and VerifyIDToken's own answer for an inactive and
// for a healthy session.

// clientServingJSON answers whoami with a given body, for the cases that need a
// session other than the healthy one.
func clientServingJSON(t *testing.T, body string) *KratosAuthClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions/whoami" {
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	cfg := ory.NewConfiguration()
	cfg.Servers = ory.ServerConfigurations{{URL: srv.URL}}
	client := ory.NewAPIClient(cfg)

	return NewKratosAuthClient(client, client)
}

// A bearer credential is native, so it must reach Kratos natively. Before
// hub#183 it was flattened in with the rest and presented as a cookie, which
// could only ever have worked for a caller pasting a cookie value into an
// Authorization header.
func TestVerifyTokenSendsABearerNatively(t *testing.T) {
	var got http.Header
	provider := providerRecordingHeaders(t, &got)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer bearer-session-token")

	_, _ = provider.VerifyToken(c)

	if got.Get("X-Session-Token") != "bearer-session-token" {
		t.Errorf("X-Session-Token = %q, want the bearer token", got.Get("X-Session-Token"))
	}
	if cookie := got.Get("Cookie"); cookie != "" {
		t.Errorf("a bearer must not be re-wrapped as a cookie, got Cookie: %q", cookie)
	}
}

// No credential must fail before Kratos is troubled at all: asking the provider
// to resolve a session nobody presented is a bug, not a lookup.
func TestVerifyTokenWithoutCredentialNeverCallsKratos(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	cfg := ory.NewConfiguration()
	cfg.Servers = ory.ServerConfigurations{{URL: srv.URL}}
	client := ory.NewAPIClient(cfg)
	provider := &KratosAuthProvider{adminClient: client, publicClient: client}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	user, err := provider.VerifyToken(c)
	if err == nil {
		t.Fatalf("expected an error with no credential, got user %+v", user)
	}
	if called {
		t.Error("Kratos was called for a request that carried no credential at all")
	}
}

func TestVerifyIDTokenRejectsAnInactiveSession(t *testing.T) {
	const inactive = `{"id":"5f2a1c88-0000-4000-8000-000000000001","active":false,` +
		`"identity":{"id":"9d4b7e10-0000-4000-8000-000000000002","schema_id":"default",` +
		`"schema_url":"http://kratos.test/schemas/default","state":"active",` +
		`"traits":{"email":"who@example.com"}}}`

	client := clientServingJSON(t, inactive)

	tok, err := client.VerifyIDToken(context.Background(), "some-credential")
	if err == nil {
		t.Fatalf("expected an error for an inactive session, got %+v", tok)
	}
	if tok != nil {
		t.Errorf("expected no token alongside the error, got %+v", tok)
	}
}

// The healthy path, so the suite says what success looks like and not only what
// each failure looks like.
func TestVerifyIDTokenReturnsTheIdentityAndItsTraits(t *testing.T) {
	client := clientServingJSON(t, activeSessionJSON)

	tok, err := client.VerifyIDToken(context.Background(), "some-credential")
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if want := "9d4b7e10-0000-4000-8000-000000000002"; tok.UID != want {
		t.Errorf("UID = %q, want %q", tok.UID, want)
	}
	if email, _ := tok.Claims["email"].(string); email != "who@example.com" {
		t.Errorf("claims[email] = %q, want the identity's trait", email)
	}
}
