package auth

import (
	"context"
	"sync"
	"time"

	"ctoup.com/coreapp/pkg/shared/util"
	"github.com/gin-gonic/gin"
)

const (
	// Context keys for authenticated user info
	AUTH_EMAIL              = "auth_email"
	AUTH_USER_ID            = "auth_user_id"
	AUTH_CLAIMS             = "auth_claims"
	AUTH_TENANT_ID_KEY      = "auth_tenant_id"
	AUTH_TENANT_MEMBERSHIPS = "tenant_memberships"
)

// TenantMembership represents a user's membership in a tenant with roles
type TenantMembership struct {
	TenantID string   `json:"tenant_id"`
	Roles    []string `json:"roles"`
}

// AuthenticatedUser represents the user info extracted from a token
type AuthenticatedUser struct {
	UserID            string                 `json:"user_id"`
	Email             string                 `json:"email"`
	EmailVerified     bool                   `json:"email_verified"`
	Claims            map[string]interface{} `json:"claims"`
	TenantID          string                 `json:"tenant_id,omitempty"`
	TenantMemberships []TenantMembership     `json:"tenant_memberships,omitempty"` // List of tenant memberships with roles
}

func (au *AuthenticatedUser) GetClaimsArray() []string {
	customClaims := util.FilterMapToArray(au.Claims, util.UppercaseOnly)
	return customClaims
}

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

// MultitenantService interface for getting tenant information
type MultitenantService interface {
	GetTenantIDWithSubdomain(ctx context.Context, subdomain string) (string, error)
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

	// BuildGlobalRoleClaims creates a provider-specific claims map for global roles
	// Firebase: {"SUPER_ADMIN": true, "ADMIN": true}
	// Kratos: {"global_roles": ["SUPER_ADMIN", "ADMIN"]}
	BuildGlobalRoleClaims(roles []string) map[string]interface{}

	// Email Actions
	EmailVerificationLink(ctx context.Context, email string) (string, error)
	PasswordResetLink(ctx context.Context, email string) (string, error)
	EmailVerificationLinkWithSettings(ctx context.Context, email string, settings *ActionCodeSettings) (string, error)
	PasswordResetLinkWithSettings(ctx context.Context, email string, settings *ActionCodeSettings) (string, error)
	EmailSignInLink(ctx context.Context, email string, settings *ActionCodeSettings) (string, error)

	// Token Verification
	VerifyIDToken(ctx context.Context, idToken string) (*Token, error)

	// Provider Capabilities
	// RequiresRecoveryProxy returns true if the provider needs a backend proxy endpoint
	// for password recovery (like Kratos), false if recovery links work directly (like Firebase)
	RequiresRecoveryProxy() bool
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
	Subdomain             string
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
	// Token Verification (Middleware use)
	VerifyToken(c *gin.Context) (*AuthenticatedUser, error)

	VerifyTokenWithTenantID(ctx context.Context, subdomain string, token string) (*AuthenticatedUser, error)

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

// AuthClientPool is an alias for AuthProvider to clarify its role in services
type AuthClientPool = AuthProvider

var (
	providersMu sync.RWMutex
	providers   = make(map[ProviderType]func(ctx context.Context, config ProviderConfig) (AuthProvider, error))
)

// RegisterProvider registers a provider factory function
func RegisterProvider(providerType ProviderType, factory func(ctx context.Context, config ProviderConfig) (AuthProvider, error)) {
	providersMu.Lock()
	defer providersMu.Unlock()
	providers[providerType] = factory
}

// GetProviderFactory returns a provider factory function
func GetProviderFactory(providerType ProviderType) (func(ctx context.Context, config ProviderConfig) (AuthProvider, error), bool) {
	providersMu.RLock()
	defer providersMu.RUnlock()
	f, ok := providers[providerType]
	return f, ok
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
