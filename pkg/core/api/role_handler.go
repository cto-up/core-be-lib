package core

import (
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	access "ctoup.com/coreapp/pkg/shared/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-One_Of
type RoleHandler struct {
	store *db.Store
}

// AddRole implements openapi.ServerInterface.
func (rh *RoleHandler) AddRole(c *gin.Context) {
	if !access.IsSuperAdmin(c) {
		c.JSON(http.StatusForbidden, "Need to be SUPER_ADMIN")
		return
	}

	var req core.AddRoleJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	userID, exist := c.Get(access.AUTH_USER_ID)
	if !exist {
		c.JSON(http.StatusBadRequest, "Need to be authenticated")
		return
	}

	role, err := rh.store.CreateRole(c, repository.CreateRoleParams{
		Name:   req.Name,
		UserID: userID.(string),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusCreated, role)
}

// UpdateRole implements openapi.ServerInterface.
func (rh *RoleHandler) UpdateRole(c *gin.Context, id uuid.UUID) {
	if !access.IsSuperAdmin(c) {
		c.JSON(http.StatusForbidden, "Need to be SUPER_ADMIN")
		return
	}
	var req core.UpdateRoleJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	if req.Id != id {
		c.JSON(http.StatusBadRequest, "ID mismatch")
		return
	}
	_, err := rh.store.UpdateRole(c, repository.UpdateRoleParams{
		ID:   id,
		Name: req.Name,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}

// DeleteRole implements openapi.ServerInterface.
func (rh *RoleHandler) DeleteRole(c *gin.Context, id uuid.UUID) {
	if !access.IsSuperAdmin(c) {
		c.JSON(http.StatusForbidden, "Need to be SUPER_ADMIN")
		return
	}
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.Status(http.StatusForbidden)
		return
	}
	_, err := rh.store.DeleteRole(c, repository.DeleteRoleParams{
		ID:     id,
		UserID: userID.(string),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// GetRoleByID implements openapi.ServerInterface.
func (rh *RoleHandler) GetRoleByID(c *gin.Context, id uuid.UUID) {
	role, err := rh.store.GetRoleByID(c, id)
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, role)
}

// FindRoles implements openapi.ServerInterface.
func (rh *RoleHandler) ListRoles(c *gin.Context, params core.ListRolesParams) {

	pagingRequest := helpers.PagingRequest{
		MaxPageSize:     50,
		DefaultPage:     1,
		DefaultPageSize: 10,
		DefaultSortBy:   "name",
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

	roles, err := rh.store.ListRoles(c, repository.ListRolesParams{
		Limit:  pagingSql.PageSize,
		Offset: pagingSql.Offset,
		Like:   like,
	})

	if !access.IsAdmin(c) && !access.IsSuperAdmin(c) {
		// remove "ADMIN" from roles
		for i, role := range roles {
			if role.Name == "ADMIN" {
				roles = append(roles[:i], roles[i+1:]...)
			}
		}
	}
	if !access.IsSuperAdmin(c) {
		// remove "SUPER_ADMIN" from roles
		for i, role := range roles {
			if role.Name == "SUPER_ADMIN" {
				roles = append(roles[:i], roles[i+1:]...)
			}
		}
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, roles)
}

func NewRoleHandler(store *db.Store) *RoleHandler {
	handler := &RoleHandler{store: store}
	return handler
}
