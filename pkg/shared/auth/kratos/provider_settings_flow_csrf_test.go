package kratos

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	ory "github.com/ory/kratos-client-go"
)

// A browser settings flow binds its csrf_token to the anti-CSRF cookie Kratos
// sees when the flow is created. Forwarding only ory_kratos_session made Kratos
// mint a fresh CSRF cookie that never reached the browser, so the browser's
// submit (WebAuthn, TOTP) was refused with security_csrf_violation.
func TestInitializeSettingsFlowForwardsTheBrowsersCSRFCookie(t *testing.T) {
	const csrf = "csrf_token_abc=browser-csrf"
	const minted = "csrf_token_abc=minted; Path=/; HttpOnly"

	var gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/self-service/settings/browser" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		gotCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Add("Set-Cookie", minted)
		_, _ = w.Write([]byte(`{"id":"f","type":"browser","expires_at":"2030-01-01T00:00:00Z","issued_at":"2026-01-01T00:00:00Z","request_url":"http://x","state":"show_form","identity":{"id":"i","schema_id":"default","schema_url":"http://x","traits":{}},"ui":{"action":"http://x","method":"POST","nodes":[]}}`))
	}))
	t.Cleanup(srv.Close)

	cfg := ory.NewConfiguration()
	cfg.Servers = ory.ServerConfigurations{{URL: srv.URL}}
	client := ory.NewAPIClient(cfg)
	p := &KratosAuthProvider{adminClient: client, publicClient: client}

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Request.Header.Set("Cookie", "ory_kratos_session=sess; "+csrf)

	if _, err := p.InitializeSettingsFlow(c); err != nil {
		t.Fatalf("InitializeSettingsFlow: %v", err)
	}
	if gotCookie != "ory_kratos_session=sess; "+csrf {
		t.Errorf("Kratos saw Cookie %q, want the session and the browser's CSRF cookie", gotCookie)
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 1 || got[0] != minted {
		t.Errorf("Set-Cookie passed to the browser = %q, want %q", got, minted)
	}
}
