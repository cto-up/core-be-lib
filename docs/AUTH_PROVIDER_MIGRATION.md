# Authentication Provider Migration Guide

## Overview

The authentication system has been refactored using the **Strategy Pattern** to allow easy swapping between different authentication providers (Firebase, Ory Kratos, or custom implementations).

## What Changed

### Removed Files

- `firebase_auth_middleware.go` - Replaced by provider pattern
- `ws_firebase_auth_middleware.go` - Renamed to `ws_auth_middleware.go`

### New Files

- `auth_provider.go` - Core interface
- `firebase_auth_provider.go` - Firebase implementation
- `kratos_auth_provider.go` - Kratos placeholder
- `auth_provider_factory.go` - Factory pattern
- `ws_auth_middleware.go` - Generic WebSocket auth

### Modified Files

- `auth_middleware.go` - Now uses AuthProvider interface
- `server_config.go` - Simplified, removed deprecated fields

## Architecture

### Core Components

1. **AuthProvider Interface** - Defines contract for all auth providers
2. **AuthenticatedUser** - Unified user representation
3. **FirebaseAuthProvider** - Firebase implementation
4. **KratosAuthProvider** - Ory Kratos placeholder
5. **AuthMiddleware** - Works with any provider
6. **WSAuthMiddleware** - WebSocket auth with any provider

## Usage

### Firebase (Default)

```go
firebaseProvider := service.NewFirebaseAuthProvider(
    firebaseTenantClientPool,
    multiTenantService,
)
authMiddleware := service.NewAuthMiddleware(firebaseProvider, clientAppService)
```

### Ory Kratos

```go
kratosProvider := service.NewKratosAuthProvider(os.Getenv("KRATOS_URL"))
authMiddleware := service.NewAuthMiddleware(kratosProvider, clientAppService)
```

### Custom Provider

```go
type MyAuthProvider struct{}

func (p *MyAuthProvider) GetProviderName() string { return "custom" }
func (p *MyAuthProvider) VerifyToken(c *gin.Context) (*service.AuthenticatedUser, error) {
    // Your implementation
}

customProvider := &MyAuthProvider{}
authMiddleware := service.NewAuthMiddleware(customProvider, clientAppService)
```

## Benefits

- Clean separation of concerns
- Easy to test with mock providers
- Swap providers without changing middleware code
- No deprecated Firebase-specific code
