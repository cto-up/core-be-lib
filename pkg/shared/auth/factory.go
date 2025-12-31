package auth

import (
	"context"
	"fmt"
	"os"
)

// DefaultAuthProviderFactory implements AuthProviderFactory
type DefaultAuthProviderFactory struct{}

func NewAuthProviderFactory() AuthProviderFactory {
	return &DefaultAuthProviderFactory{}
}

func (f *DefaultAuthProviderFactory) CreateProvider(ctx context.Context, config ProviderConfig) (AuthProvider, error) {
	factory, ok := GetProviderFactory(config.Type)
	if !ok {
		return nil, fmt.Errorf("unsupported provider type: %s", config.Type)
	}
	return factory(ctx, config)
}

// CreateProviderFromEnv creates an auth provider based on environment configuration
func CreateProviderFromEnv(ctx context.Context, multitenantService MultitenantService) (AuthProvider, error) {
	providerType := os.Getenv("AUTH_PROVIDER")
	if providerType == "" {
		providerType = "firebase" // Default to Firebase
	}

	factory := NewAuthProviderFactory()

	config := ProviderConfig{
		Type: ProviderType(providerType),
		Options: map[string]interface{}{
			"multitenantService": multitenantService,
		},
	}

	// For backward compatibility, the caller might need to handle credentials
	// In the new system, each provider's init() should handle its own defaults if possible,
	// or we pass credentials via config.Credentials.

	// If it's firebase, we might need the auth.Client if it's already initialized
	// But usually it's initialized by the factory.

	return factory.CreateProvider(ctx, config)
}

// InitializeAuthProvider is a convenience function to initialize the auth provider
func InitializeAuthProvider(ctx context.Context, multitenantService MultitenantService) (AuthProvider, error) {
	return CreateProviderFromEnv(ctx, multitenantService)
}
