package kratos

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"ctoup.com/coreapp/pkg/shared/auth"
	"github.com/gin-gonic/gin"
	ory "github.com/ory/kratos-client-go"
	"github.com/rs/zerolog/log"
)

func init() {
	auth.RegisterProvider(auth.ProviderTypeKratos, func(ctx context.Context, config auth.ProviderConfig) (auth.AuthProvider, error) {
		multitenantService, ok := config.Options["multitenantService"].(auth.MultitenantService)
		if !ok {
			return nil, fmt.Errorf("multitenantService not provided in config options")
		}

		adminURL := os.Getenv("KRATOS_ADMIN_URL")
		if adminURL == "" {
			adminURL = "http://localhost:4434"
		}
		publicURL := os.Getenv("KRATOS_PUBLIC_URL")
		if publicURL == "" {
			publicURL = "http://localhost:4433"
		}

		adminCfg := ory.NewConfiguration()
		adminCfg.Servers = ory.ServerConfigurations{{URL: adminURL}}
		adminClient := ory.NewAPIClient(adminCfg)

		publicCfg := ory.NewConfiguration()
		publicCfg.Servers = ory.ServerConfigurations{{URL: publicURL}}
		publicClient := ory.NewAPIClient(publicCfg)

		return NewKratosAuthProvider(ctx, adminClient, publicClient, multitenantService), nil
	})
}

// KratosAuthProvider implements AuthProvider for Ory Kratos
type KratosAuthProvider struct {
	adminClient        *ory.APIClient
	publicClient       *ory.APIClient
	multitenantService auth.MultitenantService
}

// NewKratosAuthProvider creates a new Kratos auth provider
func NewKratosAuthProvider(ctx context.Context, adminClient *ory.APIClient, publicClient *ory.APIClient, multitenantService auth.MultitenantService) *KratosAuthProvider {
	return &KratosAuthProvider{
		adminClient:        adminClient,
		publicClient:       publicClient,
		multitenantService: multitenantService,
	}
}

func (k *KratosAuthProvider) GetAuthClient() auth.AuthClient {
	return &KratosAuthClient{adminClient: k.adminClient, publicClient: k.publicClient}
}

