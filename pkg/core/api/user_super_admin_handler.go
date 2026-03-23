package core

import (
	"errors"
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	"ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	sharedauth "ctoup.com/coreapp/pkg/shared/auth"
	access "ctoup.com/coreapp/pkg/shared/service"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-One_Of
type UserSuperAdminHandler struct {
	store        *db.Store
	authProvider sharedauth.AuthProvider
	userService  access.UserService
}

func NewUserSuperAdminHandler(store *db.Store, authProvider sharedauth.AuthProvider) *UserSuperAdminHandler {
	factory := access.NewUserServiceStrategyFactory()
	userService := factory.CreateUserServiceStrategy(store, authProvider)

	// Try to initialize user event callback if available
	// This allows the realtime module to set up the callback for user creation events
	if initFunc := access.GetUserEventInitFunc(); initFunc != nil {
		initFunc(userService)
	}

	handler := &UserSuperAdminHandler{store: store,
		authProvider: authProvider,
		userService:  userService}
	return handler
}

// AddUser implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) AddUserFromSuperAdmin(c *gin.Context, tenantId uuid.UUID) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if !auth.IsAllowedToManageTenant(c, tenant) {
		logger.Error().Msg("Not allowed to manage this tenant")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("not allowed to manage this tenant")))
		return
	}
	var req core.AddUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	if err := auth.HasRightsForRoles(c, req.Roles); err != nil {
		logger.Err(err).Msg("User does not have rights for the requested roles")
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForTenant(c, tenant.TenantID)
	if err != nil {
		logger.Err(err).Msg("Failed to get auth client for tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	user, err := uh.userService.CreateUser(c, baseAuthClient, tenant.TenantID, req, nil)
	if err != nil {
		logger.Err(err).Msg("Failed to add user")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	url, err := getResetPasswordURL(c, tenant.Subdomain)
	if err != nil {
		logger.Err(err).Msg("Failed to get reset password URL")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	err = sendWelcomeEmail(c, baseAuthClient, url, req.Email)
	if err != nil {
		logger.Err(err).Msg("Failed to send welcome email")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusCreated, user)
}

// (PUT /api/v1/users/{userid})
func (uh *UserSuperAdminHandler) UpdateUserFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if !auth.IsAllowedToManageTenant(c, tenant) {
		logger.Error().Msg("Not allowed to manage this tenant")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("not allowed to manage this tenant")))
		return
	}
	var req core.UpdateUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	if err := auth.HasRightsForRoles(c, req.Roles); err != nil {
		logger.Err(err).Msg("User does not have rights for the requested roles")
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(err))
		return
	}
	baseAuthClient, err := uh.authProvider.GetAuthClientForTenant(c, tenant.TenantID)
	if err != nil {
		logger.Err(err).Msg("Failed to get auth client for tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	err = uh.userService.UpdateUser(c, baseAuthClient, tenant.TenantID, userid, req)
	if err != nil {
		logger.Err(err).Msg("Failed to update user")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteUser implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) DeleteUserFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if !auth.IsAllowedToManageTenant(c, tenant) {
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("not allowed to manage this tenant")))
		return
	}
	baseAuthClient, err := uh.authProvider.GetAuthClientForTenant(c, tenant.TenantID)
	if err != nil {
		logger.Err(err).Msg("Failed to get auth client for tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	err = uh.userService.RemoveUserFromTenant(c, baseAuthClient, tenant.TenantID, userid)
	if err != nil {
		logger.Err(err).Msg("Failed to remove user from tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// RemoveUserFromTenantFromSuperAdmin removes a user from a specific tenant (deletes membership only)
// (DELETE /superadmin-api/v1/tenants/{tenantid}/users/{userid}/remove-from-tenant)
func (uh *UserSuperAdminHandler) RemoveUserFromTenantFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if !auth.IsAllowedToManageTenant(c, tenant) {
		logger.Error().Msg("Not allowed to manage this tenant")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("not allowed to manage this tenant")))
		return
	}

	// Check if user exists in this tenant
	isMember, err := uh.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
		UserID:   userid,
		TenantID: tenant.TenantID,
	})
	if err != nil || !isMember {
		logger.Err(err).Msg("failed to check user membership")
		c.JSON(http.StatusNotFound, helpers.ErrorResponse(errors.New("user not found in this tenant")))
		return
	}
	baseAuthClient, err := uh.authProvider.GetAuthClientForTenant(c, tenant.TenantID)
	if err != nil {
		logger.Err(err).Msg("Failed to get auth client for tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Remove user from tenant (delete membership)
	err = uh.userService.RemoveUserFromTenant(c, baseAuthClient, tenant.TenantID, userid)
	if err != nil {
		logger.Err(err).Msg("Failed to remove user from tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}

// FindUserByID implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) GetUserByIDFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, id string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if !auth.IsAllowedToManageTenant(c, tenant) {
		logger.Error().Msg("Not allowed to manage this tenant")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("not allowed to manage this tenant")))
		return
	}

	user, err := uh.userService.GetUserByTenantIDByID(c, tenant.TenantID, id)
	if err != nil {
		logger.Err(err).Msg("Failed to get user by ID")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, user)
}

// GetUsers implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) ListUsersFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, params core.ListUsersFromSuperAdminParams) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if !auth.IsAllowedToManageTenant(c, tenant) {
		logger.Error().Msg("Not allowed to manage this tenant")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("not allowed to manage this tenant")))
		return
	}
	pagingRequest := helpers.PagingRequest{
		MaxPageSize:     50,
		DefaultPage:     1,
		DefaultPageSize: 10,
		DefaultSortBy:   "email",
		DefaultOrder:    "asc",
		Page:            params.Page,
		PageSize:        params.PageSize,
		SortBy:          params.SortBy,
		Order:           (*string)(params.Order),
	}
	pagingSql := helpers.GetPagingSQL(pagingRequest)

	like := pgtype.Text{
		Valid: false,
	}

	if params.Q != nil {
		like.String = *params.Q + "%"
		like.Valid = true
	}

	users, err := uh.userService.ListUsers(c, tenant.TenantID, pagingSql, like)
	if err != nil {
		logger.Err(err).Msg("Failed to list users")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, users)
}

