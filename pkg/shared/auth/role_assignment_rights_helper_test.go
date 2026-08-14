package auth

import (
	"net/http/httptest"
	"testing"

	"ctoup.com/coreapp/api/openapi/core"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func contextWithClaims(claims map[string]interface{}) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(AUTH_CLAIMS, claims)
	return c
}

func TestHasRightsForRole(t *testing.T) {
	customerAdmin := map[string]interface{}{string(core.CUSTOMERADMIN): true}
	// What the Kratos provider actually issues in a managed tenant: the inherited
	// CUSTOMER_ADMIN plus ACTING_RESELLER to mark it as inherited.
	actingResellerLive := map[string]interface{}{
		string(core.CUSTOMERADMIN): true,
		ACTING_RESELLER:            true,
	}
	// ACTING_RESELLER on its own must still carry admin rights — the two claims
	// are granted together today, but nothing here should depend on that.
	actingReseller := map[string]interface{}{ACTING_RESELLER: true}
	plainUser := map[string]interface{}{string(core.USER): true}
	admin := map[string]interface{}{string(core.ADMIN): true}
	superAdmin := map[string]interface{}{string(core.SUPERADMIN): true}

	tests := []struct {
		name    string
		claims  map[string]interface{}
		role    core.Role
		allowed bool
	}{
		{"acting reseller may appoint a customer admin", actingResellerLive, core.CUSTOMERADMIN, true},
		{"acting reseller may not grant ADMIN despite inherited role", actingResellerLive, core.ADMIN, false},
		{"ACTING_RESELLER alone still appoints a customer admin", actingReseller, core.CUSTOMERADMIN, true},
		{"acting reseller may add a plain user", actingReseller, core.USER, true},
		{"acting reseller may not grant ADMIN", actingReseller, core.ADMIN, false},
		{"acting reseller may not grant SUPER_ADMIN", actingReseller, core.SUPERADMIN, false},
		{"customer admin may appoint a customer admin", customerAdmin, core.CUSTOMERADMIN, true},
		{"customer admin may not grant ADMIN", customerAdmin, core.ADMIN, false},
		{"plain user may not appoint a customer admin", plainUser, core.CUSTOMERADMIN, false},
		{"admin may grant ADMIN", admin, core.ADMIN, true},
		{"admin may not grant SUPER_ADMIN", admin, core.SUPERADMIN, false},
		{"super admin may grant SUPER_ADMIN", superAdmin, core.SUPERADMIN, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := HasRightsForRole(contextWithClaims(tt.claims), tt.role)
			if tt.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestHasRightsForRoleWithoutClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	require.Error(t, HasRightsForRole(c, core.CUSTOMERADMIN))
	require.NoError(t, HasRightsForRole(c, core.USER))
}
