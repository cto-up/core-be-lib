package core

import (
	"fmt"
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
)

// requireTenantUserLicenseAdmin resolves the caller's tenant and enforces that
// the caller may manage user licenses (admin privileges) and that the target
// user is an active member of that tenant. Returns the tenant ID, or false when
// a response has already been written.
func (uh *UserAdminHandler) requireTenantUserLicenseAdmin(c *gin.Context, userid string) (string, bool) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists || tenantID.(string) == "" {
		logger.Error().Msg("TenantID not found")
		c.JSON(http.StatusBadRequest, helpers.ErrorStringResponse("TenantID not found"))
		return "", false
	}

	// User licenses are a narrowing of the tenant entitlement, so a tenant admin
	// (CUSTOMER_ADMIN/RESELLER/ADMIN/SUPER_ADMIN) may manage seats for their own
	// tenant. The tenant-ceiling check on write prevents granting beyond it.
	if !auth.HasAdminPrivileges(c) {
		logger.Error().Msg("Only RESELLER, CUSTOMER_ADMIN, ADMIN or SUPER_ADMIN can manage user licenses")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Only RESELLER, CUSTOMER_ADMIN, ADMIN or SUPER_ADMIN can manage user licenses"})
		return "", false
	}

	isMember, err := uh.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
		UserID:   userid,
		TenantID: tenantID.(string),
	})
	if err != nil {
		logger.Err(err).Msg("Failed to check tenant membership")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return "", false
	}
	if !isMember {
		c.JSON(http.StatusNotFound, helpers.ErrorStringResponse("user not found in this tenant"))
		return "", false
	}

	return tenantID.(string), true
}

// GetUserFeatureLicenses returns a user's per-user feature licenses (seats).
// (GET /api/v1/users/{userid}/feature-licenses)
func (uh *UserAdminHandler) GetUserFeatureLicenses(c *gin.Context, userid string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	tenantID, ok := uh.requireTenantUserLicenseAdmin(c, userid)
	if !ok {
		return
	}

	licenses, err := uh.store.GetUserFeatureLicenses(c, repository.GetUserFeatureLicensesParams{
		UserID:   userid,
		TenantID: tenantID,
	})
	if err != nil {
		logger.Err(err).Msg("Failed to get user feature licenses")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if licenses == nil {
		licenses = subentity.TenantFeatureLicenses{}
	}
	c.JSON(http.StatusOK, licenses)
}

// UpdateUserFeatureLicenses sets a user's per-user feature licenses (seats).
// Only features enabled for the tenant may be assigned — the tenant entitlement
// is the ceiling, so a tenant admin can never grant a feature the tenant lacks.
// (PUT /api/v1/users/{userid}/feature-licenses)
func (uh *UserAdminHandler) UpdateUserFeatureLicenses(c *gin.Context, userid string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	var req subentity.TenantFeatureLicenses
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	tenantID, ok := uh.requireTenantUserLicenseAdmin(c, userid)
	if !ok {
		return
	}

	tenant, err := uh.store.GetTenantByTenantID(c, tenantID)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	for featureName := range req {
		if !tenant.Features[featureName] {
			c.JSON(http.StatusBadRequest, helpers.ErrorResponse(
				fmt.Errorf("feature %q is not enabled for this tenant", featureName),
			))
			return
		}
	}

	if _, err := uh.store.UpdateUserFeatureLicenses(c, repository.UpdateUserFeatureLicensesParams{
		UserID:          userid,
		TenantID:        tenantID,
		FeatureLicenses: req,
	}); err != nil {
		logger.Err(err).Msg("Failed to update user feature licenses")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}