// AssignRole implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) AssignRoleFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userID string, role core.Role) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if !auth.IsAllowedToManageTenant(c, tenant) {
		logger.Error().Msg("Not allowed to manage this tenant")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("not allowed to manage this tenant")))
		return
	}
	if auth.IsReseller(c) && auth.GetRoleLevel(string(role)) > auth.GetRoleLevel(string(core.CUSTOMERADMIN)) {
		logger.Error().Msg("Resellers are not allowed to assign roles higher than CUSTOMER_ADMIN")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("resellers are not allowed to assign roles higher than CUSTOMER_ADMIN")))
		return
	}
	baseAuthClient, err := uh.authProvider.GetAuthClientForTenant(c, tenant.TenantID)
	if err != nil {
		logger.Err(err).Msg("Failed to get auth client for tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	err = uh.userService.AssignRole(c, baseAuthClient, tenant.TenantID, userID, role)
	if err != nil {
		logger.Err(err).Msg("Failed to assign role to user")
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

// UnassignRole implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) UnassignRoleFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userID string, role core.Role) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if !auth.IsAllowedToManageTenant(c, tenant) {
		logger.Error().Msg("Not allowed to manage this tenant")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("not allowed to manage this tenant")))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForTenant(c, tenant.TenantID)
	if err != nil {
		logger.Err(err).Msg("Failed to get auth client for tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	err = uh.userService.UnassignRole(c, baseAuthClient, tenant.TenantID, userID, role)
	if err != nil {
		logger.Err(err).Msg("Failed to unassign role from user")
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

// UpdateUserStatus implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) UpdateUserStatusFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userID string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if !auth.IsAllowedToManageTenant(c, tenant) {
		logger.Error().Msg("Not allowed to manage this tenant")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("not allowed to manage this tenant")))
		return
	}
	var req core.UpdateUserStatusJSONBody
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForTenant(c, tenant.TenantID)
	if err != nil {
		logger.Err(err).Msg("Failed to get auth client for tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.UpdateUserStatus(c, baseAuthClient, tenant.TenantID, userID, (string)(req.Name), req.Value)

	if err != nil {
		logger.Err(err).Msg("Failed to update user status")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
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
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if !auth.IsAllowedToManageTenant(c, tenant) {
		logger.Error().Msg("Not allowed to manage this tenant")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("not allowed to manage this tenant")))
		return
	}

	email := string(params.Email)

	// Check if user exists globally (across all tenants)
	user, err := uh.userService.GetUserByEmailGlobal(c, email)
	if err != nil {
		logger.Err(err).Msg("Failed to get user by email globally")
		// User doesn't exist
		c.JSON(http.StatusOK, gin.H{
			"exists": false,
		})
		return
	}

	// Check if user is already a member of the specified tenant
	isMember, err := uh.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
		UserID:   user.Id,
		TenantID: tenant.TenantID,
	})
	if err != nil {
		logger.Err(err).Msg("Failed to check tenant membership")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Count how many tenants the user belongs to
	tenantCount, err := uh.store.CountUserTenants(c, user.Id)
	if err != nil {
		logger.Err(err).Msg("Failed to count user tenants")
		tenantCount = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"exists": true,
		"user": gin.H{
			"id":               user.Id,
			"name":             user.Profile.Name,
			"email":            user.Email,
			"tenantCount":      tenantCount,
			"isMemberOfTenant": isMember,
		},
	})
}

