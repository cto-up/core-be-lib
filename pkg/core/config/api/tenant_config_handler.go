package config

import (
	"errors"
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

// https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-One_Of
type TenantConfigHandler struct {
	authProvider auth.AuthProvider
	store        *db.Store
}

// AddTenantConfig implements openapi.ServerInterface.
func (exh *TenantConfigHandler) AddTenantConfig(c *gin.Context) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req core.AddTenantConfigJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	userID, exist := c.Get(auth.AUTH_USER_ID)
	if !exist {
		// should not happen as the middleware ensures that the user is authenticated
		c.JSON(http.StatusBadRequest, "Need to be authenticated")
		return
	}
	tenantConfig, err := exh.store.CreateTenantConfig(c,
		repository.CreateTenantConfigParams{
			UserID:   userID.(string),
			Name:     req.Name,
			Value:    util.ToNullableText(req.Value),
			TenantID: tenantID.(string),
		})
	if err != nil {
		log.Error().Err(err).Msg("Error creating tenant config")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusCreated, tenantConfig)
}

// UpdateTenantConfig implements openapi.ServerInterface.
func (exh *TenantConfigHandler) UpdateTenantConfig(c *gin.Context, id uuid.UUID) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req core.UpdateTenantConfigJSONBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	_, err := exh.store.UpdateTenantConfig(c,
		repository.UpdateTenantConfigParams{
			ID:       id,
			Name:     req.Name,
			Value:    util.ToNullableText(req.Value),
			TenantID: tenantID.(string),
		})
	if err != nil {
		log.Error().Err(err).Msg("Error updating tenant config")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteTenantConfig implements openapi.ServerInterface.
func (exh *TenantConfigHandler) DeleteTenantConfig(c *gin.Context, id uuid.UUID) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	_, err := exh.store.DeleteTenantConfig(c, repository.DeleteTenantConfigParams{
		ID:       id,
		TenantID: tenantID.(string),
	})
	if err != nil {
		log.Error().Err(err).Msg("Error deleting tenant config")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// FindTenantConfigByID implements openapi.ServerInterface.
func (exh *TenantConfigHandler) GetTenantConfigByID(c *gin.Context, id uuid.UUID) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	tenantConfig, err := exh.store.GetTenantConfigByID(c, repository.GetTenantConfigByIDParams{
		ID:       id,
		TenantID: tenantID.(string),
	})
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, tenantConfig)
}

// ListTenantConfigs implements openapi.ServerInterface.
func (exh *TenantConfigHandler) ListTenantConfigs(c *gin.Context, params core.ListTenantConfigsParams) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
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

	query := repository.ListTenantConfigsParams{
		Limit:    pagingSql.PageSize,
		Offset:   pagingSql.Offset,
		Like:     like,
		SortBy:   pagingSql.SortBy,
		Order:    pagingSql.Order,
		TenantID: tenantID.(string),
	}

	tenantConfigs, err := exh.store.ListTenantConfigs(c, query)
	if err != nil {
		log.Error().Err(err).Msg("Error listing tenant configs")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	if params.Detail != nil && *params.Detail == "basic" {
		basicEntities := make([]subentity.BasicEntity, 0)
		for _, tenantConfig := range tenantConfigs {
			basicEntity := subentity.BasicEntity{
				ID:   tenantConfig.ID.String(),
				Name: tenantConfig.Name,
			}
			basicEntities = append(basicEntities, basicEntity)
		}
		c.JSON(http.StatusOK, basicEntities)
	} else {
		c.JSON(http.StatusOK, tenantConfigs)
	}
}

func NewTenantConfigHandler(store *db.Store, authProvider auth.AuthProvider) *TenantConfigHandler {
	return &TenantConfigHandler{
		store:        store,
		authProvider: authProvider,
	}
}
