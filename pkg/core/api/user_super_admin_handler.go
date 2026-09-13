package core

import (
	"errors"
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/auth"
	sharedauth "ctoup.com/coreapp/pkg/shared/auth"
	access "ctoup.com/coreapp/pkg/shared/service"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-One_Of
type UserSuperAdminHandler struct {
	// Same thirteen operations as UserAdminHandler, over a named tenant instead
	// of the session's. Both surfaces are adapters over userOps — see
	// user_scope.go for why that seam is where it is.
	userOps
}

func NewUserSuperAdminHandler(store *db.Store, authProvider sharedauth.AuthProvider) *UserSuperAdminHandler {
	factory := access.NewUserServiceStrategyFactory()
	userService := factory.CreateUserServiceStrategy(store, authProvider)

	// Try to initialize user event callback if available
	// This allows the realtime module to set up the callback for user creation events
	if initFunc := access.GetUserEventInitFunc(); initFunc != nil {
		initFunc(userService)
	}

	return &UserSuperAdminHandler{userOps{
		store:        store,
		authProvider: authProvider,
		userService:  userService,
	}}
}

// AddUser implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) AddUserFromSuperAdmin(c *gin.Context, tenantId uuid.UUID) {
	uh.addUser(c, uh.ManagedTenant(tenantId))
}

// (PUT /superadmin-api/v1/tenants/{tenantid}/users/{userid})
func (uh *UserSuperAdminHandler) UpdateUserFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	uh.updateUser(c, uh.ManagedTenant(tenantId), userid)
}

// DeleteUser implements openapi.ServerInterface.
//
// "Delete" here has always meant "remove from this tenant" — actual deletion is
// HardDeleteUserFromSuperAdmin, below. The /admin-api twin means the same thing
// except on the root domain, where there is no tenant to remove a membership
// from; that branch cannot be reached through this route, which always has one.
func (uh *UserSuperAdminHandler) DeleteUserFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	uh.deleteUser(c, uh.ManagedTenant(tenantId), userid)
}

// RemoveUserFromTenantFromSuperAdmin removes a user from a specific tenant (deletes membership only)
// (DELETE /superadmin-api/v1/tenants/{tenantid}/users/{userid}/remove-from-tenant)
func (uh *UserSuperAdminHandler) RemoveUserFromTenantFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	uh.removeUserFromTenant(c, uh.ManagedTenant(tenantId), userid)
}

// FindUserByID implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) GetUserByIDFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, id string) {
	uh.getUserByID(c, uh.ManagedTenant(tenantId), id)
}

// GetUsers implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) ListUsersFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, params core.ListUsersFromSuperAdminParams) {
	// Scope and Detail are absent by design: this surface never lists across
	// tenants (the tenant IS the path) and its one frontend wants whole users.
	uh.listUsers(c, uh.ManagedTenant(tenantId), listParams{
		Page:     params.Page,
		PageSize: params.PageSize,
		SortBy:   params.SortBy,
		Order:    (*string)(params.Order),
		Q:        params.Q,
	})
}

// AssignRole implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) AssignRoleFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userID string, role core.Role) {
	uh.assignRole(c, uh.ManagedTenant(tenantId), userID, role)
}

// UnassignRole implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) UnassignRoleFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userID string, role core.Role) {
	uh.unassignRole(c, uh.ManagedTenant(tenantId), userID, role)
}

// UpdateUserStatus implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) UpdateUserStatusFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userID string) {
	uh.updateUserStatus(c, uh.ManagedTenant(tenantId), userID)
}

