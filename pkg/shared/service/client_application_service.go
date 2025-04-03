package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

// Client application service constants
const (
	// Token format: prefix_random_base62
	TokenPrefix        = "api_"
	TokenRandomLength  = 32
	TokenPrefixLength  = 8   // First 8 chars for display/identification
	DefaultTokenExpiry = 90  // 90 days default expiry
	MaxTokenExpiry     = 365 // Maximum token expiry in days
	TokenAuditCreated  = "CREATED"
	TokenAuditUsed     = "USED"
	TokenAuditRevoked  = "REVOKED"
	TokenAuditUpdated  = "UPDATED"
)

// ClientApplicationService handles client applications and API tokens
type ClientApplicationService struct {
	store *db.Store
}

// NewClientApplicationService creates a new client application service
func NewClientApplicationService(store *db.Store) *ClientApplicationService {
	return &ClientApplicationService{
		store: store,
	}
}

// CreateClientApplication creates a new client application
func (s *ClientApplicationService) CreateClientApplication(ctx context.Context, tenantID string, name, description, createdBy string) (repository.CoreClientApplication, error) {
	log.Info().Str("tenantID", tenantID).Str("name", name).Msg("Creating client application")

	// Tenant ID can be null for super admin (global) applications
	var tenantIDParam *string
	if tenantID != "" {
		tenantIDParam = &tenantID
	}

	app, err := s.store.CreateClientApplication(ctx, repository.CreateClientApplicationParams{
		Name:        name,
		Description: pgtype.Text{String: description, Valid: true},
		TenantID:    util.ToNullableText(tenantIDParam),
		CreatedBy:   createdBy,
	})

	if err != nil {
		log.Error().Err(err).Str("tenantID", tenantID).Str("name", name).Msg("Failed to create client application")
		return repository.CoreClientApplication{}, err
	}

	return app, nil
}

// GetClientApplicationByID retrieves a client application by ID
func (s *ClientApplicationService) GetClientApplicationByID(ctx context.Context, id uuid.UUID, tenantID string) (repository.CoreClientApplication, error) {
	var tenantIDParam *string
	if tenantID != "" {
		tenantIDParam = &tenantID
	}

	app, err := s.store.GetClientApplicationByID(ctx, repository.GetClientApplicationByIDParams{
		ID:       id,
		TenantID: util.ToNullableText(tenantIDParam),
	})

	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Failed to get client application")
		return repository.CoreClientApplication{}, err
	}

	return app, nil
}

// ListClientApplications returns a list of client applications
func (s *ClientApplicationService) ListClientApplications(ctx context.Context, tenantID string,
	limit, offset int32, sortBy, order string,
	searchQuery string, includeInactive bool) ([]repository.CoreClientApplication, error) {

	var tenantIDParam *string
	if tenantID != "" {
		tenantIDParam = &tenantID
	}

	var includeInactiveParam *bool
	if includeInactive {
		includeInactiveParam = &includeInactive
	}

	var likeParam *pgtype.Text
	if searchQuery != "" {
		likeParam = &pgtype.Text{
			String: searchQuery + "%",
			Valid:  true,
		}
	}

	apps, err := s.store.ListClientApplications(ctx, repository.ListClientApplicationsParams{
		TenantID:        util.ToNullableText(tenantIDParam),
		IncludeInactive: util.ToNullableBool(includeInactiveParam),
		Like:            likeParam,
		Limit:           limit,
		Offset:          offset,
		SortBy:          sortBy,
		Order:           order,
	})

	if err != nil {
		log.Error().Err(err).Str("tenantID", tenantID).Msg("Failed to list client applications")
		return nil, err
	}

	return apps, nil
}

// UpdateClientApplication updates a client application
func (s *ClientApplicationService) UpdateClientApplication(ctx context.Context, id uuid.UUID,
	tenantID string, name, description string, active bool) (repository.CoreClientApplication, error) {

	var tenantIDParam *string
	if tenantID != "" {
		tenantIDParam = &tenantID
	}

	app, err := s.store.UpdateClientApplication(ctx, repository.UpdateClientApplicationParams{
		ID:          id,
		Name:        name,
		Description: pgtype.Text{String: description, Valid: true},
		Active:      active,
		TenantID:    util.ToNullableText(tenantIDParam),
	})

	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Failed to update client application")
		return repository.CoreClientApplication{}, err
	}

	return app, nil
}

