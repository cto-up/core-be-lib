package auth

import (
	"context"
	"fmt"
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
	providerType := "kratos"

	factory := NewAuthProviderFactory()

	config := ProviderConfig{
		Type: ProviderType(providerType),
		Options: map[string]interface{}{
			"multitenantService": multitenantService,
		},
	}

	return factory.CreateProvider(ctx, config)
}

// InitializeAuthProvider is a convenience function to initialize the auth provider
func InitializeAuthProvider(ctx context.Context, multitenantService MultitenantService) (AuthProvider, error) {
	return CreateProviderFromEnv(ctx, multitenantService)
}