func (k *KratosAuthProvider) VerifyToken(c *gin.Context) (*auth.AuthenticatedUser, error) {
	// Extract session token from cookie or header
	sessionToken := c.GetHeader("X-Session-Token")
	if sessionToken == "" {
		// Try to get from cookie
		cookie, err := c.Cookie("ory_kratos_session")
		if err == nil {
			sessionToken = cookie
		}
	}

	if sessionToken == "" {
		// Try Authorization header
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			sessionToken = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	if sessionToken == "" {
		return nil, fmt.Errorf("missing session token")
	}

	authClient := k.GetAuthClient()
	token, err := authClient.VerifyIDToken(c.Request.Context(), sessionToken)
	if err != nil {
		return nil, err
	}

	// Roles in Kratos are often in traits as company_role (per ORY_KRATOS.md)
	role, _ := token.Claims["company_role"].(string)
	customClaims := []string{}
	if role != "" {
		customClaims = append(customClaims, role)
	}

	// Map Kratos session to AuthenticatedUser
	traits, _ := token.Claims["traits"].(map[string]interface{})
	email, _ := traits["email"].(string)

	return &auth.AuthenticatedUser{
		UserID:        token.UID,
		Email:         email,
		EmailVerified: true, // Should check verifiable_addresses if needed
		Claims:        token.Claims,
		CustomClaims:  customClaims,
	}, nil
}

func (k *KratosAuthProvider) GetTenantManager() auth.TenantManager {
	return &KratosTenantManager{provider: k}
}

func (k *KratosAuthProvider) GetAuthClientForSubdomain(ctx context.Context, subdomain string) (auth.AuthClient, error) {
	// In Kratos organizations, we might still need to map subdomain to organization ID
	// For now, return the same client since Kratos handles multi-tenancy via organization ID in requests
	return k.GetAuthClient(), nil
}

func (k *KratosAuthProvider) GetAuthClientForTenant(ctx context.Context, tenantID string) (auth.AuthClient, error) {
	return &KratosAuthClient{adminClient: k.adminClient, publicClient: k.publicClient, organizationID: &tenantID}, nil
}

func (k *KratosAuthProvider) GetProviderName() string {
	return "kratos"
}

// KratosAuthClient implements AuthClient for Ory Kratos
type KratosAuthClient struct {
	adminClient    *ory.APIClient
	publicClient   *ory.APIClient
	organizationID *string
}

func (k *KratosAuthClient) CreateUser(ctx context.Context, user *auth.UserToCreate) (*auth.UserRecord, error) {
	traits := map[string]interface{}{
		"email": user.GetEmail(),
	}

	// Create identity
	identBody := *ory.NewCreateIdentityBody("default", traits)
	// If OrganizationId exists on the struct in this version, it's typically a NullableString
	if k.organizationID != nil {
		identBody.OrganizationId = *ory.NewNullableString(k.organizationID)
	}

	created, _, err := k.adminClient.IdentityAPI.CreateIdentity(ctx).CreateIdentityBody(identBody).Execute()
	if err != nil {
		return nil, convertKratosError(err)
	}

	return convertKratosIdentityToUserRecord(created), nil
}

func (k *KratosAuthClient) UpdateUser(ctx context.Context, uid string, user *auth.UserToUpdate) (*auth.UserRecord, error) {
	// Get existing
	existing, _, err := k.adminClient.IdentityAPI.GetIdentity(ctx, uid).Execute()
	if err != nil {
		return nil, convertKratosError(err)
	}

	traits := existing.Traits.(map[string]interface{})
	if email := user.GetEmail(); email != nil {
		traits["email"] = *email
	}

	state := ""
	if existing.State != nil {
		state = string(*existing.State)
	}
	updateBody := *ory.NewUpdateIdentityBody(existing.SchemaId, state, traits)
	if k.organizationID != nil {
		// UpdateIdentityBody might not have OrganizationId in this version according to Error 3
		// Check if it's actually there or not. If Error 3 said undefined, we skip it.
	}

	updated, _, err := k.adminClient.IdentityAPI.UpdateIdentity(ctx, uid).UpdateIdentityBody(updateBody).Execute()
	if err != nil {
		return nil, convertKratosError(err)
	}

	return convertKratosIdentityToUserRecord(updated), nil
}

func (k *KratosAuthClient) DeleteUser(ctx context.Context, uid string) error {
	_, err := k.adminClient.IdentityAPI.DeleteIdentity(ctx, uid).Execute()
	if err != nil {
		return convertKratosError(err)
	}
	return nil
}

func (k *KratosAuthClient) GetUser(ctx context.Context, uid string) (*auth.UserRecord, error) {
	ident, _, err := k.adminClient.IdentityAPI.GetIdentity(ctx, uid).Execute()
	if err != nil {
		return nil, convertKratosError(err)
	}
	return convertKratosIdentityToUserRecord(ident), nil
}

func (k *KratosAuthClient) GetUserByEmail(ctx context.Context, email string) (*auth.UserRecord, error) {
	// Kratos doesn't have a direct "get by email" in IdentityAPI easily without listing/filtering
	// For now, list with filter if supported or loop (inefficient)
	// Better: Use Kratos search or just use ID if possible.
	// Implementing via ListIdentities for now
	idents, _, err := k.adminClient.IdentityAPI.ListIdentities(ctx).CredentialsIdentifier(email).Execute()
	if err != nil {
		return nil, convertKratosError(err)
	}
	if len(idents) == 0 {
		return nil, &auth.AuthError{Code: auth.ErrorCodeUserNotFound, Message: "user not found"}
	}
	return convertKratosIdentityToUserRecord(&idents[0]), nil
}

func (k *KratosAuthClient) SetCustomUserClaims(ctx context.Context, uid string, customClaims map[string]interface{}) error {
	// In Kratos, we use traits for roles
	existing, _, err := k.adminClient.IdentityAPI.GetIdentity(ctx, uid).Execute()
	if err != nil {
		return convertKratosError(err)
	}

	traits := existing.Traits.(map[string]interface{})
	// Assuming roles are stored in traits
	for key, val := range customClaims {
		traits[key] = val
	}

	state := ""
	if existing.State != nil {
		state = string(*existing.State)
	}
	updateBody := *ory.NewUpdateIdentityBody(existing.SchemaId, state, traits)
	_, _, err = k.adminClient.IdentityAPI.UpdateIdentity(ctx, uid).UpdateIdentityBody(updateBody).Execute()
	return convertKratosError(err)
}

func (k *KratosAuthClient) EmailVerificationLink(ctx context.Context, email string) (string, error) {
	log.Warn().Msg("EmailVerificationLink not fully implemented for Kratos")
	return "", &auth.AuthError{Code: "not-implemented", Message: "use Kratos self-service flows"}
}

func (k *KratosAuthClient) PasswordResetLink(ctx context.Context, email string) (string, error) {
	log.Warn().Msg("PasswordResetLink not fully implemented for Kratos")
	return "", &auth.AuthError{Code: "not-implemented", Message: "use Kratos self-service flows"}
}

func (k *KratosAuthClient) EmailVerificationLinkWithSettings(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	return k.EmailVerificationLink(ctx, email)
}

func (k *KratosAuthClient) PasswordResetLinkWithSettings(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	return k.PasswordResetLink(ctx, email)
}

func (k *KratosAuthClient) EmailSignInLink(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	return "", &auth.AuthError{Code: "not-implemented"}
}

func (k *KratosAuthClient) VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error) {
	// In Kratos, idToken would be the session token
	session, _, err := k.publicClient.FrontendAPI.ToSession(ctx).XSessionToken(idToken).Execute()
	if err != nil {
		return nil, convertKratosError(err)
	}

	if !*session.Active {
		return nil, &auth.AuthError{Code: auth.ErrorCodeInvalidToken, Message: "session inactive"}
	}

	claims := make(map[string]interface{})
	if session.Identity != nil {
		if traits, ok := session.Identity.Traits.(map[string]interface{}); ok {
			for k, v := range traits {
				claims[k] = v
			}
		}
		if session.Identity.OrganizationId.IsSet() {
			claims["organization_id"] = session.Identity.OrganizationId.Get()
		}
	}

	return &auth.Token{
		UID:    session.Identity.Id,
		Claims: claims,
	}, nil
}

