package firebase

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/util"
	firebase "firebase.google.com/go"
	fbauth "firebase.google.com/go/auth"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/option"
)

func init() {
	auth.RegisterProvider(auth.ProviderTypeFirebase, func(ctx context.Context, config auth.ProviderConfig) (auth.AuthProvider, error) {
		// Extract information from config
		// This logic comes from the old factory.go

		multitenantService, ok := config.Options["multitenantService"].(auth.MultitenantService)
		if !ok {
			return nil, fmt.Errorf("multitenantService not provided in config options")
		}

		// Check for credentials in options first
		if client, ok := config.Credentials.(*fbauth.Client); ok {
			return NewFirebaseAuthProvider(ctx, client, multitenantService), nil
		}

		// Initialize Firebase client from FIREBASE_CONFIG environment variable
		client, err := initializeFirebaseClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize firebase client: %w", err)
		}

		return NewFirebaseAuthProvider(ctx, client, multitenantService), nil
	})
}

// FirebaseAuthProvider implements AuthProvider for Firebase Authentication
type FirebaseAuthProvider struct {
	client             *fbauth.Client
	tenantManager      *FirebaseTenantManager
	multitenantService auth.MultitenantService
	tenantClients      map[string]*fbauth.TenantClient
	mu                 sync.RWMutex
}

// NewFirebaseAuthProvider creates a new Firebase auth provider
func NewFirebaseAuthProvider(ctx context.Context, client *fbauth.Client, multitenantService auth.MultitenantService) *FirebaseAuthProvider {
	provider := &FirebaseAuthProvider{
		client:             client,
		multitenantService: multitenantService,
		tenantClients:      make(map[string]*fbauth.TenantClient),
	}
	provider.tenantManager = &FirebaseTenantManager{
		client:   client,
		provider: provider,
	}
	return provider
}

func (f *FirebaseAuthProvider) GetAuthClient() auth.AuthClient {
	return &FirebaseAuthClient{client: f.client}
}

func (f *FirebaseAuthProvider) VerifyToken(c *gin.Context) (*auth.AuthenticatedUser, error) {
	// Extract token from Authorization header or Token header
	authHeader := c.Request.Header.Get("Authorization")
	token := strings.Replace(authHeader, "Bearer ", "", 1)

	if token == "" {
		token = c.Request.Header.Get("Token")
	}

	if token == "" {
		return nil, fmt.Errorf("missing token")
	}

	// Get subdomain for tenant-aware authentication
	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		return nil, err
	}

	tenantID, err := f.multitenantService.GetTenantIDWithSubdomain(c.Request.Context(), subdomain)
	if err != nil {
		return nil, err
	}

	return f.VerifyTokenWithTenantID(c, tenantID, token)
}

func (f *FirebaseAuthProvider) VerifyTokenWithTenantID(c context.Context, tenantID string, token string) (*auth.AuthenticatedUser, error) {

	// Get tenant-specific auth client
	authClient, err := f.GetAuthClientForTenant(c, tenantID)
	if err != nil {
		return nil, err
	}

	// Verify the ID token
	idToken, err := authClient.VerifyIDToken(c, token)
	if err != nil {
		return nil, err
	}

	// Extract user information
	email, _ := idToken.Claims["email"].(string)
	emailVerified, _ := idToken.Claims["email_verified"].(bool)
	userID := idToken.UID

	return &auth.AuthenticatedUser{
		UserID:        userID,
		Email:         email,
		EmailVerified: emailVerified,
		Claims:        idToken.Claims,
	}, nil
}

func (f *FirebaseAuthProvider) GetTenantManager() auth.TenantManager {
	return f.tenantManager
}

func (f *FirebaseAuthProvider) GetAuthClientForSubdomain(ctx context.Context, subdomain string) (auth.AuthClient, error) {
	tenantID, err := f.multitenantService.GetTenantIDWithSubdomain(ctx, subdomain)
	if err != nil {
		return nil, err
	}
	return f.GetAuthClientForTenant(ctx, tenantID)
}

