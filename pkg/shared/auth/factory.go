package auth

import (
	"context"
	"fmt"
	"os"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/auth"
	"google.golang.org/api/option"
)

// DefaultAuthProviderFactory implements AuthProviderFactory
type DefaultAuthProviderFactory struct{}

func NewAuthProviderFactory() AuthProviderFactory {
	return &DefaultAuthProviderFactory{}
}

func (f *DefaultAuthProviderFactory) CreateProvider(ctx context.Context, config ProviderConfig) (AuthProvider, error) {
	switch config.Type {
	case ProviderTypeFirebase:
		return f.createFirebaseProvider(ctx, config)
	case ProviderTypeKratos:
		return f.createKratosProvider(ctx, config)
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", config.Type)
	}
}

func (f *DefaultAuthProviderFactory) createFirebaseProvider(ctx context.Context, config ProviderConfig) (AuthProvider, error) {
	// Get Firebase credentials
	var opts []option.ClientOption

	if creds, ok := config.Credentials.(string); ok && creds != "" {
		opts = append(opts, option.WithCredentialsFile(creds))
	} else if credsJSON := os.Getenv("FIREBASE_CREDENTIALS_JSON"); credsJSON != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(credsJSON)))
	}

	// Initialize Firebase app
	app, err := firebase.NewApp(ctx, nil, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Firebase app: %w", err)
	}

	// Get auth client
	authClient, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Firebase auth client: %w", err)
	}

	// Get multitenant service from config
	multitenantService, ok := config.Options["multitenantService"].(MultitenantService)
	if !ok {
		return nil, fmt.Errorf("multitenantService not provided in config options")
	}

	return NewFirebaseAuthProvider(ctx, authClient, multitenantService), nil
}

func (f *DefaultAuthProviderFactory) createKratosProvider(ctx context.Context, config ProviderConfig) (AuthProvider, error) {
	// Get Kratos client from config
	kratosClient, ok := config.Credentials.(KratosClient)
	if !ok {
		return nil, fmt.Errorf("invalid Kratos client credentials")
	}

	// Get multitenant service from config
	multitenantService, ok := config.Options["multitenantService"].(MultitenantService)
	if !ok {
		return nil, fmt.Errorf("multitenantService not provided in config options")
	}

	return NewKratosAuthProvider(ctx, kratosClient, multitenantService), nil
}

// Helper function to create provider from environment
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

	switch providerType {
	case "firebase":
		// Firebase credentials from env or file
		if credsFile := os.Getenv("FIREBASE_CREDENTIALS_FILE"); credsFile != "" {
			config.Credentials = credsFile
		}
	case "kratos":
		// Kratos client would be initialized here
		// This is a placeholder - you'd need to implement actual Kratos client initialization
		return nil, fmt.Errorf("Kratos provider initialization not fully implemented")
	default:
		return nil, fmt.Errorf("unsupported auth provider: %s", providerType)
	}

	return factory.CreateProvider(ctx, config)
}

// CreateAdapterFromProvider creates a backward-compatible adapter
func CreateAdapterFromProvider(provider AuthProvider, multitenantService MultitenantService) *AuthProviderAdapter {
	return NewAuthProviderAdapter(provider, multitenantService)
}

// InitializeAuthProvider is a convenience function to initialize the auth provider
// and return an adapter for backward compatibility
func InitializeAuthProvider(ctx context.Context, multitenantService MultitenantService) (*AuthProviderAdapter, error) {
	provider, err := CreateProviderFromEnv(ctx, multitenantService)
	if err != nil {
		return nil, err
	}

	return CreateAdapterFromProvider(provider, multitenantService), nil
}

// For direct Firebase initialization (backward compatibility)
func InitializeFirebaseProvider(ctx context.Context, authClient *auth.Client, multitenantService MultitenantService) *AuthProviderAdapter {
	provider := NewFirebaseAuthProvider(ctx, authClient, multitenantService)
	return CreateAdapterFromProvider(provider, multitenantService)
}
