package kratos

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	ory "github.com/ory/kratos-client-go"
)

// The MFA and AAL endpoints read the raw Cookie header and forwarded it to
// Kratos, so they worked for a browser and for nothing else. Three failed
// closed with a 401, which is right. GetSessionAALInfo did not: with no cookie
// it answered "aal1, and this user cannot upgrade" with a nil error -- a
// confident claim about someone's MFA capability built from no information.
//
// For a browser that was harmless, because no cookie means no session and the
// caller was about to 401 anyway. It stopped being harmless the moment a native
// client called it: that client holds a perfectly good session and was told it
// has no MFA -- and checkAALRequirements then lets the mutation through,
// because Current == Available.

// emptyContext carries no credential of any kind.
func emptyContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c
}

func nativeTokenContext(token string) *gin.Context {
	c := emptyContext()
	c.Request.Header.Set("X-Session-Token", token)
	return c
}

// providerRefusingToBeCalled fails the test if Kratos is contacted at all.
func providerRefusingToBeCalled(t *testing.T) *KratosAuthProvider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Kratos was called (%s) for a request carrying no credential", r.URL.Path)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	cfg := ory.NewConfiguration()
	cfg.Servers = ory.ServerConfigurations{{URL: srv.URL}}
	client := ory.NewAPIClient(cfg)
	return &KratosAuthProvider{adminClient: client, publicClient: client}
}

// The one that used to fail open-ish.
func TestGetSessionAALInfoRefusesWithoutCredential(t *testing.T) {
	p := providerRefusingToBeCalled(t)

	info, err := p.GetSessionAALInfo(emptyContext())
	if err == nil {
		t.Fatalf("expected an error with no credential, got %+v", info)
	}
	if info != nil {
		t.Errorf("a fabricated AAL default must not be returned, got %+v", info)
	}
}

func TestGetMFAStatusRefusesWithoutCredential(t *testing.T) {
	p := providerRefusingToBeCalled(t)

	if _, err := p.GetMFAStatus(emptyContext()); err == nil {
		t.Fatal("expected an error with no credential, got nil")
	}
}

func TestDisableWebAuthnRefusesWithoutCredential(t *testing.T) {
	p := providerRefusingToBeCalled(t)

	if err := p.DisableWebAuthn(emptyContext()); err == nil {
		t.Fatal("expected an error with no credential, got nil")
	}
}

func TestInitializeSettingsFlowRefusesWithoutCredential(t *testing.T) {
	p := providerRefusingToBeCalled(t)

	if _, err := p.InitializeSettingsFlow(emptyContext()); err == nil {
		t.Fatal("expected an error with no credential, got nil")
	}
}

// A native client must be able to ask about its own MFA, which is the half of
// this defect that is blocking rather than merely wrong.
func TestGetSessionAALInfoAcceptsANativeToken(t *testing.T) {
	var got http.Header
	p := providerRecordingHeaders(t, &got)

	// The admin identity lookup that follows is allowed to fail; availableAAL
	// simply stays aal1. What matters is that the session was resolved at all.
	if _, err := p.GetSessionAALInfo(nativeTokenContext("native-session-token")); err != nil {
		t.Fatalf("a native client must be able to resolve its session: %v", err)
	}

	if got.Get("X-Session-Token") != "native-session-token" {
		t.Errorf("X-Session-Token = %q, want the native token", got.Get("X-Session-Token"))
	}
	if cookie := got.Get("Cookie"); cookie != "" {
		t.Errorf("a native token must not be re-wrapped as a cookie, got Cookie: %q", cookie)
	}
}
