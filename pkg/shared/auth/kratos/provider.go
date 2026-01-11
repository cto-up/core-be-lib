package kratos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"ctoup.com/coreapp/pkg/shared/auth"
	"github.com/gin-gonic/gin"
	ory "github.com/ory/kratos-client-go"
	"github.com/rs/zerolog/log"
)

// Global roles stored in Kratos metadata_public
var globalRoles = []string{"SUPER_ADMIN"}

// Tenant-specific roles (CUSTOMER_ADMIN, ADMIN, USER) are stored in core_user_tenant_memberships table
var tenantRoles = []string{"CUSTOMER_ADMIN", "ADMIN", "USER"}

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
	// Tenant
	tenantID := c.GetString(auth.AUTH_TENANT_ID_KEY)

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
	return k.VerifyTokenWithTenantID(c.Request.Context(), tenantID, sessionToken)
}

func (k *KratosAuthProvider) VerifyTokenWithTenantID(ctx context.Context, tenantID string, sessionToken string) (*auth.AuthenticatedUser, error) {
	authClient := k.GetAuthClient()

	token, err := authClient.VerifyIDToken(ctx, sessionToken)
	if err != nil {
		return nil, err
	}

	email, _ := token.Claims["email"].(string)

	customClaims := []string{}

	// Extract global roles from metadata_public
	if globalRolesArr, ok := token.Claims["global_roles"].([]interface{}); ok {
		for _, role := range globalRolesArr {
			if roleStr, ok := role.(string); ok {
				customClaims = append(customClaims, roleStr)
			}
		}
	}

	// if customClaims containts SUPER_ADMIN
	isSuperAdmin := false
	for _, claim := range customClaims {
		if claim == "SUPER_ADMIN" {
			isSuperAdmin = true
			break // Exit loop once found
		}
	}

	user := &auth.AuthenticatedUser{
		UserID:            token.UID,
		Email:             email,
		EmailVerified:     true, // Should check verifiable_addresses if needed
		Claims:            token.Claims,
		CustomClaims:      customClaims,
		TenantID:          tenantID,
		TenantMemberships: []auth.TenantMembership{},
	}

	// Skip tenant validation for root domain or SUPER_ADMIN
	if tenantID == "" {
		if isSuperAdmin {
			return user, nil
		} else {
			return user, fmt.Errorf("Only SUPER_ADMIN can access root domain")
		}
	}

	if membershipsInterface, ok := token.Claims[auth.AUTH_TENANT_MEMBERSHIPS].([]interface{}); ok {
		for _, m := range membershipsInterface {
			if membershipMap, ok := m.(map[string]interface{}); ok {
				membership := auth.TenantMembership{}
				if tid, ok := membershipMap["tenant_id"].(string); ok {
					membership.TenantID = tid
				}

				// Only validate for tenant
				if membership.TenantID == tenantID {
					if rolesInterface, ok := membershipMap["roles"].([]interface{}); ok {
						for _, r := range rolesInterface {
							if roleStr, ok := r.(string); ok {
								customClaims = append(customClaims, roleStr)
							}
						}
					}
					return user, nil
				}
			}
		}
	}

	return user, fmt.Errorf("User with userID %s, not allowed for tenantID %s .", user.UserID, tenantID)
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
	return &KratosAuthClient{adminClient: k.adminClient, publicClient: k.publicClient}, nil
}

func (k *KratosAuthProvider) GetProviderName() string {
	return "kratos"
}

// KratosAuthClient implements AuthClient for Ory Kratos
type KratosAuthClient struct {
	adminClient  *ory.APIClient
	publicClient *ory.APIClient
}

// GetAdminClient returns the admin API client
func (k *KratosAuthClient) GetAdminClient() *ory.APIClient {
	return k.adminClient
}

// GetPublicClient returns the public API client
func (k *KratosAuthClient) GetPublicClient() *ory.APIClient {
	return k.publicClient
}

