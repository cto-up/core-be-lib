package core

import (
	"net/http"
	"time"

	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	access "ctoup.com/coreapp/pkg/shared/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

// ClientApplicationHandler handles client application endpoints
type ClientApplicationHandler struct {
	store            *db.Store
	clientAppService *access.ClientApplicationService
}

// Creates a new client application handler
func NewClientApplicationHandler(store *db.Store, clientAppService *access.ClientApplicationService) *ClientApplicationHandler {
	return &ClientApplicationHandler{
		store:            store,
		clientAppService: clientAppService,
	}
}

// Convert repository model to API model
func toAPIClientApplication(app repository.CoreClientApplication) core.ClientApplication {
	result := core.ClientApplication{
		Id:     app.ID,
		Name:   app.Name,
		Active: app.Active,
	}

	if app.Description.Valid {
		result.Description = app.Description.String
	}

	if app.TenantID.Valid {
		result.TenantId = &app.TenantID.String
	}

	if app.LastUsedAt.Valid {
		lastUsed := app.LastUsedAt.Time
		result.LastUsedAt = &lastUsed
	}

	return result
}

// Convert repository token model to API model
func toAPIToken(token repository.ListAPITokensRow) core.APIToken {
	result := core.APIToken{
		Id:                  token.ID,
		ClientApplicationId: token.ClientApplicationID,
		ApplicationName:     token.ApplicationName,
		Name:                token.Name,
		TokenPrefix:         token.TokenPrefix,
		ExpiresAt:           token.ExpiresAt,
		Revoked:             token.Revoked,
		CreatedBy:           token.CreatedBy,
		CreatedAt:           token.CreatedAt,
		UpdatedAt:           token.UpdatedAt,
	}

	if token.RevokedAt.Valid {
		revokedAt := token.RevokedAt.Time
		result.RevokedAt = &revokedAt
	}

	if token.RevokedReason.Valid {
		result.RevokedReason = &token.RevokedReason.String
	}

	if token.RevokedBy.Valid {
		result.RevokedBy = &token.RevokedBy.String
	}

	if token.Scopes != nil {
		result.Scopes = &token.Scopes
	}

	if token.LastUsedAt.Valid {
		lastUsed := token.LastUsedAt.Time
		result.LastUsedAt = &lastUsed
	}

	if token.LastUsedIp.Valid {
		result.LastUsedIp = &token.LastUsedIp.String
	}

	return result
}

func toAPITokenSingle(token repository.GetAPITokenByIDRow) core.APIToken {
	result := core.APIToken{
		Id:                  token.ID,
		ClientApplicationId: token.ClientApplicationID,
		Name:                token.Name,
		TokenPrefix:         token.TokenPrefix,
		ExpiresAt:           token.ExpiresAt,
		Revoked:             token.Revoked,
		CreatedBy:           token.CreatedBy,
		CreatedAt:           token.CreatedAt,
		UpdatedAt:           token.UpdatedAt,
	}

	if token.Description.Valid {
		result.Description = &token.Description.String
	}

	if token.RevokedAt.Valid {
		revokedAt := token.RevokedAt.Time
		result.RevokedAt = &revokedAt
	}

	if token.RevokedReason.Valid {
		result.RevokedReason = &token.RevokedReason.String
	}

	if token.RevokedBy.Valid {
		result.RevokedBy = &token.RevokedBy.String
	}

	if token.Scopes != nil {
		result.Scopes = &token.Scopes
	}

	if token.LastUsedAt.Valid {
		lastUsed := token.LastUsedAt.Time
		result.LastUsedAt = &lastUsed
	}

	if token.LastUsedIp.Valid {
		result.LastUsedIp = &token.LastUsedIp.String
	}

	return result
}

func toAPITokenCreated(token string, apiToken repository.CoreApiToken) core.APITokenCreated {
	result := core.APITokenCreated{
		Token: token,
		ApiToken: core.APIToken{
			Id:                  apiToken.ID,
			ClientApplicationId: apiToken.ClientApplicationID,
			Name:                apiToken.Name,
			TokenPrefix:         apiToken.TokenPrefix,
			ExpiresAt:           apiToken.ExpiresAt,
			Revoked:             apiToken.Revoked,
			CreatedBy:           apiToken.CreatedBy,
			CreatedAt:           apiToken.CreatedAt,
			UpdatedAt:           apiToken.UpdatedAt,
		},
	}

	if apiToken.Description.Valid {
		result.ApiToken.Description = &apiToken.Description.String
	}

	if apiToken.Scopes != nil {
		result.ApiToken.Scopes = &apiToken.Scopes
	}

	return result
}

// Convert audit log to API model
func toAPIAuditLog(log repository.CoreApiTokenAuditLog) core.APITokenAuditLog {
	result := core.APITokenAuditLog{
		Id:        log.ID,
		TokenId:   log.TokenID,
		Action:    core.APITokenAuditLogAction(log.Action),
		Timestamp: log.Timestamp,
	}

	if log.IpAddress.Valid {
		result.IpAddress = &log.IpAddress.String
	}

	if log.UserAgent.Valid {
		result.UserAgent = &log.UserAgent.String
	}

	return result
}

// ListClientApplications returns all client applications
func (h *ClientApplicationHandler) ListClientApplications(c *gin.Context, params core.ListClientApplicationsParams) {
	// Only super admins can access this endpoint
	// The middleware should already check for SUPER_ADMIN role
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Set up paging parameters
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

	// Check for optional include inactive parameter
	includeInactive := false
	if params.IncludeInactive != nil {
		includeInactive = *params.IncludeInactive
	}

	// Set up search
	searchQuery := ""
	if params.Q != nil {
		searchQuery = *params.Q
	}

	// List applications without tenant restriction for super admin
	apps, err := h.clientAppService.ListClientApplications(
		c,
		"", // Empty tenant ID for super admin to list all apps
		pagingSql.PageSize,
		pagingSql.Offset,
		pagingSql.SortBy,
		pagingSql.Order,
		searchQuery,
		includeInactive,
	)

	if err != nil {
		log.Error().Err(err).Str("userID", userID.(string)).Msg("Failed to list client applications")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Convert results to API model
	result := make([]core.ClientApplication, len(apps))
	for i, app := range apps {
		result[i] = toAPIClientApplication(app)
	}

	c.JSON(http.StatusOK, result)
}

// CreateClientApplication creates a new client application
func (h *ClientApplicationHandler) CreateClientApplication(c *gin.Context) {
	// Only super admins can access this endpoint
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Parse request body
	var req core.CreateClientApplicationJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Create application (no tenant ID for super admin apps)
	description := req.Description

	app, err := h.clientAppService.CreateClientApplication(
		c,
		"", // Empty tenant ID for super admin apps
		req.Name,
		description,
		userID.(string),
	)

	if err != nil {
		log.Error().Err(err).Str("userID", userID.(string)).Str("name", req.Name).Msg("Failed to create client application")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusCreated, toAPIClientApplication(app))
}

// GetClientApplicationById returns a client application by ID
func (h *ClientApplicationHandler) GetClientApplicationById(c *gin.Context, id uuid.UUID) {
	// Only super admins can access this endpoint
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Get application
	app, err := h.clientAppService.GetClientApplicationByID(c, id, "")
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		log.Error().Err(err).Str("userID", userID.(string)).Str("appID", id.String()).Msg("Failed to get client application")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, toAPIClientApplication(app))
}

// UpdateClientApplication updates a client application
func (h *ClientApplicationHandler) UpdateClientApplication(c *gin.Context, id uuid.UUID) {
	// Only super admins can access this endpoint
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Parse request body
	var req core.UpdateClientApplicationJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Get current application
	app, err := h.clientAppService.GetClientApplicationByID(c, id, "")
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		log.Error().Err(err).Str("userID", userID.(string)).Str("appID", id.String()).Msg("Failed to get client application for update")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Update application
	description := req.Description

	updatedApp, err := h.clientAppService.UpdateClientApplication(
		c,
		id,
		"", // Empty tenant ID for super admin
		req.Name,
		description,
		app.Active, // Keep current active status
	)

	if err != nil {
		log.Error().Err(err).Str("userID", userID.(string)).Str("appID", id.String()).Msg("Failed to update client application")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, toAPIClientApplication(updatedApp))
}

// DeleteClientApplication deletes a client application
func (h *ClientApplicationHandler) DeleteClientApplication(c *gin.Context, id uuid.UUID) {
	// Only super admins can access this endpoint
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	// Delete application
	err := h.clientAppService.DeleteClientApplication(c, id, "")
	if err != nil {
		log.Error().Err(err).Str("userID", userID.(string)).Str("appID", id.String()).Msg("Failed to delete client application")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}

// DeactivateClientApplication deactivates a client application
func (h *ClientApplicationHandler) DeactivateClientApplication(c *gin.Context, id uuid.UUID) {
	// Only super admins can access this endpoint
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Deactivate application
	err := h.clientAppService.DeactivateClientApplication(c, id, "")
	if err != nil {
		log.Error().Err(err).Str("userID", userID.(string)).Str("appID", id.String()).Msg("Failed to deactivate client application")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}

// ListAPITokens lists API tokens for a client application
func (h *ClientApplicationHandler) ListAPITokens(c *gin.Context, id uuid.UUID, params core.ListAPITokensParams) {
	// Only super admins can access this endpoint
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	// Set up paging parameters
	pagingRequest := helpers.PagingRequest{
		MaxPageSize:     50,
		DefaultPage:     1,
		DefaultPageSize: 10,
		DefaultSortBy:   "created_at",
		DefaultOrder:    "desc",
		Page:            params.Page,
		PageSize:        params.PageSize,
		SortBy:          params.SortBy,
		Order:           (*string)(params.Order),
	}

	pagingSql := helpers.GetPagingSQL(pagingRequest)

	// Check for optional parameters
	includeRevoked := false
	if params.IncludeRevoked != nil {
		includeRevoked = *params.IncludeRevoked
	}

	includeExpired := false
	if params.IncludeExpired != nil {
		includeExpired = *params.IncludeExpired
	}

	// List tokens
	tokens, err := h.clientAppService.ListAPITokens(
		c,
		&id,
		"", // Empty tenant ID for super admin
		pagingSql.PageSize,
		pagingSql.Offset,
		pagingSql.SortBy,
		pagingSql.Order,
		includeRevoked,
		includeExpired,
	)

	if err != nil {
		log.Error().Err(err).Str("userID", userID.(string)).Str("appID", id.String()).Msg("Failed to list API tokens")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Convert results to API model
	result := make([]core.APIToken, len(tokens))
	for i, token := range tokens {
		result[i] = toAPIToken(token)
	}

	c.JSON(http.StatusOK, result)
}

// CreateAPIToken creates a new API token for a client application
func (h *ClientApplicationHandler) CreateAPIToken(c *gin.Context, id uuid.UUID) {
	// Only super admins can access this endpoint
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Parse request body
	var req core.CreateAPITokenJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Prepare parameters
	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	// Calculate expiry days from timestamp
	expiresAt := req.ExpiresAt

	// Calculate days from now
	expiryDays := int(time.Until(expiresAt).Hours() / 24)

	// Handle scopes
	var scopes []string
	if req.Scopes != nil {
		scopes = *req.Scopes
	}

	// Create API token
	token, apiToken, err := h.clientAppService.CreateAPIToken(
		c,
		id,
		req.Name,
		description,
		expiryDays,
		userID.(string),
		scopes,
	)

	if err != nil {
		log.Error().Err(err).Str("userID", userID.(string)).Str("appID", id.String()).Msg("Failed to create API token")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Return the token and API token
	c.JSON(http.StatusCreated, toAPITokenCreated(token, apiToken))
}

// GetAPITokenById retrieves an API token by ID
func (h *ClientApplicationHandler) GetAPITokenById(c *gin.Context, id uuid.UUID, tokenId uuid.UUID) {
	// Only super admins can access this endpoint
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Get token
	token, err := h.clientAppService.GetAPITokenByID(c, tokenId, "")
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		log.Error().Err(err).Str("userID", userID.(string)).Str("tokenID", id.String()).Msg("Failed to get API token")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	if token.ClientApplicationID != id {
		c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		return
	}

	c.JSON(http.StatusOK, toAPITokenSingle(token))
}

// DeleteAPIToken deletes an API token
func (h *ClientApplicationHandler) DeleteAPIToken(c *gin.Context, id uuid.UUID, tokenId uuid.UUID) {
	// Only super admins can access this endpoint
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}
	// Verify token exists and belongs to the client application
	token, err := h.clientAppService.GetAPITokenByID(c, tokenId, "")
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		log.Error().Err(err).Str("userID", userID.(string)).Str("tokenID", tokenId.String()).Msg("Failed to get API token for deletion")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	if token.ClientApplicationID != id {
		c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		return
	}

	// Delete token
	err = h.clientAppService.DeleteAPIToken(c, tokenId)
	if err != nil {
		log.Error().Err(err).Str("userID", userID.(string)).Str("tokenID", tokenId.String()).Msg("Failed to delete API token")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	c.Status(http.StatusNoContent)
}

// RevokeAPIToken revokes an API token
func (h *ClientApplicationHandler) RevokeAPIToken(c *gin.Context, id uuid.UUID, tokenId uuid.UUID) {
	// Only super admins can access this endpoint
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Verify token exists and belongs to the client application
	token, err := h.clientAppService.GetAPITokenByID(c, tokenId, "")
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		log.Error().Err(err).Str("userID", userID.(string)).Str("tokenID", id.String()).Msg("Failed to get API token for revocation")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}
	if token.ClientApplicationID != id {
		c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		return
	}

	// Parse request body
	var req core.RevokeAPITokenJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, helpers.ErrorResponse(err))
		return
	}

	// Set revocation reason
	reason := req.Reason

	// Revoke token
	revokedToken, err := h.clientAppService.RevokeAPIToken(c, tokenId, reason, userID.(string))
	if err != nil {
		log.Error().Err(err).Str("userID", userID.(string)).Str("tokenID", tokenId.String()).Msg("Failed to revoke API token")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Convert to API model
	apiToken := core.APIToken{
		Id:                  revokedToken.ID,
		ClientApplicationId: revokedToken.ClientApplicationID,
		Name:                revokedToken.Name,
		TokenPrefix:         revokedToken.TokenPrefix,
		ExpiresAt:           revokedToken.ExpiresAt,
		Revoked:             revokedToken.Revoked,
		CreatedBy:           revokedToken.CreatedBy,
		CreatedAt:           revokedToken.CreatedAt,
		UpdatedAt:           revokedToken.UpdatedAt,
	}

	if revokedToken.Description.Valid {
		apiToken.Description = &revokedToken.Description.String
	}

	if revokedToken.RevokedAt.Valid {
		revokedAt := revokedToken.RevokedAt.Time
		apiToken.RevokedAt = &revokedAt
	}

	if revokedToken.RevokedReason.Valid {
		apiToken.RevokedReason = &revokedToken.RevokedReason.String
	}

	if revokedToken.RevokedBy.Valid {
		apiToken.RevokedBy = &revokedToken.RevokedBy.String
	}

	if revokedToken.Scopes != nil {
		apiToken.Scopes = &revokedToken.Scopes
	}

	c.JSON(http.StatusOK, apiToken)
}

// GetAPITokenAuditLogs retrieves audit logs for an API token
func (h *ClientApplicationHandler) GetAPITokenAuditLogs(c *gin.Context, id uuid.UUID, tokenId uuid.UUID, params core.GetAPITokenAuditLogsParams) {
	// Only super admins can access this endpoint
	userID, exists := c.Get(access.AUTH_USER_ID)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Verify token exists and belongs to the client application
	token, err := h.clientAppService.GetAPITokenByID(c, tokenId, "")
	if err != nil {
		if err.Error() == pgx.ErrNoRows.Error() {
			c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
			return
		}
		log.Error().Err(err).Str("userID", userID.(string)).Str("tokenID", tokenId.String()).Msg("Failed to get API token for audit logs")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	if token.ClientApplicationID != id {
		c.JSON(http.StatusNotFound, helpers.ErrorResponse(err))
		return
	}

	// Set up paging
	page := int32(1)
	if params.Page != nil {
		page = *params.Page
	}

	pageSize := int32(20)
	if params.PageSize != nil {
		pageSize = *params.PageSize
	}

	offset := (page - 1) * pageSize

	// Get audit logs
	logs, err := h.clientAppService.GetAPITokenAuditLogs(c, tokenId, pageSize, offset)
	if err != nil {
		log.Error().Err(err).Str("userID", userID.(string)).Str("tokenID", tokenId.String()).Msg("Failed to get API token audit logs")
		c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(err))
		return
	}

	// Convert to API model
	result := make([]core.APITokenAuditLog, len(logs))
	for i, log := range logs {
		result[i] = toAPIAuditLog(log)
	}

	c.JSON(http.StatusOK, result)
}
