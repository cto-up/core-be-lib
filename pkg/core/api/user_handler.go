package core

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"

	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	access "ctoup.com/coreapp/pkg/shared/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-One_Of
type UserHandler struct {
	store          *db.Store
	authClientPool *access.FirebaseTenantClientConnectionPool
	userService    *access.UserService
}

// AddUser implements openapi.ServerInterface.
func (uh *UserHandler) AddUser(c *gin.Context) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req core.AddUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	user, err := uh.userService.AddUser(c, baseAuthClient, tenantID.(string), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusCreated, user)
}

// (PUT /api/v1/users/{userid})
func (uh *UserHandler) UpdateUser(c *gin.Context, userid string) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req core.UpdateUserJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.UpdateUser(c, baseAuthClient, tenantID.(string), userid, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteUser implements openapi.ServerInterface.
func (uh *UserHandler) DeleteUser(c *gin.Context, userid string) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.DeleteUser(c, baseAuthClient, tenantID.(string), userid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// FindUserByID implements openapi.ServerInterface.
func (uh *UserHandler) GetUserByID(c *gin.Context, id string) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, err)
		return
	}
	user, err := uh.userService.GetUserByID(c, baseAuthClient, tenantID.(string), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, user)
}

// GetUsers implements openapi.ServerInterface.
func (u *UserHandler) ListUsers(c *gin.Context, params core.ListUsersParams) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
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
func (uh *UserHandler) AssignRole(c *gin.Context, userID string, roleID uuid.UUID) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.AssignRole(c, baseAuthClient, tenantID.(string), userID, roleID)
	if err != nil {
		log.Printf("error %v\n", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

// UnassignRole implements openopenapi.ServerInterface.
func (uh *UserHandler) UnassignRole(c *gin.Context, userID string, roleID uuid.UUID) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	err = uh.userService.UnassignRole(c, baseAuthClient, tenantID.(string), userID, roleID)
	if err != nil {
		log.Printf("error %v\n", err)
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Status(http.StatusNoContent)
}

// UpdateUserStatus implements openopenapi.ServerInterface.
func (uh *UserHandler) UpdateUserStatus(c *gin.Context, userID string) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	var req core.UpdateUserStatusJSONBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	err = uh.userService.UpdateUserStatus(c, baseAuthClient, tenantID.(string), userID, (string)(req.Name), req.Value)

	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (uh *UserHandler) ResetPasswordRequest(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	host, err := access.GetHost(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	url := fmt.Sprintf("%s://%s/signin?from=/", host.Scheme, host.Host)

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get Firebase client"})
		return
	}
	resetPasswordRequest(c, baseAuthClient, url, req.Email)
}

func (uh *UserHandler) ResetPasswordRequestByAdmin(c *gin.Context, userID string) {

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

	user, err := uh.store.GetUserByID(c, repository.GetUserByIDParams{
		ID:       userID,
		TenantID: c.GetString(access.AUTH_TENANT_ID_KEY),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if user.Email.String != req.Email {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email"})
		return
	}

	host, err := access.GetHost(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	url := fmt.Sprintf("%s://%s/signin?from=/", host.Scheme, host.Host)

	baseAuthClient, err := uh.authClientPool.GetBaseAuthClient(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get Firebase client"})
		return
	}
	resetPasswordRequest(c, baseAuthClient, url, req.Email)
}

func NewUserHandler(store *db.Store, authClientPool *access.FirebaseTenantClientConnectionPool) *UserHandler {
	userService := access.NewUserService(store, authClientPool)
	handler := &UserHandler{store: store,
		authClientPool: authClientPool,
		userService:    userService}
	return handler
}
