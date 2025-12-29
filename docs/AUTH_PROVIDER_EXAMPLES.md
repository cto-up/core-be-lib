# Authentication Provider Examples

## Example 1: Using Firebase (Default)

```go
package main

import (
    "ctoup.com/coreapp/pkg/shared/service"
)

func setupAuth() {
    // Create Firebase provider
    firebaseProvider := service.NewFirebaseAuthProvider(
        firebaseTenantClientPool,
        multiTenantService,
    )

    // Create auth middleware
    authMiddleware := service.NewAuthMiddleware(
        firebaseProvider,
        clientAppService,
    )

    // Use in routes
    router.Use(authMiddleware.MiddlewareFunc())
}
```

## Example 2: Using Ory Kratos

```go
package main

import (
    "os"
    "ctoup.com/coreapp/pkg/shared/service"
)

func setupAuth() {
    // Create Kratos provider
    kratosProvider := service.NewKratosAuthProvider(
        os.Getenv("KRATOS_URL"),
    )

    // Create auth middleware
    authMiddleware := service.NewAuthMiddleware(
        kratosProvider,
        clientAppService,
    )

    // Use in routes
    router.Use(authMiddleware.MiddlewareFunc())
}
```

## Example 3: Using Factory Pattern

```go
package main

import (
    "ctoup.com/coreapp/pkg/shared/service"
)

func setupAuth() {
    // Create factory
    factory := service.NewAuthProviderFactory(
        firebaseTenantClientPool,
        multiTenantService,
    )

    // Create provider based on AUTH_PROVIDER env var
    // Defaults to Firebase if not set
    authProvider := factory.CreateProviderFromEnv()

    // Create auth middleware
    authMiddleware := service.NewAuthMiddleware(
        authProvider,
        clientAppService,
    )

    // Use in routes
    router.Use(authMiddleware.MiddlewareFunc())
}
```

## Example 4: Custom Auth Provider

```go
package myauth

import (
    "errors"
    "github.com/gin-gonic/gin"
    "ctoup.com/coreapp/pkg/shared/service"
)

type CustomAuthProvider struct {
    apiKey string
}

func NewCustomAuthProvider(apiKey string) *CustomAuthProvider {
    return &CustomAuthProvider{apiKey: apiKey}
}

func (p *CustomAuthProvider) GetProviderName() string {
    return "custom"
}

func (p *CustomAuthProvider) VerifyToken(c *gin.Context) (*service.AuthenticatedUser, error) {
    token := c.GetHeader("X-Custom-Token")

    if token == "" {
        return nil, errors.New("missing token")
    }

    // Your custom verification logic here
    // ...

    return &service.AuthenticatedUser{
        UserID:        "user123",
        Email:         "user@example.com",
        EmailVerified: true,
        Claims: map[string]interface{}{
            "ADMIN": true,
        },
        CustomClaims: []string{"ADMIN"},
    }, nil
}

// Usage
func setupAuth() {
    customProvider := NewCustomAuthProvider("my-secret-key")
    authMiddleware := service.NewAuthMiddleware(
        customProvider,
        clientAppService,
    )
    router.Use(authMiddleware.MiddlewareFunc())
}
```

## Example 5: WebSocket Authentication

```go
package main

import (
    "ctoup.com/coreapp/pkg/shared/service"
)

func setupWebSocketAuth() {
    // Create any auth provider
    authProvider := service.NewFirebaseAuthProvider(
        firebaseTenantClientPool,
        multiTenantService,
    )

    // Create WebSocket middleware
    wsAuthMiddleware := service.NewWSAuthMiddleware(authProvider)

    // Use in WebSocket routes
    router.GET("/ws", wsAuthMiddleware.MiddlewareFunc(), handleWebSocket)
}
```

## Example 6: Accessing Authenticated User

```go
package handlers

import (
    "github.com/gin-gonic/gin"
    "ctoup.com/coreapp/pkg/shared/service"
)

func MyHandler(c *gin.Context) {
    // Get authenticated user from context
    user := service.GetAuthenticatedUser(c)

    // Access user properties
    userID := user.UserID
    email := user.Email
    isAdmin := user.Claims["ADMIN"] == true

    // Use custom claims
    for _, claim := range user.CustomClaims {
        // Process custom claims
    }
}
```

## Example 7: Testing with Mock Provider

```go
package handlers_test

import (
    "testing"
    "github.com/gin-gonic/gin"
    "ctoup.com/coreapp/pkg/shared/service"
)

type MockAuthProvider struct{}

func (m *MockAuthProvider) GetProviderName() string {
    return "mock"
}

func (m *MockAuthProvider) VerifyToken(c *gin.Context) (*service.AuthenticatedUser, error) {
    return &service.AuthenticatedUser{
        UserID: "test-user",
        Email:  "test@example.com",
        Claims: map[string]interface{}{"ADMIN": true},
    }, nil
}

func TestMyHandler(t *testing.T) {
    mockProvider := &MockAuthProvider{}
    authMiddleware := service.NewAuthMiddleware(mockProvider, nil)

    router := gin.New()
    router.Use(authMiddleware.MiddlewareFunc())
    router.GET("/test", MyHandler)

    // Test your handler
}
```
