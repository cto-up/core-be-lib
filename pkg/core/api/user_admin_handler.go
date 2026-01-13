package core

import (
	"encoding/csv"
	"io"
	"strings"
	"time"

	"errors"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"

	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	auth "ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/event"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	access "ctoup.com/coreapp/pkg/shared/service"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

// https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-One_Of
type UserAdminHandler struct {
	store        *db.Store
	authProvider auth.AuthProvider
	userService  access.UserService
}

func NewUserAdminHandler(store *db.Store, authProvider auth.AuthProvider) *UserAdminHandler {

	factory := access.NewUserServiceStrategyFactory()
	userService := factory.CreateUserServiceStrategy(store, authProvider)

	// Try to initialize user event callback if available
	// This allows the realtime module to set up the callback for user creation events
	if initFunc := access.GetUserEventInitFunc(); initFunc != nil {
		initFunc(userService)
	}

	handler := &UserAdminHandler{store: store,
		authProvider: authProvider,
		userService:  userService}
	return handler
}

// AddUser implements openapi.ServerInterface.
func (uh *UserAdminHandler) AddUser(c *gin.Context) {

	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req core.AddUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	if err := access.HasRightsForRoles(c, req.Roles); err != nil {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(err))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForSubdomain(c, subdomain)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get auth client for subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	user, err := uh.userService.CreateUser(c, baseAuthClient, tenantID.(string), req, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to add user")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	url, err := getResetPasswordURL(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get reset password URL")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	err = sendWelcomeEmail(c, baseAuthClient, url, req.Email)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send welcome email")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusCreated, user)
}

// (PUT /api/v1/users/{userid})
func (uh *UserAdminHandler) UpdateUser(c *gin.Context, userid string) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req core.UpdateUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	if err := access.HasRightsForRoles(c, req.Roles); err != nil {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(err))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForSubdomain(c, subdomain)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get auth client for subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.UpdateUser(c, baseAuthClient, tenantID.(string), userid, req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update user")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteUser implements openapi.ServerInterface.
func (uh *UserAdminHandler) DeleteUser(c *gin.Context, userid string) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	// check if user is deleting self
	if userid == c.GetString(auth.AUTH_USER_ID) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Cannot delete self"})
		return
	}
	// check if user has rights to delete user CUSTOMER_ADMIN, ADMIN, SUPER_ADMIN
	if !access.IsCustomerAdmin(c) && !access.IsAdmin(c) && !access.IsSuperAdmin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Only CUSTOMER_ADMIN, ADMIN or SUPER_ADMIN can delete user"})
		return
	}
	var user core.User
	var err error

	if tenantID == "" {
		if !access.IsSuperAdmin(c) {
			log.Error().Msg("Only SUPER_ADMIN can delete user without tenant")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Only SUPER_ADMIN can delete user without tenant"})
			return
		}
		user, err = uh.userService.GetUserByID(c, userid)
		if err != nil {
			log.Error().Err(err).Msg("failed to get user by ID")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}
	} else {

		user, err = uh.userService.GetUserByTenantIDByID(c, tenantID.(string), userid)
		if err != nil {
			log.Error().Err(err).Msg("failed to get user by ID")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}
	}

	err = access.HasRightsForRoles(c, user.Roles)
	if err != nil {
		log.Error().Err(err).Msg("user does not have rights to be deleted")
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(err))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForSubdomain(c, subdomain)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get auth client for subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.DeleteUser(c, baseAuthClient, tenantID.(string), userid)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete user")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// RemoveUserFromTenant removes a user from the current tenant (deletes membership only)
// (DELETE /api/v1/users/{userid}/remove-from-tenant)
func (uh *UserAdminHandler) RemoveUserFromTenant(c *gin.Context, userid string) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	// Check if user is removing self
	if userid == c.GetString(auth.AUTH_USER_ID) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Cannot remove self from tenant"})
		return
	}

	// Check if user has rights to remove user (CUSTOMER_ADMIN, ADMIN, SUPER_ADMIN)
	if !access.IsCustomerAdmin(c) && !access.IsAdmin(c) && !access.IsSuperAdmin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Only CUSTOMER_ADMIN, ADMIN or SUPER_ADMIN can remove user from tenant"})
		return
	}

	// Check if user exists and get their roles
	// First check if user has membership in this tenant
	isMember, err := uh.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
		UserID:   userid,
		TenantID: tenantID.(string),
	})
	if err != nil || !isMember {
		log.Error().Err(err).Msg("failed to check user membership")
		c.JSON(http.StatusNotFound, helpers.ErrorResponse(errors.New("user not found in this tenant")))
		return
	}

	// Get user roles from membership
	roles, err := uh.store.GetUserTenantRoles(c, repository.GetUserTenantRolesParams{
		UserID:   userid,
		TenantID: tenantID.(string),
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to get user roles")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	result := make([]core.Role, len(roles))
	for i, r := range roles {
		result[i] = core.Role(r)
	}

	err = access.HasRightsForRoles(c, result)

	if err != nil {
		log.Error().Err(err).Msg("user does not have rights to be removed from tenant")
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(err))
		return
	}

	// Remove user from tenant (delete membership)
	err = uh.userService.RemoveUserFromTenant(c, uh.authProvider.GetAuthClient(), tenantID.(string), userid)
	if err != nil {
		log.Error().Err(err).Msg("Failed to remove user from tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}

// GetUserByID implements openapi.ServerInterface.
func (uh *UserAdminHandler) GetUserByID(c *gin.Context, id string) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	// in case root domain is used
	if tenantID == "" {
		if !access.IsSuperAdmin(c) {
			log.Error().Msg("Only SUPER_ADMIN can get user without tenant")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Only SUPER_ADMIN can get user without tenant"})
			return
		}

		user, err := uh.userService.GetUserByID(c, id)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get user by ID")
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}
		c.JSON(http.StatusOK, user)
		return
	}

	user, err := uh.userService.GetUserByTenantIDByID(c, tenantID.(string), id)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user by ID")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, user)
}

// GetUsers implements openapi.ServerInterface.
func (u *UserAdminHandler) ListUsers(c *gin.Context, params core.ListUsersParams) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
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

	users, err := u.userService.ListUsers(c, tenantID.(string), pagingSql, like)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list users")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if params.Detail != nil && *params.Detail == "basic" {
		basicEntities := make([]subentity.BasicEntity, 0)
		for _, user := range users {
			basicEntity := subentity.BasicEntity{
				ID:   user.Id,
				Name: user.Profile.Name,
			}
			basicEntities = append(basicEntities, basicEntity)
		}
		c.JSON(http.StatusOK, basicEntities)
	} else {
		c.JSON(http.StatusOK, users)
	}
}

