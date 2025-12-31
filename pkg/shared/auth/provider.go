package auth

import (
	"context"
	"time"
)

// UserRecord represents a user in the authentication system
type UserRecord struct {
	UID           string
	Email         string
	EmailVerified bool
	DisplayName   string
	PhotoURL      string
	Disabled      bool
	CreatedAt     time.Time
	CustomClaims  map[string]interface{}
}

// UserToCreate represents parameters for creating a new user
type UserToCreate struct {
	email         string
	emailVerified bool
	displayName   string
	photoURL      string
	disabled      bool
	password      *string
}

func (u *UserToCreate) Email(email string) *UserToCreate {
	u.email = email
	return u
}

func (u *UserToCreate) EmailVerified(verified bool) *UserToCreate {
	u.emailVerified = verified
	return u
}

func (u *UserToCreate) DisplayName(name string) *UserToCreate {
	u.displayName = name
	return u
}

func (u *UserToCreate) PhotoURL(url string) *UserToCreate {
	u.photoURL = url
	return u
}

func (u *UserToCreate) Disabled(disabled bool) *UserToCreate {
	u.disabled = disabled
	return u
}

func (u *UserToCreate) Password(password string) *UserToCreate {
	u.password = &password
	return u
}

func (u *UserToCreate) GetEmail() string       { return u.email }
func (u *UserToCreate) GetEmailVerified() bool { return u.emailVerified }
func (u *UserToCreate) GetDisplayName() string { return u.displayName }
func (u *UserToCreate) GetPhotoURL() string    { return u.photoURL }
func (u *UserToCreate) GetDisabled() bool      { return u.disabled }
func (u *UserToCreate) GetPassword() *string   { return u.password }

// UserToUpdate represents parameters for updating an existing user
type UserToUpdate struct {
	email         *string
	emailVerified *bool
	displayName   *string
	photoURL      *string
	disabled      *bool
	password      *string
}

func (u *UserToUpdate) Email(email string) *UserToUpdate {
	u.email = &email
	return u
}

func (u *UserToUpdate) EmailVerified(verified bool) *UserToUpdate {
	u.emailVerified = &verified
	return u
}

func (u *UserToUpdate) DisplayName(name string) *UserToUpdate {
	u.displayName = &name
	return u
}

func (u *UserToUpdate) PhotoURL(url string) *UserToUpdate {
	u.photoURL = &url
	return u
}

func (u *UserToUpdate) Disabled(disabled bool) *UserToUpdate {
	u.disabled = &disabled
	return u
}

func (u *UserToUpdate) Password(password string) *UserToUpdate {
	u.password = &password
	return u
}

func (u *UserToUpdate) GetEmail() *string       { return u.email }
func (u *UserToUpdate) GetEmailVerified() *bool { return u.emailVerified }
func (u *UserToUpdate) GetDisplayName() *string { return u.displayName }
func (u *UserToUpdate) GetPhotoURL() *string    { return u.photoURL }
func (u *UserToUpdate) GetDisabled() *bool      { return u.disabled }
func (u *UserToUpdate) GetPassword() *string    { return u.password }

// ActionCodeSettings represents settings for email action links
type ActionCodeSettings struct {
	URL                   string
	HandleCodeInApp       bool
	DynamicLinkDomain     string
	IOSBundleID           string
	AndroidPackageName    string
	AndroidInstallApp     bool
	AndroidMinimumVersion string
}

// Token represents an authentication token
type Token struct {
	UID    string
	Claims map[string]interface{}
}

