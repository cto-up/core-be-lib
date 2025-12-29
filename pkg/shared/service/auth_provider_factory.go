package service

import (
	"os"

	"github.com/rs/zerolog/log"
)

// AuthProviderType represents the type of authentication provider
type AuthProviderType string

const (
	AuthProviderFirebase AuthProviderType = "firebase"
	AuthProviderKratos   AuthProviderType = "kratos"
)

// AuthProviderFactory creates authentication providers based on configuration
type AuthProviderFactory struct {
	firebaseTenantPool *FirebaseTenantClientConnectionPool
	multitenantService *MultitenantService
	kratosURL          string
}

// NewAuthProviderFactory creates a new auth provider factory
func NewAuthProviderFactory(
	firebaseTenantPool *FirebaseTenantClientConnectionPool,
	multitenantService *MultitenantService,
) *AuthProviderFactory {
	return &AuthProviderFactory{
		firebaseTenantPool: firebaseTenantPool,
		multitenantService: multitenantService,
		kratosURL:          os.Getenv("KRATOS_URL"),
	}
}

// CreateProvider creates an auth provider based on the specified type
func (f *AuthProviderFactory) CreateProvider(providerType AuthProviderType) AuthProvider {
	switch providerType {
	case AuthProviderFirebase:
		return NewFirebaseAuthProvider(f.firebaseTenantPool, f.multitenantService)
	case AuthProviderKratos:
		if f.kratosURL == "" {
			log.Fatal().Msg("KRATOS_URL environment variable is required for Kratos provider")
		}
		return NewKratosAuthProvider(f.kratosURL)
	default:
		log.Fatal().Str("provider", string(providerType)).Msg("unknown auth provider type")
		return nil
	}
}

// CreateProviderFromEnv creates an auth provider based on environment configuration
func (f *AuthProviderFactory) CreateProviderFromEnv() AuthProvider {
	providerType := os.Getenv("AUTH_PROVIDER")
	if providerType == "" {
		providerType = "firebase" // Default to Firebase
	}

	log.Info().Str("provider", providerType).Msg("initializing auth provider")
	return f.CreateProvider(AuthProviderType(providerType))
}
