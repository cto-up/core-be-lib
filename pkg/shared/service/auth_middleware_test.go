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
		{"no tenant context", "", http.StatusForbidden},
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