// DeactivateClientApplication deactivates a client application
func (s *ClientApplicationService) DeactivateClientApplication(ctx context.Context, id uuid.UUID, tenantID string) error {
	var tenantIDParam *string
	if tenantID != "" {
		tenantIDParam = &tenantID
	}

	_, err := s.store.DeactivateClientApplication(ctx, repository.DeactivateClientApplicationParams{
		ID:       id,
		TenantID: util.ToNullableText(tenantIDParam),
	})

	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Failed to deactivate client application")
		return err
	}

	return nil
}

// DeleteClientApplication deletes a client application
func (s *ClientApplicationService) DeleteClientApplication(ctx context.Context, id uuid.UUID, tenantID string) error {
	var tenantIDParam *string
	if tenantID != "" {
		tenantIDParam = &tenantID
	}

	_, err := s.store.DeleteClientApplication(ctx, repository.DeleteClientApplicationParams{
		ID:       id,
		TenantID: util.ToNullableText(tenantIDParam),
	})

	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Failed to delete client application")
		return err
	}

	return nil
}

// GenerateSecureToken generates a secure random token
func (s *ClientApplicationService) GenerateSecureToken() (string, string, []byte, error) {
	// Generate random bytes
	randomBytes := make([]byte, TokenRandomLength)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", "", nil, err
	}

	// Encode to base64
	randomString := base64.RawURLEncoding.EncodeToString(randomBytes)

	// Create full token
	token := fmt.Sprintf("%s%s", TokenPrefix, randomString)

	// Get first 8 chars of token for display purposes
	tokenPrefix := token[:TokenPrefixLength]

	// Hash token for storage
	hash := sha256.Sum256([]byte(token))
	tokenHash := hash[:]

	return token, tokenPrefix, tokenHash, nil
}

// CreateAPIToken creates a new API token for a client application
func (s *ClientApplicationService) CreateAPIToken(ctx *gin.Context, clientApplicationID uuid.UUID,
	name, description string, expiresInDays int, createdBy string, scopes []string) (string, repository.CoreApiToken, error) {

	// Validate client application exists and is active
	app, err := s.store.GetClientApplicationByID(ctx, repository.GetClientApplicationByIDParams{
		ID: clientApplicationID,
	})

	if err != nil {
		log.Error().Err(err).Str("clientApplicationID", clientApplicationID.String()).Msg("Failed to get client application for token creation")
		return "", repository.CoreApiToken{}, err
	}

	if !app.Active {
		log.Error().Str("clientApplicationID", clientApplicationID.String()).Msg("Cannot create token for inactive application")
		return "", repository.CoreApiToken{}, fmt.Errorf("cannot create token for inactive application")
	}

	// Generate token
	token, tokenPrefix, tokenHash, err := s.GenerateSecureToken()
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate secure token")
		return "", repository.CoreApiToken{}, err
	}

	// Set expiry
	if expiresInDays <= 0 {
		expiresInDays = DefaultTokenExpiry
	} else if expiresInDays > MaxTokenExpiry {
		expiresInDays = MaxTokenExpiry
	}

	expiryTime := time.Now().AddDate(0, 0, expiresInDays)

	// Create token in database
	var scopesArray []string
	if len(scopes) > 0 {
		scopesArray = scopes
	}

	apiToken, err := s.store.CreateAPIToken(ctx, repository.CreateAPITokenParams{
		ClientApplicationID: clientApplicationID,
		Name:                name,
		Description:         pgtype.Text{String: description, Valid: true},
		TokenHash:           tokenHash,
		TokenPrefix:         tokenPrefix,
		ExpiresAt:           expiryTime,
		CreatedBy:           createdBy,
		Scopes:              scopesArray,
	})

	if err != nil {
		log.Error().Err(err).Str("clientApplicationID", clientApplicationID.String()).Msg("Failed to create API token")
		return "", repository.CoreApiToken{}, err
	}

	// Create audit log entry
	ipAddress := ctx.ClientIP()
	userAgent := ctx.GetHeader("User-Agent")

	_, err = s.store.CreateAPITokenAuditLog(ctx, repository.CreateAPITokenAuditLogParams{
		TokenID:        apiToken.ID,
		Action:         TokenAuditCreated,
		IpAddress:      pgtype.Text{String: ipAddress, Valid: true},
		UserAgent:      pgtype.Text{String: userAgent, Valid: true},
		AdditionalData: nil,
	})

	if err != nil {
		log.Warn().Err(err).Str("tokenID", apiToken.ID.String()).Msg("Failed to create audit log for token creation")
		// Don't fail the token creation if audit log fails
	}

	// Update the client application's last used timestamp
	err = s.store.UpdateClientApplicationLastUsed(ctx, clientApplicationID)
	if err != nil {
		log.Warn().Err(err).Str("clientApplicationID", clientApplicationID.String()).Msg("Failed to update client application last used timestamp")
	}

	return token, apiToken, nil
}

