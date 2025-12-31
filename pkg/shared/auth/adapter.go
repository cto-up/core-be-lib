package auth

import (
	"context"

	"ctoup.com/coreapp/pkg/shared/service"
	"firebase.google.com/go/auth"
)

// AuthProviderAdapter adapts the new AuthProvider interface to work with existing code
// that expects the old FirebaseTenantClientConnectionPool
type AuthProviderAdapter struct {
	provider           AuthProvider
	multitenantService MultitenantService
}

// NewAuthProviderAdapter creates a new adapter
func NewAuthProviderAdapter(provider AuthProvider, multitenantService MultitenantService) *AuthProviderAdapter {
	return &AuthProviderAdapter{
		provider:           provider,
		multitenantService: multitenantService,
	}
}

// GetBaseAuthClient returns an auth client for a given subdomain
// This maintains compatibility with existing code
func (a *AuthProviderAdapter) GetBaseAuthClient(ctx context.Context, subdomain string) (service.BaseAuthClient, error) {
	authClient, err := a.provider.GetAuthClientForSubdomain(ctx, subdomain)
	if err != nil {
		return nil, err
	}

	// If the underlying client is already a Firebase client, return it directly
	if fbClient, ok := authClient.(*FirebaseAuthClient); ok {
		return fbClient.client, nil
	}

	// Otherwise, wrap it in an adapter
	return &BaseAuthClientAdapter{client: authClient}, nil
}

// GetBaseAuthClientForTenant returns an auth client for a given tenant ID
func (a *AuthProviderAdapter) GetBaseAuthClientForTenant(tenantID string) (service.BaseAuthClient, error) {
	authClient, err := a.provider.GetAuthClientForTenant(context.Background(), tenantID)
	if err != nil {
		return nil, err
	}

	// If the underlying client is already a Firebase client, return it directly
	if fbClient, ok := authClient.(*FirebaseAuthClient); ok {
		return fbClient.client, nil
	}

	// Otherwise, wrap it in an adapter
	return &BaseAuthClientAdapter{client: authClient}, nil
}

// GetClient returns the base auth client (for backward compatibility)
// This is used for super admin operations
func (a *AuthProviderAdapter) GetClient() interface{} {
	authClient := a.provider.GetAuthClient()

	// If it's a Firebase client, return the underlying Firebase auth.Client
	if fbClient, ok := authClient.(*FirebaseAuthClient); ok {
		return fbClient.client
	}

	return authClient
}

// BaseAuthClientAdapter adapts non-Firebase AuthClient implementations to the BaseAuthClient interface
// This is only used when the underlying provider is NOT Firebase
type BaseAuthClientAdapter struct {
	client AuthClient
}

func (b *BaseAuthClientAdapter) CreateUser(ctx context.Context, user *auth.UserToCreate) (*auth.UserRecord, error) {
	// This adapter is only used for non-Firebase providers
	// Since Firebase types can't be easily converted, this should not be called
	// The adapter returns the Firebase client directly when using Firebase provider
	return nil, &AuthError{
		Code:    "not-implemented",
		Message: "CreateUser with Firebase types is only supported with Firebase provider",
	}
}

func (b *BaseAuthClientAdapter) UpdateUser(ctx context.Context, uid string, user *auth.UserToUpdate) (*auth.UserRecord, error) {
	return nil, &AuthError{
		Code:    "not-implemented",
		Message: "UpdateUser with Firebase types is only supported with Firebase provider",
	}
}

func (b *BaseAuthClientAdapter) DeleteUser(ctx context.Context, uid string) error {
	return b.client.DeleteUser(ctx, uid)
}

func (b *BaseAuthClientAdapter) GetUser(ctx context.Context, uid string) (*auth.UserRecord, error) {
	record, err := b.client.GetUser(ctx, uid)
	if err != nil {
		return nil, err
	}
	return convertToFirebaseUserRecord(record), nil
}

func (b *BaseAuthClientAdapter) SetCustomUserClaims(ctx context.Context, uid string, customClaims map[string]interface{}) error {
	return b.client.SetCustomUserClaims(ctx, uid, customClaims)
}

func (b *BaseAuthClientAdapter) EmailVerificationLink(ctx context.Context, email string) (string, error) {
	return b.client.EmailVerificationLink(ctx, email)
}

func (b *BaseAuthClientAdapter) PasswordResetLink(ctx context.Context, email string) (string, error) {
	return b.client.PasswordResetLink(ctx, email)
}

func (b *BaseAuthClientAdapter) EmailVerificationLinkWithSettings(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	newSettings := convertFromFirebaseActionCodeSettings(settings)
	return b.client.EmailVerificationLinkWithSettings(ctx, email, newSettings)
}

func (b *BaseAuthClientAdapter) PasswordResetLinkWithSettings(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	newSettings := convertFromFirebaseActionCodeSettings(settings)
	return b.client.PasswordResetLinkWithSettings(ctx, email, newSettings)
}

func (b *BaseAuthClientAdapter) EmailSignInLink(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	newSettings := convertFromFirebaseActionCodeSettings(settings)
	return b.client.EmailSignInLink(ctx, email, newSettings)
}

func (b *BaseAuthClientAdapter) VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error) {
	token, err := b.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, err
	}
	return &auth.Token{
		UID:    token.UID,
		Claims: token.Claims,
	}, nil
}

// Helper conversion functions

func convertToFirebaseUserRecord(record *UserRecord) *auth.UserRecord {
	if record == nil {
		return nil
	}
	return &auth.UserRecord{
		UserInfo: &auth.UserInfo{
			UID:         record.UID,
			Email:       record.Email,
			DisplayName: record.DisplayName,
			PhotoURL:    record.PhotoURL,
		},
		EmailVerified: record.EmailVerified,
		Disabled:      record.Disabled,
		CustomClaims:  record.CustomClaims,
		UserMetadata: &auth.UserMetadata{
			CreationTimestamp: record.CreatedAt.Unix(),
		},
	}
}

func convertFromFirebaseActionCodeSettings(settings *auth.ActionCodeSettings) *ActionCodeSettings {
	if settings == nil {
		return nil
	}
	return &ActionCodeSettings{
		URL:                   settings.URL,
		HandleCodeInApp:       settings.HandleCodeInApp,
		DynamicLinkDomain:     settings.DynamicLinkDomain,
		IOSBundleID:           settings.IOSBundleID,
		AndroidPackageName:    settings.AndroidPackageName,
		AndroidInstallApp:     settings.AndroidInstallApp,
		AndroidMinimumVersion: settings.AndroidMinimumVersion,
	}
}
