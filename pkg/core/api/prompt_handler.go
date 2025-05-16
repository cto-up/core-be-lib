package core

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"ctoup.com/coreapp/api/helpers"
	api "ctoup.com/coreapp/api/openapi/core"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/service"
	"ctoup.com/coreapp/pkg/core/service/gochains"
	"ctoup.com/coreapp/pkg/shared/event"
	"ctoup.com/coreapp/pkg/shared/llmmodels"
	"ctoup.com/coreapp/pkg/shared/repository/subentity"
	access "ctoup.com/coreapp/pkg/shared/service"
	"ctoup.com/coreapp/pkg/shared/util"
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
		UserID:             userID.(string),
		TenantID:           tenantID.(string),
		Name:               req.Name,
		Content:            req.Content,
		Tags:               req.Tags,
		Parameters:         req.Parameters,
		SampleParameters:   util.ToJSON(req.SampleParameters),
		Format:             string(req.Format),
		FormatInstructions: util.ToNullableText(&req.FormatInstructions),
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
		ID:                 id,
		TenantID:           tenantID.(string),
		Name:               pgtype.Text{String: req.Name, Valid: true},
		Content:            pgtype.Text{String: req.Content, Valid: true},
		Tags:               req.Tags,
		Parameters:         req.Parameters,
		SampleParameters:   util.ToJSON(req.SampleParameters),
		Format:             string(req.Format),
		FormatInstructions: util.ToNullableText(&req.FormatInstructions),
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
	promptDB, err := exh.store.GetPromptByID(c, repository.GetPromptByIDParams{
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

	sampleParameters := util.FromJSONB[map[string]string](promptDB.SampleParameters)

	prompt := api.Prompt{
		Id:                 promptDB.ID,
		Name:               promptDB.Name,
		Content:            promptDB.Content,
		Tags:               promptDB.Tags,
		Parameters:         promptDB.Parameters,
		SampleParameters:   &sampleParameters,
		Format:             api.PromptFormat(promptDB.Format),
		FormatInstructions: promptDB.FormatInstructions.String,
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
func (h *PromptHandler) FormatPrompt(c *gin.Context, params api.FormatPromptParams) {
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
	var content string
	if req.Content != nil {
		content = *req.Content
	} else {
		content = prompt.Content
	}

	var parameters []string
	if req.Parameters != nil {
		parameters = *req.Parameters
	} else {
		parameters = prompt.Parameters
	}

	result, err := h.executionService.ExecutePrompt(c, content, parameters, service.ExecutePromptParams{
		Parameters: *req.ParametersValues,
	})

	if err != nil {
		if strings.HasPrefix(err.Error(), "missing required parameter:") {
			c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
			return
		}
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, api.PromptResponse{
		Result: result,
	})
}

// ExecutePrompt implements api.ServerInterface.
func (h *PromptHandler) ExecutePrompt(c *gin.Context, queryParams api.ExecutePromptParams) {
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
	id := queryParams.Id
	name := queryParams.Name
	if id == nil && name == nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(errors.New("id or name must be provided")))
		return
	}

	// Fetch prompt
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

	userID, exist := c.Get(access.AUTH_USER_ID)
	if !exist {
		c.JSON(http.StatusBadRequest, "Need to be authenticated")
		return
	}

	// Set up parameters
	maxTokens := 2000
	if queryParams.MaxTokens != nil {
		maxTokens = int(*queryParams.MaxTokens)
	}
	if queryParams.Llm == nil {
		c.JSON(http.StatusBadRequest, "LLM must be provided as query parameter")
		return
	}
	llm := string(*queryParams.Llm)

	if queryParams.Provider == nil {
		c.JSON(http.StatusBadRequest, "Provider must be provided as query parameter")
		return
	}
	if !llmmodels.Provider(*queryParams.Provider).IsValid() {
		c.JSON(http.StatusBadRequest, "Invalid provider")
		return
	}
	provider := llmmodels.Provider(*queryParams.Provider)

	temperature := 0.7
	if queryParams.Temperature != nil {
		temperature = float64(*queryParams.Temperature)
	}

	// Set up content and parameters
	var content string
	if req.Content != nil {
		content = *req.Content
	} else {
		content = prompt.Content
	}
	var parameters []string
	if req.Parameters != nil {
		parameters = *req.Parameters
	} else {
		parameters = prompt.Parameters
	}
	formatInstructions := ""
	if req.FormatInstructions != nil {
		formatInstructions = *req.FormatInstructions
	} else {
		formatInstructions = prompt.FormatInstructions.String
	}

	format := "markdown"
	if req.Format != nil {
		format = string(*req.Format)
	} else {
		format = prompt.Format
	}

	// Create chain config
	chainConfig, err := gochains.NewBaseChain(
		content,
		parameters,
		formatInstructions,
		maxTokens,
		temperature,
		provider,
		llm,
		format == "json",
	)
	if err != nil {
		log.Printf("Error NewBaseChain: %v", err)
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(errors.New("NewBaseChain error")))
		return
	}

	// Check if streaming is requested
	streaming := c.GetHeader("Accept") == "text/event-stream"

	// Convert parameters
	parametersValues := make(map[string]any)
	if req.ParametersValues != nil {
		for k, v := range *req.ParametersValues {
			parametersValues[k] = v
		}
	}

	// Handle non-streaming case
	if !streaming {
		generatedAnswer, err := h.executionService.GenerateAnswer(c,
			chainConfig,
			parametersValues,
			userID.(string),
			nil,
		)

		if err != nil {
			log.Printf("Error generating answer: %v", err)
			c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
			return
		}

		c.JSON(http.StatusOK, api.PromptResponse{
			Result: generatedAnswer,
		})
		return
	}

	// Handle streaming case
	clientChan := make(chan event.ProgressEvent)
	errorChan := make(chan error, 1)
	resultChan := make(chan string, 1)

	// Set headers for SSE before any data is written
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	// Start generation in goroutine
	go func() {
		defer close(clientChan)

		answer, err := h.executionService.GenerateAnswer(c,
			chainConfig,
			parametersValues,
			userID.(string),
			clientChan,
		)

		if err != nil {
			errorChan <- err
			return
		}

		resultChan <- answer
	}()

	// Stream events to client
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
	// No need to send JSON response after streaming
	// The client will receive the final result as an SSE event
}

func NewPromptHandler(store *db.Store, authClientPool *access.FirebaseTenantClientConnectionPool) *PromptHandler {
	return &PromptHandler{
		store:            store,
		authClientPool:   authClientPool,
		executionService: service.NewPromptExecutionService(store),
	}
}
