package auth

import (
	"context"
	"sync"
	"time"

	"firebase.google.com/go/auth"
)

// MultitenantService interface for getting tenant information
type MultitenantService interface {
	GetFirebaseTenantID(ctx context.Context, subdomain string) (string, error)
}

// FirebaseAuthProvider implements AuthProvider for Firebase Authentication
type FirebaseAuthProvider struct {
	client             *auth.Client
	tenantManager      *FirebaseTenantManager
	multitenantService MultitenantService
	tenantClients      map[string]*auth.TenantClient
	mu                 sync.RWMutex
}

// NewFirebaseAuthProvider creates a new Firebase auth provider
func NewFirebaseAuthProvider(ctx context.Context, client *auth.Client, multitenantService MultitenantService) *FirebaseAuthProvider {
	provider := &FirebaseAuthProvider{
		client:             client,
		multitenantService: multitenantService,
		tenantClients:      make(map[string]*auth.TenantClient),
	}
	provider.tenantManager = &FirebaseTenantManager{
		client:   client,
		provider: provider,
	}
	return provider
}

func (f *FirebaseAuthProvider) GetAuthClient() AuthClient {
	return &FirebaseAuthClient{client: f.client}
}

func (f *FirebaseAuthProvider) GetTenantManager() TenantManager {
	return f.tenantManager
}

func (f *FirebaseAuthProvider) GetAuthClientForSubdomain(ctx context.Context, subdomain string) (AuthClient, error) {
	tenantID, err := f.multitenantService.GetFirebaseTenantID(ctx, subdomain)
	if err != nil {
		return nil, err
	}
	return f.GetAuthClientForTenant(ctx, tenantID)
}

func (f *FirebaseAuthProvider) GetAuthClientForTenant(ctx context.Context, tenantID string) (AuthClient, error) {
	if tenantID == "" {
		return f.GetAuthClient(), nil
	}

	f.mu.RLock()
	tenantClient, exists := f.tenantClients[tenantID]
	f.mu.RUnlock()

	if exists {
		return &FirebaseAuthClient{client: tenantClient}, nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	// Double-check after acquiring write lock
	if tenantClient, exists = f.tenantClients[tenantID]; exists {
		return &FirebaseAuthClient{client: tenantClient}, nil
	}

	// Create new tenant client
	tenantClient, err := f.client.TenantManager.AuthForTenant(tenantID)
	if err != nil {
		return nil, &AuthError{
			Code:    ErrorCodeTenantNotFound,
			Message: "failed to get tenant auth client",
			Err:     err,
		}
	}

	f.tenantClients[tenantID] = tenantClient
	return &FirebaseAuthClient{client: tenantClient}, nil
}

func (f *FirebaseAuthProvider) GetProviderName() string {
	return "firebase"
}

// FirebaseAuthClient wraps Firebase auth.Client to implement AuthClient interface
type FirebaseAuthClient struct {
	client firebaseAuthClientInterface
}

// firebaseAuthClientInterface abstracts both *auth.Client and *auth.TenantClient
type firebaseAuthClientInterface interface {
	CreateUser(ctx context.Context, user *auth.UserToCreate) (*auth.UserRecord, error)
	UpdateUser(ctx context.Context, uid string, user *auth.UserToUpdate) (*auth.UserRecord, error)
	DeleteUser(ctx context.Context, uid string) error
	GetUser(ctx context.Context, uid string) (*auth.UserRecord, error)
	GetUserByEmail(ctx context.Context, email string) (*auth.UserRecord, error)
	SetCustomUserClaims(ctx context.Context, uid string, customClaims map[string]interface{}) error
	EmailVerificationLink(ctx context.Context, email string) (string, error)
	PasswordResetLink(ctx context.Context, email string) (string, error)
	EmailVerificationLinkWithSettings(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error)
	PasswordResetLinkWithSettings(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error)
	EmailSignInLink(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error)
	VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error)
}

func (f *FirebaseAuthClient) CreateUser(ctx context.Context, user *UserToCreate) (*UserRecord, error) {
	fbUser := (&auth.UserToCreate{}).
		Email(user.GetEmail()).
		EmailVerified(user.GetEmailVerified()).
		DisplayName(user.GetDisplayName()).
		PhotoURL(user.GetPhotoURL()).
		Disabled(user.GetDisabled())

	if password := user.GetPassword(); password != nil {
		fbUser = fbUser.Password(*password)
	}

	record, err := f.client.CreateUser(ctx, fbUser)
	if err != nil {
		return nil, convertFirebaseError(err)
	}

	return convertFirebaseUserRecord(record), nil
}