// GetAPITokenByID retrieves an API token by ID
func (s *ClientApplicationService) GetAPITokenByID(ctx context.Context, id uuid.UUID, tenantID string) (repository.GetAPITokenByIDRow, error) {
	var tenantIDParam *string
	if tenantID != "" {
		tenantIDParam = &tenantID
	}

	token, err := s.store.GetAPITokenByID(ctx, repository.GetAPITokenByIDParams{
		ID:       id,
		TenantID: util.ToNullableText(tenantIDParam),
	})

	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Failed to get API token")
		return repository.GetAPITokenByIDRow{}, err
	}

	return token, nil
}

// ListAPITokens lists API tokens for a client application
func (s *ClientApplicationService) ListAPITokens(ctx context.Context, clientApplicationID *uuid.UUID,
	tenantID string, limit, offset int32, sortBy, order string,
	includeRevoked, includeExpired bool) ([]repository.ListAPITokensRow, error) {

	var tenantIDParam *string
	if tenantID != "" {
		tenantIDParam = &tenantID
	}

	var clientAppIDParam *uuid.UUID
	if clientApplicationID != nil {
		clientAppIDParam = clientApplicationID
	}

	var includeRevokedParam *bool
	if includeRevoked {
		includeRevokedParam = &includeRevoked
	}

	var includeExpiredParam *bool
	if includeExpired {
		includeExpiredParam = &includeExpired
	}

	tokens, err := s.store.ListAPITokens(ctx, repository.ListAPITokensParams{
		ClientApplicationID: util.ToNullableUUID(clientAppIDParam),
		TenantID:            util.ToNullableText(tenantIDParam),
		IncludeRevoked:      util.ToNullableBool(includeRevokedParam),
		IncludeExpired:      util.ToNullableBool(includeExpiredParam),
		Limit:               limit,
		Offset:              offset,
		SortBy:              sortBy,
		Order:               order,
	})

	if err != nil {
		log.Error().Err(err).Msg("Failed to list API tokens")
		return nil, err
	}

	return tokens, nil
}

// RevokeAPIToken revokes an API token
func (s *ClientApplicationService) RevokeAPIToken(ctx *gin.Context, id uuid.UUID, reason, revokedBy string) (repository.CoreApiToken, error) {
	// Check if token exists and is not already revoked
	token, err := s.store.GetAPITokenByID(ctx, repository.GetAPITokenByIDParams{
		ID: id,
	})

	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Failed to get API token for revocation")
		return repository.CoreApiToken{}, err
	}

	if token.Revoked {
		log.Warn().Str("id", id.String()).Msg("Token is already revoked")
		return repository.CoreApiToken{}, fmt.Errorf("token is already revoked")
	}

	// Revoke token
	revokedToken, err := s.store.RevokeAPIToken(ctx, repository.RevokeAPITokenParams{
		ID:            id,
		RevokedReason: pgtype.Text{String: reason, Valid: true},
		RevokedBy:     pgtype.Text{String: revokedBy, Valid: true},
	})

	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Failed to revoke API token")
		return repository.CoreApiToken{}, err
	}

	// Create audit log entry
	ipAddress := ctx.ClientIP()
	userAgent := ctx.GetHeader("User-Agent")

	_, err = s.store.CreateAPITokenAuditLog(ctx, repository.CreateAPITokenAuditLogParams{
		TokenID:        id,
		Action:         TokenAuditRevoked,
		IpAddress:      pgtype.Text{String: ipAddress, Valid: true},
		UserAgent:      pgtype.Text{String: userAgent, Valid: true},
		AdditionalData: nil,
	})

	if err != nil {
		log.Warn().Err(err).Str("tokenID", id.String()).Msg("Failed to create audit log for token revocation")
		// Don't fail the revocation if audit log fails
	}

	return revokedToken, nil
}

// DeleteAPIToken deletes an API token
func (s *ClientApplicationService) DeleteAPIToken(ctx context.Context, id uuid.UUID) error {
	_, err := s.store.DeleteAPIToken(ctx, id)

	if err != nil {
		log.Error().Err(err).Str("id", id.String()).Msg("Failed to delete API token")
		return err
	}

	return nil
}

// GetAPITokenAuditLogs retrieves audit logs for an API token
func (s *ClientApplicationService) GetAPITokenAuditLogs(ctx context.Context, tokenID uuid.UUID, limit, offset int32) ([]repository.CoreApiTokenAuditLog, error) {
	logs, err := s.store.GetAPITokenAuditLogs(ctx, repository.GetAPITokenAuditLogsParams{
		TokenID: tokenID,
		Limit:   limit,
		Offset:  offset,
	})

	if err != nil {
		log.Error().Err(err).Str("tokenID", tokenID.String()).Msg("Failed to get API token audit logs")
		return nil, err
	}

	return logs, nil
}

