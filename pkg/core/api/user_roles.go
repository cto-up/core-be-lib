package core

import (
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
)

// The role and status operations, written once. See user_scope.go for the seam.

// assignRole grants a role in the scoped tenant.
//
// The reseller cap is applied on BOTH paths now, where it lived only on the
// super-admin one. That is not a new restriction — SharedUserService.AssignRole
// already refuses ADMIN and SUPER_ADMIN as global-only through
// validateTenantScopedRole, so the cap has always been enforced somewhere. What
// changes is that the refusal is now the same refusal, with the same status and
// the same message, whichever door the reseller came through.
func (o *userOps) assignRole(c *gin.Context, scope TenantScope, userID string, role core.Role) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	target, ok := resolve(c, scope)
	if !ok {
		return
	}
	if auth.IsReseller(c) && auth.GetRoleLevel(string(role)) > auth.GetRoleLevel(string(core.CUSTOMERADMIN)) {
		logger.Error().Msg("Resellers are not allowed to assign roles higher than CUSTOMER_ADMIN")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errResellerRoleCap))
		return
	}

	client, ok := target.client(c)
	if !ok {
		return
	}
	if err := o.userService.AssignRole(c, client, target.id, userID, role); err != nil {
		logger.Err(err).Msg("Failed to assign role to user")
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

func (o *userOps) unassignRole(c *gin.Context, scope TenantScope, userID string, role core.Role) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	target, ok := resolve(c, scope)
	if !ok {
		return
	}
	client, ok := target.client(c)
	if !ok {
		return
	}
	if err := o.userService.UnassignRole(c, client, target.id, userID, role); err != nil {
		logger.Err(err).Msg("Failed to unassign role")
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

func (o *userOps) updateUserStatus(c *gin.Context, scope TenantScope, userID string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	target, ok := resolve(c, scope)
	if !ok {
		return
	}

	var req core.UpdateUserStatusJSONBody
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	client, ok := target.client(c)
	if !ok {
		return
	}
	if err := o.userService.UpdateUserStatus(c, client, target.id, userID, string(req.Name), req.Value); err != nil {
		logger.Err(err).Msg("Failed to update user status")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (o *userOps) reactivateUser(c *gin.Context, scope TenantScope, userID string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	target, ok := resolve(c, scope)
	if !ok {
		return
	}
	err := o.store.ReactivateUserMembership(c, repository.ReactivateUserMembershipParams{
		UserID:   userID,
		TenantID: target.id,
	})
	if err != nil {
		logger.Err(err).Msg("Failed to reactivate user membership")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}