// AssignRole implements openopenapi.ServerInterface.
func (uh *UserAdminHandler) AssignRole(c *gin.Context, userID string, role core.Role) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForSubdomain(c, subdomain)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get auth client for subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.AssignRole(c, baseAuthClient, tenantID.(string), userID, role)
	if err != nil {
		log.Printf("error %v\n", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

// UnassignRole implements openopenapi.ServerInterface.
func (uh *UserAdminHandler) UnassignRole(c *gin.Context, userID string, role core.Role) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForSubdomain(c, subdomain)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get auth client for subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.UnassignRole(c, baseAuthClient, tenantID.(string), userID, role)
	if err != nil {
		log.Printf("error %v\n", err)
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Status(http.StatusNoContent)
}

// UpdateUserStatus implements openopenapi.ServerInterface.
func (uh *UserAdminHandler) UpdateUserStatus(c *gin.Context, userID string) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	var req core.UpdateUserStatusJSONBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForSubdomain(c, subdomain)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get auth client for subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.UpdateUserStatus(c, baseAuthClient, tenantID.(string), userID, (string)(req.Name), req.Value)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update user status")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}

func (uh *UserAdminHandler) ResetPasswordRequestByAdmin(c *gin.Context, userID string) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// check if authorized user is admin
	if !access.IsAdmin(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Only admin can reset password"})
		return
	}

	user, err := uh.userService.GetUserByTenantIDByID(c, tenantID.(string), userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user by ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if user.Email != req.Email {
		log.Error().Msg("Email does not match user ID")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email"})
		return
	}

	url, err := getResetPasswordURL(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get reset password URL")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForSubdomain(c, subdomain)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get auth client for subdomain")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get auth client"})
		return
	}

	err = resetPasswordRequest(c, baseAuthClient, url, req.Email)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send password reset email")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password reset email sent"})
}

