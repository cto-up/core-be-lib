package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// KratosAuthProvider implements AuthProvider for Ory Kratos
type KratosAuthProvider struct {
	client             KratosClient
	multitenantService MultitenantService
	tenantClients      map[string]KratosClient
}

// KratosClient interface for Ory Kratos operations
// This would be implemented using the Ory Kratos SDK
type KratosClient interface {
	// Add Kratos-specific methods here
	CreateIdentity(ctx context.Context, identity *KratosIdentity) (*KratosIdentity, error)
	UpdateIdentity(ctx context.Context, id string, identity *KratosIdentity) (*KratosIdentity, error)
	DeleteIdentity(ctx context.Context, id string) error
	GetIdentity(ctx context.Context, id string) (*KratosIdentity, error)
	GetIdentityByEmail(ctx context.Context, email string) (*KratosIdentity, error)
	// Add more methods as needed
}

// KratosIdentity represents a Kratos identity
type KratosIdentity struct {
	ID             string
	Email          string
	EmailVerified  bool
	Name           string
	Disabled       bool
	CreatedAt      time.Time
	MetadataPublic map[string]interface{}
	MetadataAdmin  map[string]interface{}
}

// NewKratosAuthProvider creates a new Kratos auth provider
func NewKratosAuthProvider(ctx context.Context, client KratosClient, multitenantService MultitenantService) *KratosAuthProvider {
	return &KratosAuthProvider{
		client:             client,
		multitenantService: multitenantService,
		tenantClients:      make(map[string]KratosClient),
	}
}

func (k *KratosAuthProvider) GetAuthClient() AuthClient {
	return &KratosAuthClient{client: k.client}
}

func (k *KratosAuthProvider) GetTenantManager() TenantManager {
	return &KratosTenantManager{provider: k}
}

func (k *KratosAuthProvider) GetAuthClientForSubdomain(ctx context.Context, subdomain string) (AuthClient, error) {
	// In Kratos, you might handle multi-tenancy differently
	// This is a placeholder implementation
	tenantID, err := k.multitenantService.GetFirebaseTenantID(ctx, subdomain)
	if err != nil {
		return nil, err
	}
	return k.GetAuthClientForTenant(ctx, tenantID)
}

func (k *KratosAuthProvider) GetAuthClientForTenant(ctx context.Context, tenantID string) (AuthClient, error) {
	// Kratos multi-tenancy implementation
	// This would depend on how you structure multi-tenancy in Kratos
	// For now, return the base client
	return k.GetAuthClient(), nil
}

func (k *KratosAuthProvider) GetProviderName() string {
	return "kratos"
}

// KratosAuthClient implements AuthClient for Ory Kratos
type KratosAuthClient struct {
	client KratosClient
}

func (k *KratosAuthClient) CreateUser(ctx context.Context, user *UserToCreate) (*UserRecord, error) {
	// Convert UserToCreate to KratosIdentity
	identity := &KratosIdentity{
		Email:         user.GetEmail(),
		EmailVerified: user.GetEmailVerified(),
		Name:          user.GetDisplayName(),
		Disabled:      user.GetDisabled(),
		MetadataPublic: map[string]interface{}{
			"photo_url": user.GetPhotoURL(),
		},
	}

	// Handle password if provided
	if password := user.GetPassword(); password != nil {
		// In Kratos, password would be set through credentials
		identity.MetadataAdmin = map[string]interface{}{
			"password": *password,
		}
	}

	created, err := k.client.CreateIdentity(ctx, identity)
	if err != nil {
		return nil, convertKratosError(err)
	}

	return convertKratosIdentityToUserRecord(created), nil
}

func (k *KratosAuthClient) UpdateUser(ctx context.Context, uid string, user *UserToUpdate) (*UserRecord, error) {
	// Get existing identity
	existing, err := k.client.GetIdentity(ctx, uid)
	if err != nil {
		return nil, convertKratosError(err)
	}

	// Update fields
	if email := user.GetEmail(); email != nil {
		existing.Email = *email
	}
	if emailVerified := user.GetEmailVerified(); emailVerified != nil {
		existing.EmailVerified = *emailVerified
	}
	if displayName := user.GetDisplayName(); displayName != nil {
		existing.Name = *displayName
	}
	if disabled := user.GetDisabled(); disabled != nil {
		existing.Disabled = *disabled
	}
	if photoURL := user.GetPhotoURL(); photoURL != nil {
		if existing.MetadataPublic == nil {
			existing.MetadataPublic = make(map[string]interface{})
		}
		existing.MetadataPublic["photo_url"] = *photoURL
	}

	updated, err := k.client.UpdateIdentity(ctx, uid, existing)
	if err != nil {
		return nil, convertKratosError(err)
	}

	return convertKratosIdentityToUserRecord(updated), nil
}

func (k *KratosAuthClient) DeleteUser(ctx context.Context, uid string) error {
	err := k.client.DeleteIdentity(ctx, uid)
	if err != nil {
		return convertKratosError(err)
	}
	return nil
}

func (k *KratosAuthClient) GetUser(ctx context.Context, uid string) (*UserRecord, error) {
	identity, err := k.client.GetIdentity(ctx, uid)
	if err != nil {
		return nil, convertKratosError(err)
	}
	return convertKratosIdentityToUserRecord(identity), nil
}

func (k *KratosAuthClient) GetUserByEmail(ctx context.Context, email string) (*UserRecord, error) {
	identity, err := k.client.GetIdentityByEmail(ctx, email)
	if err != nil {
		return nil, convertKratosError(err)
	}
	return convertKratosIdentityToUserRecord(identity), nil
}

