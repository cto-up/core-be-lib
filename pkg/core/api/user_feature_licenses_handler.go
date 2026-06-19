package core

import (
	"fmt"
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	api "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// resolveLicenseTenant determines which (user, tenant) membership the caller may
// manage seats for, and returns that tenant's string ID plus its enabled features
// (the entitlement ceiling). Two contexts:
//
//   - Tenant subdomain: AUTH_TENANT_ID is set, so the caller's own tenant is used
//     (tenant admin managing their own users).
//   - Root / super-admin domain: there is no subdomain tenant, so the caller passes
//     tenant_id (UUID) and must be allowed to manage that tenant (super admin or the
//     tenant's reseller).
//
// Returns ok=false when a response has already been written.
func (uh *UserAdminHandler) resolveLicenseTenant(c *gin.Context, userid string, tenantIDParam *openapi_types.UUID) (string, subentity.TenantFeatures, bool) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	if !auth.HasAdminPrivileges(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only RESELLER, CUSTOMER_ADMIN, ADMIN or SUPER_ADMIN can manage user licenses"})
		return "", nil, false
	}

	var tenantID string
	var features subentity.TenantFeatures

	if tenantIDParam != nil {
		// Cross-tenant (root/super-admin): the caller must be allowed to manage it.
		allowed, err := auth.IsAllowedToManageTenantByID(c, uh.store, *tenantIDParam)
		if err != nil {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return "", nil, false
		}
		if !allowed {
			c.JSON(http.StatusForbidden, helpers.ErrorStringResponse("Not allowed to manage this tenant"))
			return "", nil, false
		}
		tenant, err := uh.store.GetTenantByID(c, *tenantIDParam)
		if err != nil {
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return "", nil, false
		}
		tenantID = tenant.TenantID
		features = tenant.Features
	} else {
		// Tenant subdomain: use the caller's tenant context.
		tid, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
		if !exists || tid.(string) == "" {
			c.JSON(http.StatusBadRequest, helpers.ErrorStringResponse("No tenant context — pass tenant_id when managing licenses from the root domain"))
			return "", nil, false
		}
		tenantID = tid.(string)
		tenant, err := uh.store.GetTenantByTenantID(c, tenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return "", nil, false
		}
		features = tenant.Features
	}

	isMember, err := uh.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
		UserID:   userid,
		TenantID: tenantID,
	})
	if err != nil {
		logger.Err(err).Msg("Failed to check tenant membership")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return "", nil, false
	}
	if !isMember {
		c.JSON(http.StatusNotFound, helpers.ErrorStringResponse("user not found in this tenant"))
		return "", nil, false
	}

	return tenantID, features, true
}

// GetUserFeatureLicenses returns a user's per-user feature licenses (seats).
// (GET /api/v1/users/{userid}/feature-licenses)
func (uh *UserAdminHandler) GetUserFeatureLicenses(c *gin.Context, userid string, params api.GetUserFeatureLicensesParams) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	tenantID, _, ok := uh.resolveLicenseTenant(c, userid, params.TenantId)
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

// UpdateUserFeatureLicenses sets a user's per-user feature licenses (seats). Only
// features enabled for the tenant may be assigned — the tenant entitlement is the
// ceiling, so an admin can never grant a feature the tenant lacks.
// (PUT /api/v1/users/{userid}/feature-licenses)
func (uh *UserAdminHandler) UpdateUserFeatureLicenses(c *gin.Context, userid string, params api.UpdateUserFeatureLicensesParams) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	var req subentity.TenantFeatureLicenses
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	tenantID, features, ok := uh.resolveLicenseTenant(c, userid, params.TenantId)
	if !ok {
		return
	}

	for featureName := range req {
		if !features[featureName] {
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