// CheckUserExists checks if a user exists globally by email
func (uh *UserAdminHandler) CheckUserExists(c *gin.Context, params core.CheckUserExistsParams) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	email := string(params.Email)

	// Check if user exists globally (across all tenants)
	user, err := uh.userService.GetUserByEmailGlobal(c, email)
	if err != nil {
		// User doesn't exist
		c.JSON(http.StatusOK, gin.H{
			"exists": false,
		})
		return
	}

	// Check if user is already a member of current tenant
	isMember, err := uh.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
		UserID:   user.Id,
		TenantID: tenantID.(string),
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to check tenant membership")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Count how many tenants the user belongs to
	tenantCount, err := uh.store.CountUserTenants(c, user.Id)
	if err != nil {
		log.Error().Err(err).Msg("Failed to count user tenants")
		tenantCount = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"exists": true,
		"user": gin.H{
			"id":                      user.Id,
			"name":                    user.Profile.Name,
			"email":                   user.Email,
			"tenantCount":             tenantCount,
			"isMemberOfCurrentTenant": isMember,
		},
	})
}

// AddUserMembership adds an existing user to the current tenant
func (uh *UserAdminHandler) AddUserMembership(c *gin.Context, userid string) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	byUserID, exists := c.Get(auth.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("ByUserID not found"))
		return
	}

	var req core.AddUserMembershipJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Check authorization for roles
	if err := access.HasRightsForRoles(c, req.Roles); err != nil {
		c.JSON(http.StatusUnauthorized, helpers.ErrorResponse(err))
		return
	}

	// Check if user already a member
	isMember, err := uh.store.IsUserMemberOfTenant(c, repository.IsUserMemberOfTenantParams{
		UserID:   userid,
		TenantID: tenantID.(string),
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to check tenant membership")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	if isMember {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User is already a member of this tenant"})
		return
	}

	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForSubdomain(c, subdomain)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get auth client for subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Add user to tenant (create membership)
	err = uh.userService.AddUserToTenant(c, baseAuthClient, tenantID.(string), userid, req.Roles, byUserID.(string))
	if err != nil {
		log.Error().Err(err).Msg("Failed to add user to tenant")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Get updated user info
	user, err := uh.userService.GetUserByTenantIDByID(c, tenantID.(string), userid)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user after adding membership")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Send notification email
	userEmail := user.Email
	url, err := getResetPasswordURL(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get URL for notification")
		// Don't fail the request if email fails
	} else {
		err = sendTenantAddedEmail(c, baseAuthClient, url, userEmail, subdomain)
		if err != nil {
			log.Error().Err(err).Msg("Failed to send tenant added notification")
			// Don't fail the request if email fails
		}
	}

	c.JSON(http.StatusCreated, user)
}