func (f *FirebaseAuthClient) UpdateUser(ctx context.Context, uid string, user *UserToUpdate) (*UserRecord, error) {
	fbUser := &auth.UserToUpdate{}

	if email := user.GetEmail(); email != nil {
		fbUser = fbUser.Email(*email)
	}
	if emailVerified := user.GetEmailVerified(); emailVerified != nil {
		fbUser = fbUser.EmailVerified(*emailVerified)
	}
	if displayName := user.GetDisplayName(); displayName != nil {
		fbUser = fbUser.DisplayName(*displayName)
	}
	if photoURL := user.GetPhotoURL(); photoURL != nil {
		fbUser = fbUser.PhotoURL(*photoURL)
	}
	if disabled := user.GetDisabled(); disabled != nil {
		fbUser = fbUser.Disabled(*disabled)
	}
	if password := user.GetPassword(); password != nil {
		fbUser = fbUser.Password(*password)
	}

	record, err := f.client.UpdateUser(ctx, uid, fbUser)
	if err != nil {
		return nil, convertFirebaseError(err)
	}

	return convertFirebaseUserRecord(record), nil
}

func (f *FirebaseAuthClient) DeleteUser(ctx context.Context, uid string) error {
	err := f.client.DeleteUser(ctx, uid)
	if err != nil {
		return convertFirebaseError(err)
	}
	return nil
}

func (f *FirebaseAuthClient) GetUser(ctx context.Context, uid string) (*UserRecord, error) {
	record, err := f.client.GetUser(ctx, uid)
	if err != nil {
		return nil, convertFirebaseError(err)
	}
	return convertFirebaseUserRecord(record), nil
}

func (f *FirebaseAuthClient) GetUserByEmail(ctx context.Context, email string) (*UserRecord, error) {
	record, err := f.client.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, convertFirebaseError(err)
	}
	return convertFirebaseUserRecord(record), nil
}

func (f *FirebaseAuthClient) SetCustomUserClaims(ctx context.Context, uid string, customClaims map[string]interface{}) error {
	err := f.client.SetCustomUserClaims(ctx, uid, customClaims)
	if err != nil {
		return convertFirebaseError(err)
	}
	return nil
}

func (f *FirebaseAuthClient) EmailVerificationLink(ctx context.Context, email string) (string, error) {
	link, err := f.client.EmailVerificationLink(ctx, email)
	if err != nil {
		return "", convertFirebaseError(err)
	}
	return link, nil
}

func (f *FirebaseAuthClient) PasswordResetLink(ctx context.Context, email string) (string, error) {
	link, err := f.client.PasswordResetLink(ctx, email)
	if err != nil {
		return "", convertFirebaseError(err)
	}
	return link, nil
}

func (f *FirebaseAuthClient) EmailVerificationLinkWithSettings(ctx context.Context, email string, settings *ActionCodeSettings) (string, error) {
	fbSettings := convertToFirebaseActionCodeSettings(settings)
	link, err := f.client.EmailVerificationLinkWithSettings(ctx, email, fbSettings)
	if err != nil {
		return "", convertFirebaseError(err)
	}
	return link, nil
}

func (f *FirebaseAuthClient) PasswordResetLinkWithSettings(ctx context.Context, email string, settings *ActionCodeSettings) (string, error) {
	fbSettings := convertToFirebaseActionCodeSettings(settings)
	link, err := f.client.PasswordResetLinkWithSettings(ctx, email, fbSettings)
	if err != nil {
		return "", convertFirebaseError(err)
	}
	return link, nil
}

func (f *FirebaseAuthClient) EmailSignInLink(ctx context.Context, email string, settings *ActionCodeSettings) (string, error) {
	fbSettings := convertToFirebaseActionCodeSettings(settings)
	link, err := f.client.EmailSignInLink(ctx, email, fbSettings)
	if err != nil {
		return "", convertFirebaseError(err)
	}
	return link, nil
}

func (f *FirebaseAuthClient) VerifyIDToken(ctx context.Context, idToken string) (*Token, error) {
	token, err := f.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, convertFirebaseError(err)
	}
	return &Token{
		UID:    token.UID,
		Claims: token.Claims,
	}, nil
}

// FirebaseTenantManager implements TenantManager for Firebase
type FirebaseTenantManager struct {
	client   *auth.Client
	provider *FirebaseAuthProvider
}

