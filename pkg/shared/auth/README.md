# Authentication Provider Abstraction

A flexible, provider-agnostic authentication layer that supports multiple authentication backends (Ory Kratos, etc.) using the Strategy design pattern.

## Features

- 🔄 **Auth Providers**: Kratos, or custom providers
- 🔌 **Plugin Architecture**: Easy to add new authentication providers
- 🏢 **Multi-tenant**: Built-in support for multi-tenant applications
- 🧪 **Testable**: Mock providers for easy unit testing
- ⚡ **Performance**: Connection pooling and caching for optimal performance

## Quick Start

### Installation

```bash
go get ctoup.com/coreapp/pkg/shared/auth
```

### Basic Usage

```go
import "ctoup.com/coreapp/pkg/shared/auth"

provider, err := auth.InitializeAuthProvider(ctx, multitenantService)
if err != nil {
    logger.Fatal(err)
}

// Get auth client for a subdomain
authClient, err := provider.GetAuthClientForSubdomain(ctx, "tenant-subdomain")
if err != nil {
    logger.Fatal(err)
}

// Create a user
user := (&auth.UserToCreate{}).
    Email("user@example.com").
    DisplayName("John Doe").
    Password("securepassword")

record, err := authClient.CreateUser(ctx, user)
```

## Configuration

### Environment Variables

```bash
# Kratos configuration (when using Kratos)
KRATOS_ADMIN_URL=http://localhost:4434
```

## Architecture

```
┌─────────────────────────────────────────┐
│         Application Layer               │
│    (Handlers, Services, Middleware)     │
└──────────────┬──────────────────────────┘
               │
               ▼
┌─────────────────────────────────────────┐
│       AuthProvider Interface            │
│  - GetAuthClient()                      │
│  - GetTenantManager()                   │
│  - GetAuthClientForSubdomain()          │
└─────────────────────────────┬───────────┘
                              │
                              ▼
                      ┌──────────────┐
                      │   Kratos     │
                      │   Provider   │
                      └──────────────┘
```

## Supported Providers

### Ory Kratos

- ✅ Full implementation
- ✅ Multi-tenant support
- ✅ Custom claims (roles)
- ✅ Webauth
- ✅ Authenticators
- ✅ Token verification

### Custom Providers