// AuthClient defines the interface for authentication operations
// This abstraction allows swapping between Firebase, Ory/Kratos, or other providers
type AuthClient interface {
	// User Management
	CreateUser(ctx context.Context, user *UserToCreate) (*UserRecord, error)
	UpdateUser(ctx context.Context, uid string, user *UserToUpdate) (*UserRecord, error)
	DeleteUser(ctx context.Context, uid string) error
	GetUser(ctx context.Context, uid string) (*UserRecord, error)
	GetUserByEmail(ctx context.Context, email string) (*UserRecord, error)

	// Custom Claims (Roles/Permissions)
	SetCustomUserClaims(ctx context.Context, uid string, customClaims map[string]interface{}) error

	// Email Actions
	EmailVerificationLink(ctx context.Context, email string) (string, error)
	PasswordResetLink(ctx context.Context, email string) (string, error)
	EmailVerificationLinkWithSettings(ctx context.Context, email string, settings *ActionCodeSettings) (string, error)
	PasswordResetLinkWithSettings(ctx context.Context, email string, settings *ActionCodeSettings) (string, error)
	EmailSignInLink(ctx context.Context, email string, settings *ActionCodeSettings) (string, error)

	// Token Verification
	VerifyIDToken(ctx context.Context, idToken string) (*Token, error)
}

// TenantManager defines the interface for multi-tenant authentication management
type TenantManager interface {
	// Tenant Operations
	CreateTenant(ctx context.Context, config *TenantConfig) (*Tenant, error)
	UpdateTenant(ctx context.Context, tenantID string, config *TenantConfig) (*Tenant, error)
	DeleteTenant(ctx context.Context, tenantID string) error
	GetTenant(ctx context.Context, tenantID string) (*Tenant, error)

	// Get tenant-specific auth client
	AuthForTenant(ctx context.Context, tenantID string) (AuthClient, error)
}

// TenantConfig represents configuration for a tenant
type TenantConfig struct {
	DisplayName           string
	EnableEmailLinkSignIn bool
	AllowPasswordSignUp   bool
	EnableAnonymousUsers  bool
	MultiFactorConfig     *MultiFactorConfig
}

// MultiFactorConfig represents multi-factor authentication configuration
type MultiFactorConfig struct {
	State string // ENABLED or DISABLED
}

// Tenant represents a tenant in the authentication system
type Tenant struct {
	ID                    string
	DisplayName           string
	EnableEmailLinkSignIn bool
	AllowPasswordSignUp   bool
}

// AuthProvider defines the top-level interface for authentication providers
// Implementations: FirebaseAuthProvider, KratosAuthProvider
type AuthProvider interface {
	// Get the base auth client (for non-tenant operations)
	GetAuthClient() AuthClient

	// Get tenant manager for multi-tenant operations
	GetTenantManager() TenantManager

	// Get tenant-specific auth client by subdomain
	GetAuthClientForSubdomain(ctx context.Context, subdomain string) (AuthClient, error)

	// Get tenant-specific auth client by tenant ID
	GetAuthClientForTenant(ctx context.Context, tenantID string) (AuthClient, error)

	// Provider metadata
	GetProviderName() string
}

// AuthProviderFactory creates auth providers based on configuration
type AuthProviderFactory interface {
	CreateProvider(ctx context.Context, config ProviderConfig) (AuthProvider, error)
}

// ProviderConfig represents configuration for creating an auth provider
type ProviderConfig struct {
	Type        ProviderType
	Credentials interface{} // Provider-specific credentials
	Options     map[string]interface{}
}

// ProviderType represents the type of authentication provider
type ProviderType string

const (
	ProviderTypeFirebase ProviderType = "firebase"
	ProviderTypeKratos   ProviderType = "kratos"
)

// Common error types
type AuthError struct {
	Code    string
	Message string
	Err     error
}

func (e *AuthError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *AuthError) Unwrap() error {
	return e.Err
}

// Error codes
const (
	ErrorCodeUserNotFound       = "user-not-found"
	ErrorCodeEmailAlreadyExists = "email-already-exists"
	ErrorCodeInvalidPassword    = "invalid-password"
	ErrorCodeInvalidToken       = "invalid-token"
	ErrorCodeTenantNotFound     = "tenant-not-found"
)

// Helper functions for error checking
func IsUserNotFound(err error) bool {
	if authErr, ok := err.(*AuthError); ok {
		return authErr.Code == ErrorCodeUserNotFound
	}
	return false
}

func IsEmailAlreadyExists(err error) bool {
	if authErr, ok := err.(*AuthError); ok {
		return authErr.Code == ErrorCodeEmailAlreadyExists
	}
	return false
}
