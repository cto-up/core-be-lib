package core

import (
	"errors"
	"net/http"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/auth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"

	"github.com/oapi-codegen/runtime/types"

	"ctoup.com/coreapp/api/helpers"
	api "ctoup.com/coreapp/api/openapi/core"
)

// CreateTranslation implements core.ServerInterface.
func (h *TranslationHandler) CreateTranslation(c *gin.Context) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	var request api.CreateTranslationJSONRequestBody
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	translation, err := h.store.CreateTranslation(c, repository.CreateTranslationParams{
		TenantID:   tenantID.(string),
		EntityType: request.EntityType,
		EntityID:   request.EntityId,
		Field:      request.Field,
		Language:   string(request.Language),
		Value:      request.Value,
	})
	if err != nil {
		log.Error().Err(err).Msg("Error creating translation")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusCreated, translation)
}

// DeleteTranslation implements core.ServerInterface.
func (h *TranslationHandler) DeleteTranslation(c *gin.Context, id types.UUID) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	err := h.store.DeleteTranslationById(c, repository.DeleteTranslationByIdParams{
		ID:       id,
		TenantID: tenantID.(string),
	})
	if err != nil {
		log.Error().Err(err).Msg("Error deleting translation")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}

// GetTranslationByID implements core.ServerInterface.
func (h *TranslationHandler) GetTranslationByID(c *gin.Context, id types.UUID, params api.GetTranslationByIDParams) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	translation, err := h.store.GetTranslationById(c, repository.GetTranslationByIdParams{
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

	c.JSON(http.StatusOK, translation)
}

func (h *TranslationHandler) GetTranslation(c *gin.Context, params api.GetTranslationParams) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	translation, err := h.store.GetTranslation(c, repository.GetTranslationParams{
		TenantID:   tenantID.(string),
		EntityType: params.EntityType,
		EntityID:   params.EntityId,
		Field:      params.Field,
		Language:   string(params.Language),
	})
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, translation)
}

// ListTranslations implements core.ServerInterface.
func (h *TranslationHandler) ListTranslations(c *gin.Context, params api.ListTranslationsParams) {
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

	translations, err := h.store.ListTranslations(c, repository.ListTranslationsParams{
		TenantID: tenantID.(string),
		Like:     like,
		Limit:    pagingSql.PageSize,
		Offset:   pagingSql.Offset,
		SortBy:   pagingSql.SortBy,
		Order:    pagingSql.Order,
	})
	if err != nil {
		log.Error().Err(err).Msg("Error listing translations")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, translations)
}

// UpdateTranslation implements core.ServerInterface.
func (h *TranslationHandler) UpdateTranslation(c *gin.Context, id types.UUID) {
	tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	var request api.UpdateTranslationJSONRequestBody
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	translation, err := h.store.UpdateTranslationById(c, repository.UpdateTranslationByIdParams{
		ID:       id,
		TenantID: tenantID.(string),
		Value:    request.Value,
	})
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, translation)
}

type TranslationHandler struct {
	store *db.Store
}

func NewTranslationHandler(store *db.Store) *TranslationHandler {
	handler := &TranslationHandler{store: store}
	return handler
}