- 📝 Easy to implement via interfaces
- 📝 See [extending guide](../../../docs/AUTH_PROVIDER_ABSTRACTION.md#extending-with-new-providers)

## API Reference

### Core Interfaces

#### AuthProvider

```go
type AuthProvider interface {
    GetAuthClient() AuthClient
    GetTenantManager() TenantManager
    GetAuthClientForSubdomain(ctx context.Context, subdomain string) (AuthClient, error)
    GetAuthClientForTenant(ctx context.Context, tenantID string) (AuthClient, error)
    GetProviderName() string
}
```

#### AuthClient

```go
type AuthClient interface {
    // User Management
    CreateUser(ctx context.Context, user *UserToCreate) (*UserRecord, error)
    UpdateUser(ctx context.Context, uid string, user *UserToUpdate) (*UserRecord, error)
    DeleteUser(ctx context.Context, uid string) error
    GetUser(ctx context.Context, uid string) (*UserRecord, error)
    GetUserByEmail(ctx context.Context, email string) (*UserRecord, error)

    // Custom Claims
    SetCustomUserClaims(ctx context.Context, uid string, customClaims map[string]interface{}) error

    // Email Actions
    EmailVerificationLink(ctx context.Context, email string) (string, error)
    PasswordResetLink(ctx context.Context, email string) (string, error)

    // Token Verification
    VerifyIDToken(ctx context.Context, idToken string) (*Token, error)
}
```

#### TenantManager

```go
type TenantManager interface {
    CreateTenant(ctx context.Context, config *TenantConfig) (*Tenant, error)
    UpdateTenant(ctx context.Context, tenantID string, config *TenantConfig) (*Tenant, error)
    DeleteTenant(ctx context.Context, tenantID string) error
    GetTenant(ctx context.Context, tenantID string) (*Tenant, error)
    AuthForTenant(ctx context.Context, tenantID string) (AuthClient, error)
}
```

## Examples

### Create a User

```go
user := (&auth.UserToCreate{}).
    Email("user@example.com").
    DisplayName("John Doe").
    EmailVerified(false).
    Password("securepassword")

record, err := authClient.CreateUser(ctx, user)
```

### Update a User

```go
update := (&auth.UserToUpdate{}).
    DisplayName("Jane Doe").
    EmailVerified(true)

record, err := authClient.UpdateUser(ctx, "user-id", update)
```

### Set Custom Claims (Roles)

```go
claims := map[string]interface{}{
    "ADMIN": true,
    "USER":  true,
}

err := authClient.SetCustomUserClaims(ctx, "user-id", claims)
```

### Send Password Reset Email

```go
link, err := authClient.PasswordResetLink(ctx, "user@example.com")
```

### Verify ID Token

```go
token, err := authClient.VerifyIDToken(ctx, idToken)
if err != nil {
    // Invalid token
}
// Use token.UID and token.Claims
```

### Manage Tenants

```go
tenantManager := provider.GetTenantManager()

config := &auth.TenantConfig{
    DisplayName:           "Acme Corp",
    EnableEmailLinkSignIn: true,
    AllowPasswordSignUp:   true,
}

tenant, err := tenantManager.CreateTenant(ctx, config)
```

## Error Handling

```go
_, err := authClient.GetUser(ctx, "user-id")
if err != nil {
    if auth.IsUserNotFound(err) {
        // Handle user not found
    } else if auth.IsEmailAlreadyExists(err) {
        // Handle duplicate email
    } else if authErr, ok := err.(*auth.AuthError); ok {
        logger.Printf("Auth error: %s - %s", authErr.Code, authErr.Message)
    }
}
```

## Testing

### Mock Provider

```go
type MockAuthClient struct {
    mock.Mock
}

func (m *MockAuthClient) CreateUser(ctx context.Context, user *auth.UserToCreate) (*auth.UserRecord, error) {
    args := m.Called(ctx, user)
    return args.Get(0).(*auth.UserRecord), args.Error(1)
}

// Use in tests
mockClient := new(MockAuthClient)
mockClient.On("CreateUser", mock.Anything, mock.Anything).
    Return(&auth.UserRecord{UID: "test-uid"}, nil)
```

## Migration Guide

See [AUTH_HANDLER_MIGRATION_EXAMPLE.md](../../../docs/AUTH_HANDLER_MIGRATION_EXAMPLE.md) for detailed migration instructions.

## Performance

- **Connection Pooling**: Tenant clients are pooled and reused
- **Lazy Loading**: Clients are created on-demand
- **Caching**: Tenant configurations are cached
- **Concurrent Safe**: Thread-safe operations with mutex locks

## Security

- ✅ Secure credential handling
- ✅ Token verification on server-side
- ✅ Multi-tenant isolation
- ✅ Custom claims validation
- ✅ Email verification flows

## Contributing

To add a new authentication provider:

1. Implement the `AuthClient` interface
2. Implement the `AuthProvider` interface
3. Add to the factory in `factory.go`
4. Add tests
5. Update documentation

See [AUTH_PROVIDER_ABSTRACTION.md](../../../docs/AUTH_PROVIDER_ABSTRACTION.md#extending-with-new-providers) for details.

## Documentation

- [Architecture Overview](../../../docs/AUTH_PROVIDER_ABSTRACTION.md)
- [Migration Guide](../../../docs/AUTH_HANDLER_MIGRATION_EXAMPLE.md)
- [API Examples](./example_test.go)

## License

[Your License Here]

## Support

For issues and questions:

- 📧 Email: support@example.com
- 🐛 Issues: [GitHub Issues](https://github.com/yourorg/yourrepo/issues)
- 📖 Docs: [Full Documentation](../../../docs/)