// AddUserMembershipFromSuperAdmin adds an existing user to a specific tenant (Super Admin)
func (uh *UserSuperAdminHandler) AddUserMembershipFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	logger := util.GetLoggerFromCtx(c.Request.Context())
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		logger.Err(err).Msg("Failed to get tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if !auth.IsAllowedToManageTenant(c, tenant) {
		logger.Error().Msg("Not allowed to manage this tenant")
		c.JSON(http.StatusForbidden, helpers.ErrorResponse(errors.New("not allowed to manage this tenant")))
		return
	}

	byUserId, exists := c.Get(auth.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("ByUserID not found"))
		return
	}

	var req core.AddUserMembershipFromSuperAdminJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Msg("Failed to bind JSON")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Check if user already a member
	isMember, err := uh.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
		UserID:   userid,
		TenantID: tenant.TenantID,
	})
	if err != nil {
		logger.Err(err).Msg("Failed to check tenant membership")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	if isMember {
		logger.Error().Str("userID", userid).Str("tenantID", tenant.TenantID).Msg("User is already a member of this tenant")
		c.JSON(http.StatusBadRequest, gin.H{"error": "User is already a member of this tenant"})
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForTenant(c, tenant.TenantID)
	if err != nil {
		logger.Err(err).Msg("Failed to get auth client for tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Add user to tenant (create membership)
	err = uh.userService.AddUserToTenant(c, baseAuthClient, tenant.TenantID, userid, req.Roles, byUserId.(string))
	if err != nil {
		logger.Err(err).Msg("Failed to add user to tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Get updated user info
	user, err := uh.userService.GetUserByTenantIDByID(c, tenant.TenantID, userid)
	if err != nil {
		logger.Err(err).Msg("Failed to get user after adding membership")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Send notification email
	if tenant.Name != "" {
		url, err := buildTenantURL(c, "/", tenant.Subdomain)
		if err != nil {
			logger.Err(err).Msg("Failed to get URL for notification")
			// Don't fail the request if email fails
		} else {
			err = sendTenantAddedEmail(c, baseAuthClient, url, user.Email, tenant.Name)
			if err != nil {
				logger.Err(err).Msg("Failed to send tenant added notification")
				// Don't fail the request if email fails
			}
		}
	}

	c.JSON(http.StatusCreated, user)
}
