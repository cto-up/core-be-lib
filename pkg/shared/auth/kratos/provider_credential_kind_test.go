package kratos

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ctoup.com/coreapp/pkg/shared/auth"
	"github.com/gin-gonic/gin"
	ory "github.com/ory/kratos-client-go"
)

// These tests assert what Kratos RECEIVES, not merely what we return. A test
// that only checked for a non-nil token would stay green with the credential in
// the wrong slot, because the stub answers whatever it is asked -- and the
// wrong slot is the entire defect: a native session token re-wrapped as
// ory_kratos_session=<token> does not decode, and Kratos says "no active
// session", a 401 indistinguishable from a wrong password.

// schema_id is required on an Ory identity: without it the SDK refuses to
// deserialize the response, and the call fails before any assertion is reached.
const activeSessionJSON = `{"id":"5f2a1c88-0000-4000-8000-000000000001","active":true,` +
	`"identity":{"id":"9d4b7e10-0000-4000-8000-000000000002","schema_id":"default",` +
	`"schema_url":"http://kratos.test/schemas/default","state":"active",` +
	`"traits":{"email":"who@example.com"}}}`

// clientRecordingHeaders captures the headers of the whoami call so a test can
// assert which slot the credential travelled in.
func clientRecordingHeaders(t *testing.T, got *http.Header) *KratosAuthClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions/whoami" {
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		*got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(activeSessionJSON))
	}))
	t.Cleanup(srv.Close)

	cfg := ory.NewConfiguration()
	cfg.Servers = ory.ServerConfigurations{{URL: srv.URL}}
	client := ory.NewAPIClient(cfg)

	return NewKratosAuthClient(client, client)
}

func TestNativeTokenIsSentAsXSessionToken(t *testing.T) {
	var got http.Header
	client := clientRecordingHeaders(t, &got)

	tok, err := client.VerifySessionCredential(context.Background(), auth.SessionCredential{
		Value: "native-session-token",
		Kind:  auth.CredentialNativeToken,
	})
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if tok == nil || tok.UID == "" {
		t.Fatalf("expected a token with a UID, got %+v", tok)
	}

	if got.Get("X-Session-Token") != "native-session-token" {
		t.Errorf("X-Session-Token = %q, want the native token", got.Get("X-Session-Token"))
	}
	// The whole point: it must NOT have been re-wrapped as a cookie.
	if cookie := got.Get("Cookie"); cookie != "" {
		t.Errorf("a native token must not travel in the cookie slot, got Cookie: %q", cookie)
	}
}

func TestCookieCredentialIsSentAsCookie(t *testing.T) {
	var got http.Header
	client := clientRecordingHeaders(t, &got)

	if _, err := client.VerifySessionCredential(context.Background(), auth.SessionCredential{
		Value: "browser-cookie-value",
		Kind:  auth.CredentialCookie,
	}); err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	if want := "ory_kratos_session=browser-cookie-value"; got.Get("Cookie") != want {
		t.Errorf("Cookie = %q, want %q", got.Get("Cookie"), want)
	}
	if got.Get("X-Session-Token") != "" {
		t.Errorf("a browser cookie must not travel as a native token, got %q", got.Get("X-Session-Token"))
	}
}

// VerifyIDToken keeps its old meaning to the byte. It is on the exported
// AuthClient interface with callers outside this module (tooling-lib's OAuth
// consent gate, skeells via VerifyTokenWithTenantID), and they must not change
// behaviour because this commit landed.
func TestVerifyIDTokenStillSendsACookie(t *testing.T) {
	var got http.Header
	client := clientRecordingHeaders(t, &got)

	if _, err := client.VerifyIDToken(context.Background(), "legacy-value"); err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	if want := "ory_kratos_session=legacy-value"; got.Get("Cookie") != want {
		t.Errorf("Cookie = %q, want %q", got.Get("Cookie"), want)
	}
	if got.Get("X-Session-Token") != "" {
		t.Errorf("VerifyIDToken must not start sending a native token, got %q", got.Get("X-Session-Token"))
	}
}

// providerRecordingHeaders builds a provider whose Kratos clients both point at
// a stub that records what it was asked.
func providerRecordingHeaders(t *testing.T, got *http.Header) *KratosAuthProvider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions/whoami" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		*got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(activeSessionJSON))
	}))
	t.Cleanup(srv.Close)

	cfg := ory.NewConfiguration()
	cfg.Servers = ory.ServerConfigurations{{URL: srv.URL}}
	client := ory.NewAPIClient(cfg)

	return &KratosAuthProvider{adminClient: client, publicClient: client}
}

// This is hub#183 end to end: an Android client sends its Kratos API-flow token
// in X-Session-Token, and that is the slot it has to reach Kratos in.
//
// This test deliberately uses only VerifyToken, so it compiles against the code
// as it stood before this commit as well -- which is what makes it a genuine
// red/green proof. There, it fails: the token was re-wrapped as a cookie.
func TestVerifyTokenSendsANativeTokenNatively(t *testing.T) {
	var got http.Header
	provider := providerRecordingHeaders(t, &got)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("X-Session-Token", "native-session-token")

	// Tenancy resolution after verification is not what this test is about --
	// with no tenant on the request it ends in an error either way. The only
	// question here is which slot the credential travelled in.
	_, _ = provider.VerifyToken(c)

	if got.Get("X-Session-Token") != "native-session-token" {
		t.Errorf("X-Session-Token = %q, want the native token to reach Kratos natively", got.Get("X-Session-Token"))
	}
	if cookie := got.Get("Cookie"); cookie != "" {
		t.Errorf("a native token must not be re-wrapped as a cookie, got Cookie: %q", cookie)
	}
}

// The browser side of the same entry point must be untouched.
func TestVerifyTokenStillSendsABrowserCookie(t *testing.T) {
	var got http.Header
	provider := providerRecordingHeaders(t, &got)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Cookie", "ory_kratos_session=browser-cookie-value")

	_, _ = provider.VerifyToken(c)

	if want := "ory_kratos_session=browser-cookie-value"; got.Get("Cookie") != want {
		t.Errorf("Cookie = %q, want %q", got.Get("Cookie"), want)
	}
	if got.Get("X-Session-Token") != "" {
		t.Errorf("a browser cookie must not become a native token, got %q", got.Get("X-Session-Token"))
	}
}
