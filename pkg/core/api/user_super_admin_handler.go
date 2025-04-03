package core

import (
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"

	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	access "ctoup.com/coreapp/pkg/shared/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-One_Of
type UserSuperAdminHandler struct {
	store          *db.Store
	authClientPool *access.FirebaseTenantClientConnectionPool
	userService    *access.UserService
}

// AddUser implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) AddUserFromSuperAdmin(c *gin.Context, tenantId uuid.UUID) {
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	var req core.AddUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClientForTenant(tenant.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	user, err := uh.userService.AddUser(c, baseAuthClient, tenant.TenantID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusCreated, user)
}

// (PUT /api/v1/users/{userid})
func (uh *UserSuperAdminHandler) UpdateUserFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	var req core.UpdateUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	baseAuthClient, err := uh.authClientPool.GetBaseAuthClientForTenant(tenant.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	err = uh.userService.UpdateUser(c, baseAuthClient, tenant.TenantID, userid, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteUser implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) DeleteUserFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userid string) {
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	baseAuthClient, err := uh.authClientPool.GetBaseAuthClientForTenant(tenant.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	err = uh.userService.DeleteUser(c, baseAuthClient, tenant.TenantID, userid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// FindUserByID implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) GetUserByIDFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, id string) {
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClientForTenant(tenant.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	user, err := uh.userService.GetUserByID(c, baseAuthClient, tenant.TenantID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, user)
}

// GetUsers implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) ListUsersFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, params core.ListUsersFromSuperAdminParams) {
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
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
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, users)
}

// AssignRole implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) AssignRoleFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userID string, roleID uuid.UUID) {
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	baseAuthClient, err := uh.authClientPool.GetBaseAuthClientForTenant(tenant.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	err = uh.userService.AssignRole(c, baseAuthClient, tenant.TenantID, userID, roleID)
	if err != nil {
		log.Printf("error %v\n", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

// UnassignRole implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) UnassignRoleFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userID string, roleID uuid.UUID) {
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClientForTenant(tenant.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	err = uh.userService.UnassignRole(c, baseAuthClient, tenant.TenantID, userID, roleID)
	if err != nil {
		log.Printf("error %v\n", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

// UpdateUserStatus implements openapi.ServerInterface.
func (uh *UserSuperAdminHandler) UpdateUserStatusFromSuperAdmin(c *gin.Context, tenantId uuid.UUID, userID string) {
	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	var req core.UpdateUserStatusJSONBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClientForTenant(tenant.TenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.UpdateUserStatus(c, baseAuthClient, tenant.TenantID, userID, (string)(req.Name), req.Value)

	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (uh *UserHandler) ResetPasswordRequestBySuperAdmin(c *gin.Context, tenantId uuid.UUID, userID string) {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenant, err := uh.store.Queries.GetTenantByID(c, tenantId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	host, err := access.GetHost(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	domain, err := access.GetDomain(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hostUrl := host.Scheme + "://" + tenant.Subdomain + "." + domain

	url := fmt.Sprintf("%s/signin?from=/", hostUrl)

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get Firebase client"})
		return
	}
	resetPasswordRequest(c, baseAuthClient, url, req.Email)
}

func NewUserSuperAdminHandler(store *db.Store, authClientPool *access.FirebaseTenantClientConnectionPool) *UserSuperAdminHandler {
	userService := access.NewUserService(store, authClientPool)
	handler := &UserSuperAdminHandler{store: store,
		authClientPool: authClientPool,
		userService:    userService}
	return handler
}