func (k *KratosAuthClient) SetCustomUserClaims(ctx context.Context, uid string, customClaims map[string]interface{}) error {
	// In Kratos, custom claims would be stored in metadata_admin
	identity, err := k.client.GetIdentity(ctx, uid)
	if err != nil {
		return convertKratosError(err)
	}

	if identity.MetadataAdmin == nil {
		identity.MetadataAdmin = make(map[string]interface{})
	}
	identity.MetadataAdmin["custom_claims"] = customClaims

	_, err = k.client.UpdateIdentity(ctx, uid, identity)
	if err != nil {
		return convertKratosError(err)
	}

	return nil
}

func (k *KratosAuthClient) EmailVerificationLink(ctx context.Context, email string) (string, error) {
	// Kratos uses recovery/verification flows
	// This would need to be implemented using Kratos SDK
	log.Warn().Msg("EmailVerificationLink not fully implemented for Kratos")
	return "", &AuthError{
		Code:    "not-implemented",
		Message: "email verification link generation not implemented for Kratos",
	}
}

func (k *KratosAuthClient) PasswordResetLink(ctx context.Context, email string) (string, error) {
	// Kratos uses recovery flows
	log.Warn().Msg("PasswordResetLink not fully implemented for Kratos")
	return "", &AuthError{
		Code:    "not-implemented",
		Message: "password reset link generation not implemented for Kratos",
	}
}

func (k *KratosAuthClient) EmailVerificationLinkWithSettings(ctx context.Context, email string, settings *ActionCodeSettings) (string, error) {
	return k.EmailVerificationLink(ctx, email)
}

func (k *KratosAuthClient) PasswordResetLinkWithSettings(ctx context.Context, email string, settings *ActionCodeSettings) (string, error) {
	return k.PasswordResetLink(ctx, email)
}

func (k *KratosAuthClient) EmailSignInLink(ctx context.Context, email string, settings *ActionCodeSettings) (string, error) {
	log.Warn().Msg("EmailSignInLink not fully implemented for Kratos")
	return "", &AuthError{
		Code:    "not-implemented",
		Message: "email sign-in link generation not implemented for Kratos",
	}
}

func (k *KratosAuthClient) VerifyIDToken(ctx context.Context, idToken string) (*Token, error) {
	// Kratos uses session tokens instead of ID tokens
	// This would need to be implemented using Kratos session verification
	log.Warn().Msg("VerifyIDToken not fully implemented for Kratos")
	return nil, &AuthError{
		Code:    "not-implemented",
		Message: "token verification not implemented for Kratos",
	}
}

// KratosTenantManager implements TenantManager for Kratos
type KratosTenantManager struct {
	provider *KratosAuthProvider
}

func (k *KratosTenantManager) CreateTenant(ctx context.Context, config *TenantConfig) (*Tenant, error) {
	// Kratos doesn't have built-in multi-tenancy like Firebase
	// You would need to implement this using metadata or separate Kratos instances
	log.Warn().Msg("CreateTenant not fully implemented for Kratos")
	return nil, &AuthError{
		Code:    "not-implemented",
		Message: "tenant creation not implemented for Kratos",
	}
}

func (k *KratosTenantManager) UpdateTenant(ctx context.Context, tenantID string, config *TenantConfig) (*Tenant, error) {
	log.Warn().Msg("UpdateTenant not fully implemented for Kratos")
	return nil, &AuthError{
		Code:    "not-implemented",
		Message: "tenant update not implemented for Kratos",
	}
}

func (k *KratosTenantManager) DeleteTenant(ctx context.Context, tenantID string) error {
	log.Warn().Msg("DeleteTenant not fully implemented for Kratos")
	return &AuthError{
		Code:    "not-implemented",
		Message: "tenant deletion not implemented for Kratos",
	}
}

func (k *KratosTenantManager) GetTenant(ctx context.Context, tenantID string) (*Tenant, error) {
	log.Warn().Msg("GetTenant not fully implemented for Kratos")
	return nil, &AuthError{
		Code:    "not-implemented",
		Message: "get tenant not implemented for Kratos",
	}
}

func (k *KratosTenantManager) AuthForTenant(ctx context.Context, tenantID string) (AuthClient, error) {
	return k.provider.GetAuthClientForTenant(ctx, tenantID)
}

// Helper functions

func convertKratosIdentityToUserRecord(identity *KratosIdentity) *UserRecord {
	photoURL := ""
	if identity.MetadataPublic != nil {
		if url, ok := identity.MetadataPublic["photo_url"].(string); ok {
			photoURL = url
		}
	}

	customClaims := make(map[string]interface{})
	if identity.MetadataAdmin != nil {
		if claims, ok := identity.MetadataAdmin["custom_claims"].(map[string]interface{}); ok {
			customClaims = claims
		}
	}

	return &UserRecord{
		UID:           identity.ID,
		Email:         identity.Email,
		EmailVerified: identity.EmailVerified,
		DisplayName:   identity.Name,
		PhotoURL:      photoURL,
		Disabled:      identity.Disabled,
		CreatedAt:     identity.CreatedAt,
		CustomClaims:  customClaims,
	}
}

func convertKratosError(err error) error {
	if err == nil {
		return nil
	}

	// Convert Kratos-specific errors to AuthError
	// This would depend on the actual Kratos SDK error types
	errMsg := err.Error()

	if contains(errMsg, "not found") {
		return &AuthError{
			Code:    ErrorCodeUserNotFound,
			Message: "user not found",
			Err:     err,
		}
	}

	if contains(errMsg, "already exists") || contains(errMsg, "duplicate") {
		return &AuthError{
			Code:    ErrorCodeEmailAlreadyExists,
			Message: "email already exists",
			Err:     err,
		}
	}

	return &AuthError{
		Code:    "unknown",
		Message: "authentication error",
		Err:     err,
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			fmt.Sprintf("%s", s) != s))
}
