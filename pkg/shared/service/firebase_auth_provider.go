package service

import (
	"context"
	"errors"
	"os"
	"strings"

	"ctoup.com/coreapp/pkg/shared/util"
	firebase "firebase.google.com/go"
	"firebase.google.com/go/auth"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/option"
)

const (
	FIREBASE_CLAIM_EMAIL          = "email"
	FIREBASE_CLAIM_EMAIL_VERIFIED = "email_verified"
	FIREBASE_CLAIM_USER_ID        = "user_id"
)

// FirebaseAuthProvider implements AuthProvider for Firebase Authentication
type FirebaseAuthProvider struct {
	tenantClientPool   *FirebaseTenantClientConnectionPool
	multitenantService *MultitenantService
}

// NewFirebaseAuthProvider creates a new Firebase authentication provider
func NewFirebaseAuthProvider(
	tenantClientPool *FirebaseTenantClientConnectionPool,
	multitenantService *MultitenantService,
) *FirebaseAuthProvider {
	return &FirebaseAuthProvider{
		tenantClientPool:   tenantClientPool,
		multitenantService: multitenantService,
	}
}

// GetProviderName returns the provider name
func (f *FirebaseAuthProvider) GetProviderName() string {
	return "firebase"
}

// VerifyToken verifies Firebase ID token and returns authenticated user
func (f *FirebaseAuthProvider) VerifyToken(c *gin.Context) (*AuthenticatedUser, error) {
	// Extract token from Authorization header or Token header
	authHeader := c.Request.Header.Get("Authorization")
	token := strings.Replace(authHeader, "Bearer ", "", 1)

	if token == "" {
		token = c.Request.Header.Get("Token")
	}

	if token == "" {
		return nil, errors.New("missing token")
	}

	// Get subdomain for tenant-aware authentication
	subdomain, err := util.GetSubdomain(c)
	if err != nil {
		log.Error().Err(err).Msg("failed to get subdomain")
		return nil, err
	}

	// Get tenant-specific auth client
	authClient, err := f.tenantClientPool.GetBaseAuthClient(c, subdomain)
	if err != nil {
		log.Error().Err(err).Msg("failed to get auth client")
		return nil, err
	}

	// Verify the ID token
	idToken, err := authClient.VerifyIDToken(context.Background(), token)
	if err != nil {
		log.Error().Err(err).Msg("failed to verify id token")
		return nil, err
	}

	// Extract user information
	email, _ := idToken.Claims[FIREBASE_CLAIM_EMAIL].(string)
	emailVerified, _ := idToken.Claims[FIREBASE_CLAIM_EMAIL_VERIFIED].(bool)
	userID, _ := idToken.Claims[FIREBASE_CLAIM_USER_ID].(string)

	// Extract custom claims (uppercase only)
	customClaims := util.FilterMapToArray(idToken.Claims, util.UppercaseOnly)

	return &AuthenticatedUser{
		UserID:        userID,
		Email:         email,
		EmailVerified: emailVerified,
		Claims:        idToken.Claims,
		CustomClaims:  customClaims,
	}, nil
}

// GetAuthClientForTenant returns a tenant-specific Firebase auth client
func (f *FirebaseAuthProvider) GetAuthClientForTenant(ctx context.Context, subdomain string) (interface{}, error) {
	return f.tenantClientPool.GetBaseAuthClient(ctx, subdomain)
}

// newFirebaseClient creates a new Firebase auth client
// This is used by FirebaseTenantClientConnectionPool
func newFirebaseClient(ctx context.Context) (*auth.Client, error) {
	fcfg := os.Getenv("FIREBASE_CONFIG")

	if fcfg == "" || fcfg == "default" {
		log.Fatal().Msg("missing FIREBASE_CONFIG environment variable or firebase-config secret in vault")
	}

	opt := option.WithCredentialsJSON([]byte(fcfg))
	app, err := firebase.NewApp(context.Background(), nil, opt)

	if err != nil {
		return nil, err
	}
	return app.Auth(ctx)
}