func (f *FirebaseAuthProvider) GetAuthClientForTenant(ctx context.Context, tenantID string) (auth.AuthClient, error) {
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
		log.Err(err).Msg("Failed to get tenant auth client")
		return nil, &auth.AuthError{
			Code:    auth.ErrorCodeTenantNotFound,
			Message: "failed to get tenant auth client",
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

func (f *FirebaseAuthClient) GetUnderlyingClient() interface{} {
	return f.client
}

// firebaseAuthClientInterface abstracts both *auth.Client and *auth.TenantClient
type firebaseAuthClientInterface interface {
	CreateUser(ctx context.Context, user *fbauth.UserToCreate) (*fbauth.UserRecord, error)
	UpdateUser(ctx context.Context, uid string, user *fbauth.UserToUpdate) (*fbauth.UserRecord, error)
	DeleteUser(ctx context.Context, uid string) error
	GetUser(ctx context.Context, uid string) (*fbauth.UserRecord, error)
	GetUserByEmail(ctx context.Context, email string) (*fbauth.UserRecord, error)
	SetCustomUserClaims(ctx context.Context, uid string, customClaims map[string]interface{}) error
	EmailVerificationLink(ctx context.Context, email string) (string, error)
	PasswordResetLink(ctx context.Context, email string) (string, error)
	EmailVerificationLinkWithSettings(ctx context.Context, email string, settings *fbauth.ActionCodeSettings) (string, error)
	PasswordResetLinkWithSettings(ctx context.Context, email string, settings *fbauth.ActionCodeSettings) (string, error)
	EmailSignInLink(ctx context.Context, email string, settings *fbauth.ActionCodeSettings) (string, error)
	VerifyIDToken(ctx context.Context, idToken string) (*fbauth.Token, error)
}

func (f *FirebaseAuthClient) CreateUser(ctx context.Context, user *auth.UserToCreate) (*auth.UserRecord, error) {
	fbUser := (&fbauth.UserToCreate{}).
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

func (f *FirebaseAuthClient) UpdateUser(ctx context.Context, uid string, user *auth.UserToUpdate) (*auth.UserRecord, error) {
	fbUser := &fbauth.UserToUpdate{}

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

func (f *FirebaseAuthClient) GetUser(ctx context.Context, uid string) (*auth.UserRecord, error) {
	record, err := f.client.GetUser(ctx, uid)
	if err != nil {
		return nil, convertFirebaseError(err)
	}
	return convertFirebaseUserRecord(record), nil
}

func (f *FirebaseAuthClient) GetUserByEmail(ctx context.Context, email string) (*auth.UserRecord, error) {
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

// BuildGlobalRoleClaims creates Firebase-specific claims format for global roles
// Returns: {"SUPER_ADMIN": true, "ADMIN": true}
func (f *FirebaseAuthClient) BuildGlobalRoleClaims(roles []string) map[string]interface{} {
	claims := make(map[string]interface{})
	for _, role := range roles {
		claims[role] = true
	}
	return claims
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

func (f *FirebaseAuthClient) EmailVerificationLinkWithSettings(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	fbSettings := convertToFirebaseActionCodeSettings(settings)
	link, err := f.client.EmailVerificationLinkWithSettings(ctx, email, fbSettings)
	if err != nil {
		return "", convertFirebaseError(err)
	}
	return link, nil
}

func (f *FirebaseAuthClient) PasswordResetLinkWithSettings(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	fbSettings := convertToFirebaseActionCodeSettings(settings)
	link, err := f.client.PasswordResetLinkWithSettings(ctx, email, fbSettings)
	if err != nil {
		return "", convertFirebaseError(err)
	}
	return link, nil
}

func (f *FirebaseAuthClient) EmailSignInLink(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	fbSettings := convertToFirebaseActionCodeSettings(settings)
	link, err := f.client.EmailSignInLink(ctx, email, fbSettings)
	if err != nil {
		return "", convertFirebaseError(err)
	}
	return link, nil
}

// RequiresRecoveryProxy returns false for Firebase since recovery links work directly
// without needing a backend proxy
func (f *FirebaseAuthClient) RequiresRecoveryProxy() bool {
	return false
}

func (f *FirebaseAuthClient) VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error) {
	token, err := f.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, convertFirebaseError(err)
	}
	return &auth.Token{
		UID:    token.UID,
		Claims: token.Claims,
	}, nil
}

// FirebaseTenantManager implements TenantManager for Firebase
type FirebaseTenantManager struct {
	client   *fbauth.Client
	provider *FirebaseAuthProvider
}

func (f *FirebaseTenantManager) CreateTenant(ctx context.Context, config *auth.TenantConfig) (*auth.Tenant, error) {
	tenantConfig := (&fbauth.TenantToCreate{}).
		DisplayName(config.DisplayName).AllowPasswordSignUp(config.AllowPasswordSignUp)

	fbTenant, err := f.client.TenantManager.CreateTenant(ctx, tenantConfig)
	if err != nil {
		log.Err(err).Msg("Failed to create tenant")
		return nil, &auth.AuthError{
			Code:    "tenant-creation-failed",
			Message: "failed to create tenant",
		}
	}

	return &auth.Tenant{
		ID:                    fbTenant.ID,
		DisplayName:           fbTenant.DisplayName,
		EnableEmailLinkSignIn: config.EnableEmailLinkSignIn,
		AllowPasswordSignUp:   config.AllowPasswordSignUp,
	}, nil
}

func (f *FirebaseTenantManager) UpdateTenant(ctx context.Context, tenantID string, config *auth.TenantConfig) (*auth.Tenant, error) {
	tenantConfig := (&fbauth.TenantToUpdate{}).
		DisplayName(config.DisplayName)

	fbTenant, err := f.client.TenantManager.UpdateTenant(ctx, tenantID, tenantConfig)
	if err != nil {
		log.Err(err).Msg("Failed to update tenant")
		return nil, &auth.AuthError{
			Code:    "tenant-update-failed",
			Message: "failed to update tenant",
		}
	}

	return &auth.Tenant{
		ID:                    fbTenant.ID,
		DisplayName:           fbTenant.DisplayName,
		EnableEmailLinkSignIn: config.EnableEmailLinkSignIn,
		AllowPasswordSignUp:   config.AllowPasswordSignUp,
	}, nil
}

func (f *FirebaseTenantManager) DeleteTenant(ctx context.Context, tenantID string) error {
	err := f.client.TenantManager.DeleteTenant(ctx, tenantID)
	if err != nil {
		log.Err(err).Msg("Failed to delete tenant")
		return &auth.AuthError{
			Code:    "tenant-deletion-failed",
			Message: "failed to delete tenant",
		}
	}

	f.provider.mu.Lock()
	delete(f.provider.tenantClients, tenantID)
	f.provider.mu.Unlock()

	return nil
}

func (f *FirebaseTenantManager) GetTenant(ctx context.Context, tenantID string) (*auth.Tenant, error) {
	fbTenant, err := f.client.TenantManager.Tenant(ctx, tenantID)
	if err != nil {
		log.Err(err).Msg("Failed to get tenant")
		return nil, &auth.AuthError{
			Code:    auth.ErrorCodeTenantNotFound,
			Message: "failed to get tenant",
		}
	}

	return &auth.Tenant{
		ID:                    fbTenant.ID,
		DisplayName:           fbTenant.DisplayName,
		EnableEmailLinkSignIn: false,
		AllowPasswordSignUp:   true,
	}, nil
}

func (f *FirebaseTenantManager) AuthForTenant(ctx context.Context, tenantID string) (auth.AuthClient, error) {
	return f.provider.GetAuthClientForTenant(ctx, tenantID)
}

// Helper functions

func convertFirebaseUserRecord(fbUser *fbauth.UserRecord) *auth.UserRecord {
	return &auth.UserRecord{
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

func convertToFirebaseActionCodeSettings(settings *auth.ActionCodeSettings) *fbauth.ActionCodeSettings {
	if settings == nil {
		return nil
	}
	return &fbauth.ActionCodeSettings{
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

	if fbauth.IsUserNotFound(err) {
		return &auth.AuthError{
			Code:    auth.ErrorCodeUserNotFound,
			Message: "user not found",
		}
	}
	if fbauth.IsEmailAlreadyExists(err) {
		return &auth.AuthError{
			Code:    auth.ErrorCodeEmailAlreadyExists,
			Message: "email already exists",
		}
	}
	log.Err(err).Msg("Firebase authentication error")
	return &auth.AuthError{
		Code:    "unknown",
		Message: "authentication error",
	}
}

// initializeFirebaseClient initializes Firebase client from FIREBASE_CONFIG environment variable
func initializeFirebaseClient(ctx context.Context) (*fbauth.Client, error) {
	firebaseConfig := os.Getenv("FIREBASE_CONFIG")
	if firebaseConfig == "" {
		return nil, fmt.Errorf("FIREBASE_CONFIG environment variable is not set")
	}

	// Initialize Firebase app with credentials from JSON string
	opt := option.WithCredentialsJSON([]byte(firebaseConfig))
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return nil, fmt.Errorf("error initializing firebase app: %w", err)
	}

	// Get Auth client
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting firebase auth client: %w", err)
	}

	return client, nil
}
