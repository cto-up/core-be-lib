package config

import (
	"net/http"

	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	access "ctoup.com/coreapp/pkg/shared/service"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-One_Of
type GlobalConfigHandler struct {
	authClientPool *auth.AuthProviderAdapter
	store          *db.Store
}

// AddGlobalConfig implements openapi.ServerInterface.
func (exh *GlobalConfigHandler) AddGlobalConfig(c *gin.Context) {
	var req core.AddGlobalConfigJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	userID, exist := c.Get(access.AUTH_USER_ID)
	if !exist {
		// should not happen as the middleware ensures that the user is authenticated
		c.JSON(http.StatusBadRequest, "Need to be authenticated")
		return
	}
	globalConfig, err := exh.store.CreateGlobalConfig(c,
		repository.CreateGlobalConfigParams{
			UserID: userID.(string),
			Name:   req.Name,
			Value:  util.ToNullableText(req.Value),
		})
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusCreated, globalConfig)
}

// UpdateGlobalConfig implements openapi.ServerInterface.
func (exh *GlobalConfigHandler) UpdateGlobalConfig(c *gin.Context, id uuid.UUID) {
	var req core.UpdateGlobalConfigJSONBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	_, err := exh.store.UpdateGlobalConfig(c,
		repository.UpdateGlobalConfigParams{
			ID:    id,
			Name:  req.Name,
			Value: util.ToNullableText(req.Value),
		})
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// DeleteGlobalConfig implements openapi.ServerInterface.
func (exh *GlobalConfigHandler) DeleteGlobalConfig(c *gin.Context, id uuid.UUID) {
	_, err := exh.store.DeleteGlobalConfig(c, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// FindGlobalConfigByID implements openapi.ServerInterface.
func (exh *GlobalConfigHandler) GetGlobalConfigByID(c *gin.Context, id uuid.UUID) {
	globalConfig, err := exh.store.GetGlobalConfigByID(c, id)
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusOK, globalConfig)
}

// ListGlobalConfigs implements openapi.ServerInterface.
func (exh *GlobalConfigHandler) ListGlobalConfigs(c *gin.Context, params core.ListGlobalConfigsParams) {
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

	query := repository.ListGlobalConfigsParams{
		Limit:  pagingSql.PageSize,
		Offset: pagingSql.Offset,
		Like:   like,
		SortBy: pagingSql.SortBy,
		Order:  pagingSql.Order,
	}

	globalConfigs, err := exh.store.ListGlobalConfigs(c, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	if params.Detail != nil && *params.Detail == "basic" {
		basicEntities := make([]subentity.BasicEntity, 0)
		for _, globalConfig := range globalConfigs {
			basicEntity := subentity.BasicEntity{
				ID:   globalConfig.ID.String(),
				Name: globalConfig.Name,
			}
			basicEntities = append(basicEntities, basicEntity)
		}
		c.JSON(http.StatusOK, basicEntities)
	} else {
		c.JSON(http.StatusOK, globalConfigs)
	}
}

func NewGlobalConfigHandler(store *db.Store, authClientPool *auth.AuthProviderAdapter) *GlobalConfigHandler {
	return &GlobalConfigHandler{
		store:          store,
		authClientPool: authClientPool,
	}
}
