package service

import (
	"context"
	"fmt"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/auth/kratos"
	"ctoup.com/coreapp/pkg/shared/util"
)

// KratosTenantService handles tenant-user associations for Kratos
type KratosTenantService struct {
	store        *db.Store
	authProvider auth.AuthProvider
}

// NewKratosTenantService creates a new Kratos tenant service
func NewKratosTenantService(store *db.Store, authProvider auth.AuthProvider) *KratosTenantService {
	return &KratosTenantService{
		store:        store,
		authProvider: authProvider,
	}
}

// AssignUserToTenant assigns a user to a tenant by updating their Kratos metadata
func (kts *KratosTenantService) AssignUserToTenant(ctx context.Context, userID string, subdomain string) error {
	// Get tenant from database
	tenant, err := kts.store.GetTenantBySubdomain(ctx, subdomain)
	if err != nil {
		return fmt.Errorf("failed to get tenant: %w", err)
	}

	// Get Kratos auth client
	authClient := kts.authProvider.GetAuthClient()
	kratosClient, ok := authClient.(*kratos.KratosAuthClient)
	if !ok {
		return fmt.Errorf("auth provider is not Kratos")
	}

	// Set tenant metadata
	metadata := kratos.TenantMetadata{
		TenantID:   tenant.TenantID,
		Subdomain:  tenant.Subdomain,
		TenantName: tenant.Name,
	}

	err = kratosClient.SetTenantMetadata(ctx, userID, metadata)
	if err != nil {
		return fmt.Errorf("failed to set tenant metadata: %w", err)
	}

	return nil
}

// GetUserTenant retrieves the tenant information for a user
func (kts *KratosTenantService) GetUserTenant(ctx context.Context, userID string) (*kratos.TenantMetadata, error) {
	authClient := kts.authProvider.GetAuthClient()
	kratosClient, ok := authClient.(*kratos.KratosAuthClient)
	if !ok {
		return nil, fmt.Errorf("auth provider is not Kratos")
	}

	return kratosClient.GetTenantMetadata(ctx, userID)
}

// RemoveUserFromTenant removes tenant association from a user
func (kts *KratosTenantService) RemoveUserFromTenant(ctx context.Context, userID string) error {
	logger := util.GetLoggerFromCtx(ctx)
	authClient := kts.authProvider.GetAuthClient()
	kratosClient, ok := authClient.(*kratos.KratosAuthClient)
	if !ok {
		return fmt.Errorf("auth provider is not Kratos")
	}

	// Set empty tenant metadata
	metadata := kratos.TenantMetadata{
		TenantID:  "",
		Subdomain: "",
	}

	err := kratosClient.SetTenantMetadata(ctx, userID, metadata)
	if err != nil {
		logger.Err(err).Str("user_id", userID).Msg("Failed to remove tenant metadata")
		return fmt.Errorf("failed to remove tenant metadata: %w", err)
	}
	return nil
}

// CreateUserWithTenant creates a new user and assigns them to a tenant
func (kts *KratosTenantService) CreateUserWithTenant(ctx context.Context, email string, password string, subdomain string, roles []string) (*auth.UserRecord, error) {
	logger := util.GetLoggerFromCtx(ctx)
	// Get tenant from database
	tenant, err := kts.store.GetTenantBySubdomain(ctx, subdomain)
	if err != nil {
		logger.Err(err).Str("subdomain", subdomain).Msg("Failed to get tenant")
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	// Get Kratos auth client
	authClient := kts.authProvider.GetAuthClient()
	kratosClient, ok := authClient.(*kratos.KratosAuthClient)
	if !ok {
		logger.Error().Msg("Auth provider is not Kratos")
		return nil, fmt.Errorf("auth provider is not Kratos")
	}

	// Create user with tenant
	user := (&auth.UserToCreate{}).
		Email(email).
		Password(password)

	record, err := kratosClient.CreateUserWithTenant(ctx, user, tenant.TenantID, tenant.Subdomain)
	if err != nil {
		logger.Err(err).Str("email", email).Str("subdomain", subdomain).Msg("Failed to create user with tenant")
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Set roles if provided
	if len(roles) > 0 {
		customClaims := make(map[string]interface{})
		for _, role := range roles {
			customClaims[role] = true
		}
		err = authClient.SetCustomUserClaims(ctx, record.UID, customClaims)
		if err != nil {
			logger.Err(err).Str("user_id", record.UID).Msg("Failed to set user roles")
			// Don't fail the entire operation, just log the error
		}
	}
	return record, nil
}

// ListTenantUsers lists all users belonging to a tenant
func (kts *KratosTenantService) ListTenantUsers(ctx context.Context, subdomain string) ([]*auth.UserRecord, error) {
	logger := util.GetLoggerFromCtx(ctx)
	// Get tenant from database
	tenant, err := kts.store.GetTenantBySubdomain(ctx, subdomain)
	if err != nil {
		logger.Err(err).Str("subdomain", subdomain).Msg("Failed to get tenant")
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	// Get Kratos auth client
	authClient := kts.authProvider.GetAuthClient()
	kratosClient, ok := authClient.(*kratos.KratosAuthClient)
	if !ok {
		logger.Error().Msg("Auth provider is not Kratos")
		return nil, fmt.Errorf("auth provider is not Kratos")
	}

	return kratosClient.ListUsersByTenant(ctx, tenant.TenantID)
}

// ValidateUserTenantAccess checks if a user has access to a specific tenant
func (kts *KratosTenantService) ValidateUserTenantAccess(ctx context.Context, userID string, subdomain string) (bool, error) {
	logger := util.GetLoggerFromCtx(ctx)
	// Get user's tenant metadata
	userTenant, err := kts.GetUserTenant(ctx, userID)
	if err != nil {
		logger.Err(err).Str("user_id", userID).Msg("Failed to get user tenant metadata")
		return false, fmt.Errorf("failed to get user tenant: %w", err)
	}

	// Get requested tenant from database
	tenant, err := kts.store.GetTenantBySubdomain(ctx, subdomain)
	if err != nil {
		logger.Err(err).Str("subdomain", subdomain).Msg("Failed to get tenant")
		return false, fmt.Errorf("failed to get tenant: %w", err)
	}

	// Check if user's tenant matches requested tenant
	return userTenant.TenantID == tenant.TenantID, nil
}