func (f *FirebaseTenantManager) CreateTenant(ctx context.Context, config *TenantConfig) (*Tenant, error) {
	tenantConfig := (&auth.TenantToCreate{}).
		DisplayName(config.DisplayName)

	// Note: Firebase SDK methods may vary - adjust based on your version
	// These are placeholder implementations

	fbTenant, err := f.client.TenantManager.CreateTenant(ctx, tenantConfig)
	if err != nil {
		return nil, &AuthError{
			Code:    "tenant-creation-failed",
			Message: "failed to create tenant",
			Err:     err,
		}
	}

	return &Tenant{
		ID:                    fbTenant.ID,
		DisplayName:           fbTenant.DisplayName,
		EnableEmailLinkSignIn: config.EnableEmailLinkSignIn,
		AllowPasswordSignUp:   config.AllowPasswordSignUp,
	}, nil
}

func (f *FirebaseTenantManager) UpdateTenant(ctx context.Context, tenantID string, config *TenantConfig) (*Tenant, error) {
	tenantConfig := (&auth.TenantToUpdate{}).
		DisplayName(config.DisplayName)

	fbTenant, err := f.client.TenantManager.UpdateTenant(ctx, tenantID, tenantConfig)
	if err != nil {
		return nil, &AuthError{
			Code:    "tenant-update-failed",
			Message: "failed to update tenant",
			Err:     err,
		}
	}

	return &Tenant{
		ID:                    fbTenant.ID,
		DisplayName:           fbTenant.DisplayName,
		EnableEmailLinkSignIn: config.EnableEmailLinkSignIn,
		AllowPasswordSignUp:   config.AllowPasswordSignUp,
	}, nil
}

func (f *FirebaseTenantManager) DeleteTenant(ctx context.Context, tenantID string) error {
	err := f.client.TenantManager.DeleteTenant(ctx, tenantID)
	if err != nil {
		return &AuthError{
			Code:    "tenant-deletion-failed",
			Message: "failed to delete tenant",
			Err:     err,
		}
	}

	// Remove from cache
	f.provider.mu.Lock()
	delete(f.provider.tenantClients, tenantID)
	f.provider.mu.Unlock()

	return nil
}

func (f *FirebaseTenantManager) GetTenant(ctx context.Context, tenantID string) (*Tenant, error) {
	fbTenant, err := f.client.TenantManager.Tenant(ctx, tenantID)
	if err != nil {
		return nil, &AuthError{
			Code:    ErrorCodeTenantNotFound,
			Message: "failed to get tenant",
			Err:     err,
		}
	}

	return &Tenant{
		ID:                    fbTenant.ID,
		DisplayName:           fbTenant.DisplayName,
		EnableEmailLinkSignIn: false, // Firebase tenant doesn't expose this
		AllowPasswordSignUp:   true,  // Firebase tenant doesn't expose this
	}, nil
}

func (f *FirebaseTenantManager) AuthForTenant(ctx context.Context, tenantID string) (AuthClient, error) {
	return f.provider.GetAuthClientForTenant(ctx, tenantID)
}

// Helper functions

func convertFirebaseUserRecord(fbUser *auth.UserRecord) *UserRecord {
	return &UserRecord{
		UID:           fbUser.UID,
		Email:         fbUser.Email,
		EmailVerified: fbUser.EmailVerified,
		DisplayName:   fbUser.DisplayName,
		PhotoURL:      fbUser.PhotoURL,
		Disabled:      fbUser.Disabled,
		CreatedAt:     time.Unix(fbUser.UserMetadata.CreationTimestamp, 0),
		CustomClaims:  fbUser.CustomClaims,
	}
}

func convertToFirebaseActionCodeSettings(settings *ActionCodeSettings) *auth.ActionCodeSettings {
	if settings == nil {
		return nil
	}
	return &auth.ActionCodeSettings{
		URL:                   settings.URL,
		HandleCodeInApp:       settings.HandleCodeInApp,
		DynamicLinkDomain:     settings.DynamicLinkDomain,
		IOSBundleID:           settings.IOSBundleID,
		AndroidPackageName:    settings.AndroidPackageName,
		AndroidInstallApp:     settings.AndroidInstallApp,
		AndroidMinimumVersion: settings.AndroidMinimumVersion,
	}
}

func convertFirebaseError(err error) error {
	if err == nil {
		return nil
	}

	// Check for specific Firebase errors
	if auth.IsUserNotFound(err) {
		return &AuthError{
			Code:    ErrorCodeUserNotFound,
			Message: "user not found",
			Err:     err,
		}
	}
	if auth.IsEmailAlreadyExists(err) {
		return &AuthError{
			Code:    ErrorCodeEmailAlreadyExists,
			Message: "email already exists",
			Err:     err,
		}
	}

	// Generic error
	return &AuthError{
		Code:    "unknown",
		Message: "authentication error",
		Err:     err,
	}
}