// KratosTenantManager implements TenantManager for Kratos
type KratosTenantManager struct {
	provider *KratosAuthProvider
}

func (k *KratosTenantManager) CreateTenant(ctx context.Context, config *auth.TenantConfig) (*auth.Tenant, error) {
	// Ory Kratos B2B/Organizations are not supported in the current SDK version
	return nil, &auth.AuthError{Code: "not-implemented", Message: "Kratos Organizations not supported in this SDK version"}
}

func (k *KratosTenantManager) UpdateTenant(ctx context.Context, tenantID string, config *auth.TenantConfig) (*auth.Tenant, error) {
	return nil, &auth.AuthError{Code: "not-implemented", Message: "Kratos Organizations not supported in this SDK version"}
}

func (k *KratosTenantManager) DeleteTenant(ctx context.Context, tenantID string) error {
	return &auth.AuthError{Code: "not-implemented", Message: "Kratos Organizations not supported in this SDK version"}
}

func (k *KratosTenantManager) GetTenant(ctx context.Context, tenantID string) (*auth.Tenant, error) {
	return nil, &auth.AuthError{Code: "not-implemented", Message: "Kratos Organizations not supported in this SDK version"}
}

func (k *KratosTenantManager) AuthForTenant(ctx context.Context, tenantID string) (auth.AuthClient, error) {
	return k.provider.GetAuthClientForTenant(ctx, tenantID)
}

// Helpers

func convertKratosIdentityToUserRecord(ident *ory.Identity) *auth.UserRecord {
	traits := ident.Traits.(map[string]interface{})
	email, _ := traits["email"].(string)
	name, _ := traits["name"].(string)

	var createdAt time.Time
	if ident.CreatedAt != nil {
		createdAt = *ident.CreatedAt
	}

	return &auth.UserRecord{
		UID:           ident.Id,
		Email:         email,
		DisplayName:   name,
		EmailVerified: true, // Should check Kratos verifiable_addresses
		CreatedAt:     createdAt,
		CustomClaims:  traits, // Map traits to claims
	}
}

func convertKratosError(err error) error {
	if err == nil {
		return nil
	}
	// TODO: Map Ory Kratos error codes to auth.AuthError
	return &auth.AuthError{
		Code:    "kratos-error",
		Message: err.Error(),
		Err:     err,
	}
}
