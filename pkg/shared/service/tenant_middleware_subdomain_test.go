package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	utils "ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The tenant middleware resolves a tenant from the request subdomain, and
// aborts with 404 when none matches. Infrastructure subdomains must skip that
// lookup: they name a vhost, not a tenant.
//
// This is only reachable for requests with NO Origin header, because GetHost
// prefers Origin over Host — which is precisely the shape of an inbound request
// from an external system. A provider redirecting a browser to an OAuth
// callback performs a cross-site top-level navigation, which sends no Origin;
// so does a webhook POST. Both used to be rejected as "Tenant not found" before
// their handler ran, despite each carrying its own tenant context (the OAuth
// state row, the webhook id).
//
// The assertion is on the subdomain classification rather than the middleware
// itself so it needs no database.
func TestInfrastructureSubdomainsAreTenantless(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Mirrors the condition in MiddlewareFunc.
	tenantless := func(subdomain string) bool {
		return utils.IsAdminSubdomain(subdomain) || subdomain == "auth" || subdomain == "api"
	}

	for _, tc := range []struct {
		host string
		want bool
		why  string
	}{
		{"api.example.com", true, "OAuth callbacks and webhooks land here with no Origin"},
		{"auth.example.com", true, "Kratos"},
		{"admin.example.com", true, "admin console"},
		{"www.example.com", true, "apex"},
		{"example.com", true, "apex"},
		{"acme.example.com", false, "a real tenant must still resolve"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/public-api/v1/aiemployee/oauth/callback", nil)
		req.Host = tc.host // deliberately no Origin header

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = req

		sub, err := utils.GetSubdomain(c)
		require.NoError(t, err, tc.host)
		require.Equal(t, tc.want, tenantless(sub),
			"%s (subdomain %q): %s", tc.host, sub, tc.why)
	}
}
