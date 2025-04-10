package core

import (
	"errors"
	"net/http"
	"strings"

	"ctoup.com/coreapp/api/helpers"
	api "ctoup.com/coreapp/api/openapi/core"
	"github.com/gin-gonic/gin"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/service"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	access "ctoup.com/coreapp/pkg/shared/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// https://pkg.go.dev/github.com/go-playground/validator/v10#hdr-One_Of
type PromptHandler struct {
	authClientPool   *access.FirebaseTenantClientConnectionPool
	store            *db.Store
	executionService *service.PromptExecutionService
}

// AddPrompt implements api.ServerInterface.
func (exh *PromptHandler) AddPrompt(c *gin.Context) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req api.AddPromptJSONRequestBody
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
	params := repository.CreatePromptParams{
		UserID:     userID.(string),
		TenantID:   tenantID.(string),
		Name:       req.Name,
		Content:    req.Content,
		Tags:       req.Tags,
		Parameters: req.Parameters,
	}
	prompt, err := exh.store.CreatePrompt(c, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.JSON(http.StatusCreated, prompt)
}

// UpdatePrompt implements api.ServerInterface.
func (exh *PromptHandler) UpdatePrompt(c *gin.Context, id uuid.UUID) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	var req api.UpdatePromptJSONBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}
	params := repository.UpdatePromptParams{
		ID:         id,
		TenantID:   tenantID.(string),
		Name:       pgtype.Text{String: req.Name, Valid: true},
		Content:    pgtype.Text{String: req.Content, Valid: true},
		Tags:       req.Tags,
		Parameters: req.Parameters,
	}
	_, err := exh.store.UpdatePrompt(c, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// DeletePrompt implements api.ServerInterface.
func (exh *PromptHandler) DeletePrompt(c *gin.Context, id uuid.UUID) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	_, err := exh.store.DeletePrompt(c, repository.DeletePromptParams{
		ID:       id,
		TenantID: tenantID.(string),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	c.Status(http.StatusNoContent)
}

// FindPromptByID implements api.ServerInterface.
func (exh *PromptHandler) GetPromptByID(c *gin.Context, id uuid.UUID) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}
	prompt, err := exh.store.GetPromptByID(c, repository.GetPromptByIDParams{
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
	c.JSON(http.StatusOK, prompt)
}

// ListPrompts implements api.ServerInterface.
func (exh *PromptHandler) ListPrompts(c *gin.Context, params api.ListPromptsParams) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
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

	query := repository.ListPromptsParams{
		Limit:    pagingSql.PageSize,
		Offset:   pagingSql.Offset,
		Like:     like,
		SortBy:   pagingSql.SortBy,
		Order:    pagingSql.Order,
		TenantID: tenantID.(string),
	}

	prompts, err := exh.store.ListPrompts(c, query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	if params.Detail != nil && *params.Detail == "basic" {
		basicEntities := make([]subentity.BasicEntity, 0)
		for _, prompt := range prompts {
			basicEntity := subentity.BasicEntity{
				ID:   prompt.ID.String(),
				Name: prompt.Name,
			}
			basicEntities = append(basicEntities, basicEntity)
		}
		c.JSON(http.StatusOK, basicEntities)
	} else {
		c.JSON(http.StatusOK, prompts)
	}
}

// ExecutePrompt implements api.ServerInterface.
func (h *PromptHandler) ExecutePrompt(c *gin.Context, params api.ExecutePromptParams) {
	tenantID, exists := c.Get(access.AUTH_TENANT_ID_KEY)
	if !exists {
		c.JSON(http.StatusInternalServerError, errors.New("TenantID not found"))
		return
	}

	var req api.ExecutePromptJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// get prompt by id in query params and convert into uuid.UUID
	id := params.Id
	name := params.Name
	if id == nil && name == nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(errors.New("id or name must be provided")))
		return
	}
	var prompt repository.CorePrompt
	var err error
	if id != nil {
		prompt, err = h.store.GetPromptByID(c, repository.GetPromptByIDParams{
			ID:       *id,
			TenantID: tenantID.(string),
		})
		if err != nil {
			if err.Error() == pgx.ErrNoRows.Error() {
				c.JSON(http.StatusNotFound, helpers.ErrorResponse(errors.New("prompt not found")))
				return
			}
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}

	} else {
		prompt, err = h.store.GetPromptByName(c, repository.GetPromptByNameParams{
			Name:     *name,
			TenantID: tenantID.(string),
		})
		if err != nil {
			if err.Error() == pgx.ErrNoRows.Error() {
				c.JSON(http.StatusNotFound, helpers.ErrorResponse(errors.New("prompt not found")))
				return
			}
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}

	}

	result, err := h.executionService.ExecutePrompt(c, prompt, service.ExecutePromptParams{
		Parameters: *req.Parameters,
	})

	if err != nil {
		if strings.HasPrefix(err.Error(), "missing required parameter:") {
			c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
			return
		}
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, api.ExecutePromptResponse{
		Result: result,
	})
}

func NewPromptHandler(store *db.Store, authClientPool *access.FirebaseTenantClientConnectionPool) *PromptHandler {
	return &PromptHandler{
		store:            store,
		authClientPool:   authClientPool,
		executionService: service.NewPromptExecutionService(store),
	}
}
