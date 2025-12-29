package service

import (
	"context"

	"github.com/gin-gonic/gin"
)

// AuthProvider defines the interface for authentication providers
type AuthProvider interface {
	// VerifyToken verifies the authentication token and returns user info
	VerifyToken(c *gin.Context) (*AuthenticatedUser, error)

	// GetProviderName returns the name of the auth provider
	GetProviderName() string
}

// AuthenticatedUser represents a verified user from any auth provider
type AuthenticatedUser struct {
	UserID        string
	Email         string
	EmailVerified bool
	Claims        map[string]interface{}
	CustomClaims  []string
}

// TenantAwareAuthProvider extends AuthProvider with tenant support
type TenantAwareAuthProvider interface {
	AuthProvider

	// GetAuthClientForTenant returns a tenant-specific auth client
	GetAuthClientForTenant(ctx context.Context, subdomain string) (interface{}, error)
}
