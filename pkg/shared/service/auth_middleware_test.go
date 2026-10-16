package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"ctoup.com/coreapp/pkg/shared/auth"
)

// tokenRouter mounts the auth middleware behind a stand-in for TenantMiddleware
// that resolves the request to requestTenant ("" = no tenant context).
func tokenRouter(am *AuthMiddleware, requestTenant string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if requestTenant != "" {
			c.Set(auth.AUTH_TENANT_ID_KEY, requestTenant)
		}
	}, am.MiddlewareFunc())
	r.Any("/api/v1/things", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

type sessionProvider struct {
	auth.AuthProvider
	user *auth.AuthenticatedUser
}

func (p sessionProvider) VerifyToken(*gin.Context) (*auth.AuthenticatedUser, error) {
	return p.user, nil
}

func (sessionProvider) GetProviderName() string { return "test" }

func TestAnAPITokenOnlyActsInItsOwnTenant(t *testing.T) {
	svc, _, ctx := setupTestAPITokenService(t)
	app := createTestClientApplication(t, svc)
	token, _, err := svc.CreateAPIToken(ctx, app.ID, app.TenantID.String, "n", "d", 30, "admin-1", []string{"read"})
	require.NoError(t, err)
	am := &AuthMiddleware{apiToken: svc}

	for _, tc := range []struct {
		name          string
		requestTenant string
		want          int
	}{
		{"its own tenant", app.TenantID.String, http.StatusOK},
		{"another tenant", "other-tenant", http.StatusForbidden},
		{"the root domain", "", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/things", nil)
			req.Header.Set(XApiKeyHeader, token)
			tokenRouter(am, tc.requestTenant).ServeHTTP(w, req)
			require.Equal(t, tc.want, w.Code, w.Body.String())
		})
	}
}

func TestAGlobalAPITokenOnlyActsOnTheRootDomain(t *testing.T) {
	svc, _, ctx := setupTestAPITokenService(t)
	app, err := svc.CreateClientApplication(ctx, "", "global", "d", "superadmin-1")
	require.NoError(t, err)
	token, _, err := svc.CreateAPIToken(ctx, app.ID, "", "n", "d", 30, "superadmin-1", []string{"read"})
	require.NoError(t, err)
	am := &AuthMiddleware{apiToken: svc}

	for requestTenant, want := range map[string]int{"": http.StatusOK, "acme": http.StatusForbidden} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/things", nil)
		req.Header.Set(XApiKeyHeader, token)
		tokenRouter(am, requestTenant).ServeHTTP(w, req)
		require.Equal(t, want, w.Code, "request tenant %q: %s", requestTenant, w.Body.String())
	}
}

func TestAnAPITokensScopesBoundWhatItMayChange(t *testing.T) {
	svc, _, ctx := setupTestAPITokenService(t)
	app := createTestClientApplication(t, svc)
	newToken := func(scopes ...string) string {
		token, _, err := svc.CreateAPIToken(ctx, app.ID, app.TenantID.String, "n", "d", 30, "admin-1", scopes)
		require.NoError(t, err)
		return token
	}
	readOnly, writer, admin := newToken("read"), newToken("read", "write"), newToken("admin")
	am := &AuthMiddleware{apiToken: svc}

	for _, tc := range []struct {
		name   string
		token  string
		method string
		want   int
	}{
		{"read token reads", readOnly, http.MethodGet, http.StatusOK},
		{"read token writes", readOnly, http.MethodPost, http.StatusForbidden},
		{"read token deletes", readOnly, http.MethodDelete, http.StatusForbidden},
		{"write token writes", writer, http.MethodPost, http.StatusOK},
		{"admin token writes", admin, http.MethodPut, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, "/api/v1/things", nil)
			req.Header.Set(XApiKeyHeader, tc.token)
			tokenRouter(am, app.TenantID.String).ServeHTTP(w, req)
			require.Equal(t, tc.want, w.Code, w.Body.String())
		})
	}
}

func TestASessionIsNotBoundByTokenScopes(t *testing.T) {
	am := &AuthMiddleware{authProvider: sessionProvider{user: &auth.AuthenticatedUser{
		UserID: "u1", TenantID: "t1", Claims: map[string]interface{}{},
	}}}
	w := httptest.NewRecorder()
	tokenRouter(am, "t1").ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/things", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