func (uh *UserHandler) ResetPasswordRequestBySuperAdmin(c *gin.Context, tenantId uuid.UUID, userID string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	var req struct {
		Email string `json:"email"`
	}
	if err := c.BindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// We need to check authorization here too. But uh is UserHandler or UserSuperAdminHandler?
	// The receiver in ResetPasswordRequestBySuperAdmin is UserHandler!
	// Wait, I should probably check it anyway.
	if auth.IsReseller(c) {
		authTenantID := c.GetString(auth.AUTH_TENANT_ID_KEY)
		if !tenant.ResellerID.Valid || tenant.ResellerID.String != authTenantID {
			c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("not allowed to manage this tenant")))
			return
		}
	} else if !auth.IsSuperAdmin(c) {
		logger.Error().Msg("Not allowed to perform this operation")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("not allowed to perform this operation")))
		return
	}

	// Use the reusable buildTenantURL function
	url, err := buildTenantURL(c, "/signin?from=/", tenant.Subdomain)
	if err != nil {
		logger.Err(err).Msg("Failed to build tenant URL")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForSubdomain(c, tenant.Subdomain)
	if err != nil {
		logger.Err(err).Msg("Failed to get auth client for tenant")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get auth client"})
		return
	}
	err = resetPasswordRequest(c, baseAuthClient, url, req.Email)
	if err != nil {
		logger.Err(err).Msg("Failed to send password reset email")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Password reset email sent"})
}

// CheckUserExistsFromSuperAdmin checks if a user exists globally by email (Super Admin)
func (uh *UserSuperAdminHandler) CheckUserExistsFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, params core.CheckUserExistsFromSuperAdminParams) {
	uh.checkUserExists(c, uh.ManagedTenant(tenantId), string(params.Email))
}

// AddUserMembershipFromSuperAdmin adds an existing user to a specific tenant (Super Admin)
func (uh *UserSuperAdminHandler) AddUserMembershipFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	var req core.AddUserMembershipFromSuperAdminJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		logger := util.GetLoggerFromCtx(c.Request.Context())
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	uh.addUserMembership(c, uh.ManagedTenant(tenantId), userid, req.Roles)
}

// ReactivateUserFromSuperAdmin sets a user's membership status to 'active' for a specific tenant.
func (uh *UserSuperAdminHandler) ReactivateUserFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	uh.reactivateUser(c, uh.ManagedTenant(tenantId), userid)
}

// HardDeleteUserFromSuperAdmin permanently deletes a user from the database and the identity provider.
// Unlike DeleteUserFromSuperAdmin (which only removes the tenant membership), this removes the user globally.
// Blocked if the user has active memberships in other tenants — deactivate those first.
// (DELETE /superadmin-api/v1/tenants/{tenantid}/users/{userid}/hard-delete)
func (uh *UserSuperAdminHandler) HardDeleteUserFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())

	callerID := c.GetString(sharedauth.AUTH_USER_ID)
	if callerID == "" {
		c.JSON(http.StatusInternalServerError, helpers.ErrorStringResponse("user_id not found in context"))
		return
	}
	// This operation has no twin, so it keeps its own body — but it uses the
	// same guards as the thirteen that do. It is where the self-refusal already
	// lived before the fork was noticed.
	if !uh.refuseSelf(c, userid, "Cannot permanently delete yourself") {
		return
	}

	target, ok := resolve(c, uh.ManagedTenant(tenantId))
	if !ok {
		return
	}
	if !uh.refuseIfTargetOutranks(c, target.id, userid) {
		return
	}

	// Block if the user has active memberships in other tenants.
	// Inactive memberships are fine — they will cascade-delete.
	if err := access.CheckUserNotActiveInOtherTenants(c, uh.store, userid, target.id); err != nil {
		var blocked *access.ErrUserActiveInOtherTenants
		if errors.As(err, &blocked) {
			c.JSON(http.StatusConflict, helpers.ErrorStringResponse(blocked.Error()))
			return
		}
		logger.Err(err).Msg("Failed to check active tenant memberships")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	logger.Warn().
		Str("user_id", userid).
		Str("tenant_id", target.id).
		Str("caller_id", callerID).
		Msg("SUPER_ADMIN hard deleting user — irreversible")

	baseAuthClient, ok := target.client(c)
	if !ok {
		return
	}

	if err := uh.userService.DeleteUser(c, baseAuthClient, userid); err != nil {
		if err.Error() == "no rows in result set" {
			// No core_users record — user is orphaned (exists in Kratos/memberships only).
			// Still remove the Kratos identity so the account cannot be used.
			logger.Warn().Str("user_id", userid).Msg("No core_users record found — cleaning up Kratos identity only")
			if kratosErr := baseAuthClient.DeleteUser(c, userid); kratosErr != nil && !sharedauth.IsUserNotFound(kratosErr) {
				logger.Err(kratosErr).Msg("Failed to delete orphaned Kratos identity")
				c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(kratosErr))
				return
			}
			c.Status(http.StatusNoContent)
			return
		}
		if helpers.AbortIfReferenced(c, err,
			"USER_IN_USE",
			"user is referenced by other records and cannot be deleted") {
			return
		}
		logger.Err(err).Msg("Failed to hard delete user")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}