// VerifyAPIToken verifies an API token and returns the associated application and token if valid
func (s *ClientApplicationService) VerifyAPIToken(ctx *gin.Context, tokenString string) (repository.GetAPITokenByHashRow, error) {
	// Sanitize token
	tokenString = strings.TrimSpace(tokenString)

	// Hash token for lookup
	hash := sha256.Sum256([]byte(tokenString))
	tokenHash := hash[:]

	// Look up token by hash
	token, err := s.store.GetAPITokenByHash(ctx, tokenHash)
	if err != nil {
		log.Error().Err(err).Msg("Failed to verify API token")
		return repository.GetAPITokenByHashRow{}, err
	}

	// Create audit log entry for token usage
	ipAddress := ctx.ClientIP()
	userAgent := ctx.GetHeader("User-Agent")

	_, err = s.store.CreateAPITokenAuditLog(ctx, repository.CreateAPITokenAuditLogParams{
		TokenID:        token.ID,
		Action:         TokenAuditUsed,
		IpAddress:      pgtype.Text{String: ipAddress, Valid: true},
		UserAgent:      pgtype.Text{String: userAgent, Valid: true},
		AdditionalData: nil,
	})

	if err != nil {
		log.Warn().Err(err).Str("tokenID", token.ID.String()).Msg("Failed to create audit log for token usage")
		// Don't fail the verification if audit log fails
	}

	// Update token last used
	err = s.store.UpdateAPITokenLastUsed(ctx, repository.UpdateAPITokenLastUsedParams{
		ID:        token.ID,
		IpAddress: pgtype.Text{String: ipAddress, Valid: true},
	})

	if err != nil {
		log.Warn().Err(err).Str("tokenID", token.ID.String()).Msg("Failed to update token last used timestamp")
	}

	// Update the client application's last used timestamp
	err = s.store.UpdateClientApplicationLastUsed(ctx, token.ClientApplicationID)
	if err != nil {
		log.Warn().Err(err).Str("clientApplicationID", token.ClientApplicationID.String()).Msg("Failed to update client application last used timestamp")
	}

	return token, nil
}

// APITokenMiddleware is a middleware for API token authentication
func APITokenMiddleware(clientAppService *ClientApplicationService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Public routes bypass token auth
		if strings.HasPrefix(c.Request.URL.Path, "/public") {
			c.Next()
			return
		}

		// Get token from Authorization header
		authHeader := c.Request.Header.Get("Authorization")
		token := ""

		// Check Bearer token first (OAuth 2.0 standard)
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else if strings.HasPrefix(authHeader, "Token ") {
			// Also accept Token prefix
			token = strings.TrimPrefix(authHeader, "Token ")
		} else if authHeader != "" {
			// Try to use header value directly
			token = authHeader
		}

		// If no token in header, check for query parameter (for websocket connections)
		if token == "" {
			token = c.Query("token")
		}

		if token == "" {
			// No token provided, let the next middleware handle auth
			c.Next()
			return
		}

		// Verify token
		apiToken, err := clientAppService.VerifyAPIToken(c, token)
		if err != nil {
			// Token verification failed, let the next middleware handle auth
			c.Next()
			return
		}

		// Store token info in context for later use
		c.Set("api_token", apiToken)
		c.Set("api_token_scopes", apiToken.Scopes)

		// If token has a tenant ID, set it for the tenant middleware
		if apiToken.TenantID.Valid {
			c.Set(AUTH_TENANT_ID_KEY, apiToken.TenantID.String)
		}

		c.Next()
	}
}

// HexEncodeTokenHash returns a hex-encoded token hash for display
func HexEncodeTokenHash(hash []byte) string {
	return hex.EncodeToString(hash)
}

// ValidateTokenScopes validates that a token has the required scopes
func ValidateTokenScopes(c *gin.Context, requiredScopes []string) bool {
	// If no required scopes, allow access
	if len(requiredScopes) == 0 {
		return true
	}

	// Get token scopes from context
	tokenScopes, exists := c.Get("api_token_scopes")
	if !exists {
		return false
	}

	// Check if all required scopes are present
	scopes, ok := tokenScopes.([]string)
	if !ok {
		return false
	}

	for _, requiredScope := range requiredScopes {
		if !util.Contains(scopes, requiredScope) {
			return false
		}
	}

	return true
}