func (uh *UserAdminHandler) ImportUsersFromAdmin(c *gin.Context) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	// Get Firebase auth client for tenant
	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get subdomain")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authProvider.GetAuthClientForSubdomain(c, subdomain)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get auth client for subdomain")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Get file from form
	file, err := c.FormFile("file")
	if err != nil {
		log.Error().Err(err).Msg("Failed to get uploaded file")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(fmt.Errorf("file upload error: %v", err)))
		return
	}

	// Open the file
	src, err := file.Open()
	if err != nil {
		log.Error().Err(err).Msg("Failed to open uploaded file")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(fmt.Errorf("error opening file: %v", err)))
		return
	}
	defer src.Close()

	// Parse CSV
	reader := csv.NewReader(src)
	reader.Comma = ';' // Set semicolon as delimiter

	// Read header
	header, err := reader.Read()
	if err != nil {
		log.Error().Err(err).Msg("Failed to read CSV header")
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(fmt.Errorf("error reading CSV header: %v", err)))
		return
	}

	// Strip BOM from the first header column if present
	if len(header) > 0 {
		header[0] = util.StripBOM(header[0])
	}

	// Validate header
	requiredColumns := []string{"lastname", "firstname", "email", "is_customer_admin"}
	missingColumns := []string{}

	// Create a map of header columns for easy lookup
	headerMap := make(map[string]int)
	for i, col := range header {
		headerMap[strings.ToLower(col)] = i
	}

	// Check for missing required columns
	for _, required := range requiredColumns {
		if _, exists := headerMap[required]; !exists {
			missingColumns = append(missingColumns, required)
		}
	}

	if len(missingColumns) > 0 {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(fmt.Errorf("invalid CSV format. Missing required columns: %v", missingColumns)))
		return
	}

	// Process records
	type ImportError struct {
		Line  int    `json:"line"`
		Email string `json:"email"`
		Error string `json:"error"`
	}

	var (
		total         int
		success       int
		alreadyExists int
		failed        int
		errors        []ImportError
	)

	// Handle streaming case
	clientChan := make(chan event.ProgressEvent)
	errorChan := make(chan error, 1)

	// Set headers for SSE before any data is written
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	// Process each line
	lineNum := 1 // Start from 1 to account for header
	// get total number of lines
	// Start generation in goroutine
	go func() {
		defer close(clientChan)

		for {
			lineNum++
			message := fmt.Sprintf(`Processing 
			line: %d,
			success: %d,
			already exists: %d,
			failed: %d,
			errors: %v`, lineNum, success, alreadyExists, failed, errors)
			clientChan <- event.NewProgressEvent("INFO", message, 50)

			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				errors = append(errors, ImportError{
					Line:  lineNum,
					Error: fmt.Sprintf("error reading line: %v", err),
				})
				failed++
				continue
			}

			total++

			// Extract user data
			if len(record) < 4 {
				errors = append(errors, ImportError{
					Line:  lineNum,
					Error: fmt.Sprintf("invalid record format, expected at least 4 fields, got %d", len(record)),
				})
				failed++
				continue
			}

			lastname := record[headerMap["lastname"]]
			firstname := record[headerMap["firstname"]]
			email := record[headerMap["email"]]
			isCustomerAdminStr := strings.ToLower(record[headerMap["is_customer_admin"]])

			// Parse is_customer_admin value
			isCustomerAdmin := false
			if isCustomerAdminStr == "y" || isCustomerAdminStr == "yes" || isCustomerAdminStr == "Y" || isCustomerAdminStr == "YES" || isCustomerAdminStr == "Yes" {
				isCustomerAdmin = true
			}

			var req core.AddUserJSONRequestBody
			req.Email = email
			req.Name = firstname + " " + lastname

			// check if user has rights to assign roles
			if isCustomerAdmin && (!access.IsSuperAdmin(c) && !access.IsAdmin(c) && !access.IsCustomerAdmin(c)) {
				errors = append(errors, ImportError{
					Line:  lineNum,
					Email: email,
					Error: "must be an CUSTOMER_ADMIN or SUPER_ADMIN to assign CUSTOMER_ADMIN role to a user.",
				})
				failed++
				continue
			}
			if isCustomerAdmin {
				req.Roles = []core.Role{"CUSTOMER_ADMIN"}
			}
			_, err = uh.userService.CreateUser(c, baseAuthClient, tenantID.(string), req, nil)
			if err != nil {
				// check if error is a firebase error and if so, check if it is a duplicate email error
				if auth.IsEmailAlreadyExists(err) {
					errors = append(errors, ImportError{
						Line:  lineNum,
						Email: email,
						Error: "email already exists",
					})
					alreadyExists++
					continue
				} else {
					errors = append(errors, ImportError{
						Line:  lineNum,
						Email: email,
						Error: fmt.Sprintf("error creating user: %v", err),
					})
					failed++
					continue
				}
			}

			url, err := getResetPasswordURL(c)
			if err != nil {
				errors = append(errors, ImportError{
					Line:  lineNum,
					Email: email,
					Error: fmt.Sprintf("error getting reset password url: %v", err),
				})
				failed++
				continue
			}
			err = sendWelcomeEmail(c, baseAuthClient, url, req.Email)
			if err != nil {
				errors = append(errors, ImportError{
					Line:  lineNum,
					Email: email,
					Error: fmt.Sprintf("error sending welcome email: %v", err),
				})
				failed++
				continue
			}

			success++
		}

		// Return results
		result := fmt.Sprintf(`Finished processing Users. Results:
			total: %d,
			success: %d,
			already exists: %d,
			failed: %d,
			errors: %v`,
			total, success, alreadyExists, failed, errors)

		clientChan <- event.NewProgressEvent("INFO", result, 100)
	}()

	c.Stream(func(w io.Writer) bool {
		select {
		case msg, ok := <-clientChan:
			if !ok {
				return false
			}
			c.SSEvent("message", msg)
			return msg.EventType != "ERROR" && msg.Progress != 100
		case err := <-errorChan:
			// Send error as SSE event instead of trying to change status code
			log.Printf("Error in streaming: %v", err)
			errEvent := event.NewProgressEvent("ERROR", err.Error(), 100)
			c.SSEvent("message", errEvent)
			return false
		case <-time.After(60 * time.Second):
			// Send timeout as SSE event
			timeoutEvent := event.NewProgressEvent("ERROR", "Generation timeout", 100)
			c.SSEvent("message", timeoutEvent)
			return false
		}
	})
	// Commit transaction if there were successful imports
}