func (k *KratosAuthClient) CreateUser(ctx context.Context, user *auth.UserToCreate) (*auth.UserRecord, error) {
	traits := map[string]interface{}{
		"email": user.GetEmail(),
	}

	// Create identity
	identBody := *ory.NewCreateIdentityBody("default", traits)

	if password := user.GetPassword(); password != nil {
		identBody.Credentials = &ory.IdentityWithCredentials{
			Password: &ory.IdentityWithCredentialsPassword{
				Config: &ory.IdentityWithCredentialsPasswordConfig{
					Password: password,
				},
			},
		}
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

	if password := user.GetPassword(); password != nil {
		// In Kratos, updating password via identity update requires credentials
		updateBody.Credentials = &ory.IdentityWithCredentials{
			Password: &ory.IdentityWithCredentialsPassword{
				Config: &ory.IdentityWithCredentialsPasswordConfig{
					Password: password,
				},
			},
		}
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
	existing, _, err := k.adminClient.IdentityAPI.GetIdentity(ctx, uid).Execute()
	if err != nil {
		return convertKratosError(err)
	}

	// Get existing traits (don't modify them for roles)
	traits, ok := existing.Traits.(map[string]interface{})
	if !ok {
		traits = make(map[string]interface{})
	}

	// 1. Prepare Metadata containers
	metadataPublic, ok := existing.MetadataPublic.(map[string]interface{})
	if !ok {
		metadataPublic = make(map[string]interface{})
	}

	// Ensure tenant_memberships exists in metadata
	rawMemberships, ok := metadataPublic["tenant_memberships"].([]interface{})
	if !ok {
		rawMemberships = []interface{}{}
	}

	// 2. Process the customClaims map directly
	// Handle global_roles
	if globalRoles, exists := customClaims["global_roles"]; exists {
		metadataPublic["global_roles"] = globalRoles
	}

	// Handle tenant_memberships (single membership passed as map)
	if tenantMembership, exists := customClaims["tenant_memberships"]; exists {
		newMembership, ok := tenantMembership.(map[string]interface{})
		if ok {
			newTenantID, hasTenantID := newMembership["tenant_id"].(string)
			if hasTenantID {
				// Filter out the existing membership for this specific tenant_id (Replace logic)
				updatedMemberships := []interface{}{}
				for _, m := range rawMemberships {
					mMap, isMap := m.(map[string]interface{})
					if isMap && mMap["tenant_id"] != newTenantID {
						updatedMemberships = append(updatedMemberships, m)
					}
				}
				// Add the new membership
				updatedMemberships = append(updatedMemberships, newMembership)
				rawMemberships = updatedMemberships
			}
		}
	}

	// 3. Save back to metadata
	metadataPublic["tenant_memberships"] = rawMemberships

	state := ""
	if existing.State != nil {
		state = string(*existing.State)
	}

	updateBody := *ory.NewUpdateIdentityBody(existing.SchemaId, state, traits)
	updateBody.MetadataPublic = metadataPublic

	_, _, err = k.adminClient.IdentityAPI.UpdateIdentity(ctx, uid).UpdateIdentityBody(updateBody).Execute()
	return convertKratosError(err)
}

func (k *KratosAuthClient) EmailVerificationLink(ctx context.Context, email string) (string, error) {
	// For Kratos, we need to use the Admin API to create verification links
	// The browser flow approach doesn't work from backend because it requires CSRF tokens

	// First, get the user by email to get their ID
	idents, _, err := k.adminClient.IdentityAPI.ListIdentities(ctx).CredentialsIdentifier(email).Execute()
	if err != nil {
		return "", convertKratosError(err)
	}
	if len(idents) == 0 {
		return "", &auth.AuthError{Code: auth.ErrorCodeUserNotFound, Message: "user not found"}
	}

	identityID := idents[0].Id

	// For email verification, we need to find the verifiable address
	var addressID string
	if len(idents[0].VerifiableAddresses) > 0 {
		for _, addr := range idents[0].VerifiableAddresses {
			if addr.Value == email && addr.Id != nil {
				addressID = *addr.Id
				break
			}
		}
	}

	if addressID == "" {
		return "", &auth.AuthError{Code: "address-not-found", Message: "verifiable address not found"}
	}

	// Create verification link using Admin API
	verificationLink, _, err := k.adminClient.IdentityAPI.CreateRecoveryLinkForIdentity(ctx).
		CreateRecoveryLinkForIdentityBody(ory.CreateRecoveryLinkForIdentityBody{
			IdentityId: identityID,
		}).
		Execute()

	if err != nil {
		return "", convertKratosError(err)
	}

	log.Info().Str("email", email).Str("identity_id", identityID).Msg("Verification link created via Admin API")
	return verificationLink.RecoveryLink, nil
}

func (k *KratosAuthClient) PasswordResetLink(ctx context.Context, email string) (string, error) {
	// For Kratos, we need to use the Admin API to create recovery links
	// The browser flow approach doesn't work from backend because it requires CSRF tokens

	// First, get the user by email to get their ID
	idents, _, err := k.adminClient.IdentityAPI.ListIdentities(ctx).CredentialsIdentifier(email).Execute()
	if err != nil {
		return "", convertKratosError(err)
	}
	if len(idents) == 0 {
		return "", &auth.AuthError{Code: auth.ErrorCodeUserNotFound, Message: "user not found"}
	}

	identityID := idents[0].Id

	// Create recovery link using Admin API
	recoveryLink, _, err := k.adminClient.IdentityAPI.CreateRecoveryLinkForIdentity(ctx).
		CreateRecoveryLinkForIdentityBody(ory.CreateRecoveryLinkForIdentityBody{
			IdentityId: identityID,
		}).
		Execute()

	if err != nil {
		return "", convertKratosError(err)
	}

	log.Info().Str("email", email).Str("identity_id", identityID).Str("recovery_link", recoveryLink.RecoveryLink).Msg("Recovery link created via Admin API")
	return recoveryLink.RecoveryLink, nil
}

func (k *KratosAuthClient) VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error) {
	// Construct the cookie string manually
	cookieString := fmt.Sprintf("ory_kratos_session=%s", idToken)

	// Use the SDK but inject the Cookie header into the context
	// This keeps your code clean and leverages the SDK's built-in types
	session, _, err := k.publicClient.FrontendAPI.ToSession(ctx).
		Cookie(cookieString). // The SDK has a .Cookie() method for this!
		Execute()

	if err != nil {
		return nil, convertKratosError(err)
	}

	if !*session.Active {
		return nil, &auth.AuthError{Code: auth.ErrorCodeInvalidToken, Message: "session inactive"}
	}

	claims := make(map[string]interface{})
	if session.Identity != nil {
		if traits, ok := session.Identity.Traits.(map[string]interface{}); ok {
			for key, v := range traits {
				claims[key] = v
			}
		}

		// Extract tenant and role information from metadata_public
		if metadataPublic, ok := session.Identity.MetadataPublic.(map[string]interface{}); ok {
			// Debug logging
			log.Debug().
				Str("identity_id", session.Identity.Id).
				Interface("metadata_public", metadataPublic).
				Msg("Processing metadata_public from Kratos session")

			// Add tenant_memberships to claims
			if tenantMemberships, ok := metadataPublic[auth.AUTH_TENANT_MEMBERSHIPS].([]interface{}); ok {
				claims[auth.AUTH_TENANT_MEMBERSHIPS] = tenantMemberships
			}

			// For backward compatibility, also set tenant_id and subdomain
			if tenantID, ok := metadataPublic["tenant_id"].(string); ok {
				claims["tenant_id"] = tenantID
			}

			// Extract global roles and flatten them as boolean claims for backward compatibility
			if globalRolesArr, ok := metadataPublic["global_roles"].([]interface{}); ok {
				claims["global_roles"] = globalRolesArr
				for _, role := range globalRolesArr {
					if roleStr, ok := role.(string); ok {
						claims[roleStr] = true // e.g., claims["SUPER_ADMIN"] = true
						log.Debug().
							Str("role", roleStr).
							Msg("Setting global role as boolean claim")
					}
				}
			} else {
				log.Debug().
					Str("identity_id", session.Identity.Id).
					Msg("No global_roles found in metadata_public")
			}
		} else {
			log.Debug().
				Str("identity_id", session.Identity.Id).
				Msg("No metadata_public found in session identity")
		}
	}

	return &auth.Token{
		UID:    session.Identity.Id,
		Claims: claims,
	}, nil
}

func (k *KratosAuthClient) EmailVerificationLinkWithSettings(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	return k.EmailVerificationLink(ctx, email)
}

func (k *KratosAuthClient) PasswordResetLinkWithSettings(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	// Get the Kratos recovery link
	kratosLink, err := k.PasswordResetLink(ctx, email)
	if err != nil {
		return "", err
	}

	// If settings with URL are provided, replace the Kratos base URL with the frontend URL
	// The Kratos link format is: http://localhost:4433/self-service/recovery?flow=xxx&token=yyy
	// For Kratos, we want to change it to point to our backend proxy which will handle the recovery flow:
	// http://subdomain.ctoup.localhost:5173/recovery?flow=xxx&token=yyy
	// The frontend will then call the backend proxy endpoint: /public-api/v1/auth/recovery?flow=xxx&token=yyy
	if settings != nil && settings.URL != "" {
		// Extract the query parameters from the Kratos link
		if strings.Contains(kratosLink, "?") {
			queryPart := kratosLink[strings.Index(kratosLink, "?")+1:]

			// Extract base URL from settings.URL (remove path and query string)
			// settings.URL might be: http://corpb.ctoup.localhost:5173/signin?from=/
			// We need: http://corpb.ctoup.localhost:5173
			baseURL := settings.URL
			if idx := strings.Index(baseURL, "?"); idx != -1 {
				baseURL = baseURL[:idx]
			}
			if idx := strings.LastIndex(baseURL, "/"); idx > 8 { // After http:// or https://
				baseURL = baseURL[:idx]
			}

			// Construct the frontend recovery URL
			// This URL will be opened by the user and will call our backend proxy
			frontendLink := fmt.Sprintf("%s/recovery?%s", baseURL, queryPart)
			log.Info().
				Str("kratos_link", kratosLink).
				Str("settings_url", settings.URL).
				Str("base_url", baseURL).
				Str("frontend_link", frontendLink).
				Str("email", email).
				Msg("Converted Kratos recovery link to frontend URL")
			return frontendLink, nil
		}
	}

	return kratosLink, nil
}

func (k *KratosAuthClient) EmailSignInLink(ctx context.Context, email string, settings *auth.ActionCodeSettings) (string, error) {
	return "", &auth.AuthError{Code: "not-implemented"}
}

// RequiresRecoveryProxy returns true for Kratos since it needs a backend proxy
// to activate recovery links and create settings flows
func (k *KratosAuthClient) RequiresRecoveryProxy() bool {
	return true
}

// KratosTenantManager implements TenantManager for Kratos
// Note: Kratos doesn't have built-in tenant management like Firebase.
// These methods are no-ops that return success to maintain interface compatibility.
// Actual tenant data is managed in the database (core_tenants table).
type KratosTenantManager struct {
	provider *KratosAuthProvider
}

func (k *KratosTenantManager) CreateTenant(ctx context.Context, config *auth.TenantConfig) (*auth.Tenant, error) {
	// Kratos doesn't manage tenants - they're database-only
	// Return a tenant with generated ID so the handler can proceed
	tenantID := fmt.Sprintf("%s-%d", config.Subdomain, time.Now().UnixNano())

	return &auth.Tenant{
		ID:                    tenantID,
		DisplayName:           config.DisplayName,
		EnableEmailLinkSignIn: config.EnableEmailLinkSignIn,
		AllowPasswordSignUp:   config.AllowPasswordSignUp,
	}, nil
}

func (k *KratosTenantManager) UpdateTenant(ctx context.Context, tenantID string, config *auth.TenantConfig) (*auth.Tenant, error) {
	// Kratos doesn't manage tenants - return success
	// The handler will update the database
	return &auth.Tenant{
		ID:                    tenantID,
		DisplayName:           config.DisplayName,
		EnableEmailLinkSignIn: config.EnableEmailLinkSignIn,
		AllowPasswordSignUp:   config.AllowPasswordSignUp,
	}, nil
}

func (k *KratosTenantManager) DeleteTenant(ctx context.Context, tenantID string) error {
	// Kratos doesn't manage tenants - return success
	// The handler will delete from database
	return nil
}

func (k *KratosTenantManager) GetTenant(ctx context.Context, tenantID string) (*auth.Tenant, error) {
	// Kratos doesn't manage tenants - return minimal tenant
	// The handler should get full details from database
	return &auth.Tenant{
		ID: tenantID,
	}, nil
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

	message := ""
	var apiErr *ory.GenericOpenAPIError

	if !errors.As(err, &apiErr) {
		message = err.Error()
		return &auth.AuthError{
			Code:    "unknown-error",
			Message: message,
			Err:     err,
		}
	}

	// HTTP status
	fmt.Println("HTTP:", apiErr.Error())

	// ✅ CALL Model() to get the error model
	model := apiErr.Model()
	if model == nil {
		fmt.Println("No model in error")
		return &auth.AuthError{
			Code:    "unknown-error",
			Message: err.Error(),
			Err:     err,
		}
	}

	// Kratos standard error payload
	if eg, ok := model.(ory.ErrorGeneric); ok {
		message = eg.Error.Message
		if eg.Error.Reason != nil {
			message += " reason: " + *eg.Error.Reason
		}

		return &auth.AuthError{
			Code:    "kratos-error",
			Message: message,
			Err:     err,
		}
	}

	return &auth.AuthError{
		Code:    "kratos-error",
		Message: err.Error(),
		Err:     err,
	}
}
