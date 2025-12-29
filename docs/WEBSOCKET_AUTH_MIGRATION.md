# WebSocket Authentication Migration Guide

## Overview

This guide shows how to migrate WebSocket authentication code from the old `FirebaseAuthMiddleware` to the new provider-based approach.

## Migration Examples

### Example 1: Standard WebSocket Auth (Header-based)

#### Before (Old Code)

```go
wsAuthMiddleWare := access.NewWSAuthMiddleware(serverConfig.FirebaseAuthMiddleware)
serverConfig.Router.GET("/ws/channel/:location/connections/:connectionId",
    serverConfig.TenantMiddleware.MiddlewareFunc(),
    wsAuthMiddleWare.MiddlewareFunc(),
    recruitmentWSHandler.Handler)
```

#### After (New Code)

```go
// Get the auth provider from server config (or create it)
authProvider := service.NewFirebaseAuthProvider(
    serverConfig.TenantClientPool,
    multitenantService,
)

// Create WebSocket auth middleware with the provider
wsAuthMiddleware := service.NewWSAuthMiddleware(authProvider)

serverConfig.Router.GET("/ws/channel/:location/connections/:connectionId",
    serverConfig.TenantMiddleware.MiddlewareFunc(),
    wsAuthMiddleware.MiddlewareFunc(),
    recruitmentWSHandler.Handler)
```

### Example 2: WebSocket Auth with Query Parameters (Mobile Apps)

#### Before (Old Code)

```go
careWSAuthMiddleware := func(c *gin.Context) {
    // Extract token from query parameter
    token := c.Query("token")
    if token == "" {
        log.Error().Msg("No token provided in query parameter")
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Token required"})
        c.Abort()
        return
    }

    // Extract subdomain for tenant resolution
    subdomain := c.Query("subdomain")
    if subdomain == "" {
        log.Error().Msg("No subdomain provided in query parameter")
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Subdomain required"})
        c.Abort()
        return
    }

    // Set token in header for Firebase verification
    c.Request.Header.Set("Authorization", "Bearer "+token)

    // Verify with Firebase
    _, failed := serverConfig.FirebaseAuthMiddleware.verifyToken(c)
    if failed {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Invalid token"})
        c.Abort()
        return
    }

    c.Next()
}

serverConfig.Router.GET("/ws/care/chat", careWSAuthMiddleware, careWSHandler.Handler)
```

#### After (New Code)

```go
// Get the auth provider
authProvider := service.NewFirebaseAuthProvider(
    serverConfig.TenantClientPool,
    multitenantService,
)

// Use the helper function for query parameter auth
careWSAuthMiddleware := service.NewWSAuthMiddlewareWithQueryParams(authProvider)

serverConfig.Router.GET("/ws/care/chat", careWSAuthMiddleware, careWSHandler.Handler)
```

### Example 3: WebSocket Auth with Header Fallback

For maximum flexibility (supports both header and query param auth):

```go
authProvider := service.NewFirebaseAuthProvider(
    serverConfig.TenantClientPool,
    multitenantService,
)

// This middleware tries header first, then falls back to query params
wsAuthMiddleware := service.NewWSAuthMiddlewareWithHeaderFallback(authProvider)

serverConfig.Router.GET("/ws/flexible", wsAuthMiddleware, handler)
```

## Complete Migration Example

### Before (Full Context)

```go
multitenantService := access.NewMultitenantService(coreStore)

pool, err := pgxpool.New(context.Background(), dbConnection)
if err != nil {
    log.Fatal().Err(err).Msg("Unable to create connection pool")
}
defer pool.Close()

store := recruitmentdb.NewStore(pool)
hub := recruitmentws.NewSignalingHub(pool, store, nc, natsStream)
go hub.Run(ctx)

recruitmentWSHandler := recruitmentws.NewWSHandler(hub, connPool)

// Old way - depends on serverConfig.FirebaseAuthMiddleware
wsAuthMiddleWare := access.NewWSAuthMiddleware(serverConfig.FirebaseAuthMiddleware)
serverConfig.Router.GET("/ws/channel/:location/connections/:connectionId",
    serverConfig.TenantMiddleware.MiddlewareFunc(),
    wsAuthMiddleWare.MiddlewareFunc(),
    recruitmentWSHandler.Handler)

careWSHandler := carews.NewChatWebSocketHandler(connPool, multitenantService, notificationService, nc, natsStream)

// Old custom middleware
careWSAuthMiddleware := func(c *gin.Context) {
    token := c.Query("token")
    if token == "" {
        log.Error().Msg("No token provided in query parameter")
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Token required"})
        c.Abort()
        return
    }

    subdomain := c.Query("subdomain")
    if subdomain == "" {
        log.Error().Msg("No subdomain provided in query parameter")
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Subdomain required"})
        c.Abort()
        return
    }

    c.Request.Header.Set("Authorization", "Bearer "+token)
    _, failed := serverConfig.FirebaseAuthMiddleware.verifyToken(c)
    if failed {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Invalid token"})
        c.Abort()
        return
    }
    c.Next()
}

serverConfig.Router.GET("/ws/care/chat", careWSAuthMiddleware, careWSHandler.Handler)
```

### After (Migrated)

```go
multitenantService := access.NewMultitenantService(coreStore)

pool, err := pgxpool.New(context.Background(), dbConnection)
if err != nil {
    log.Fatal().Err(err).Msg("Unable to create connection pool")
}
defer pool.Close()

store := recruitmentdb.NewStore(pool)
hub := recruitmentws.NewSignalingHub(pool, store, nc, natsStream)
go hub.Run(ctx)

recruitmentWSHandler := recruitmentws.NewWSHandler(hub, connPool)

// Create auth provider (can be Firebase, Kratos, or custom)
authProvider := service.NewFirebaseAuthProvider(
    serverConfig.TenantClientPool,
    multitenantService,
)

// Standard WebSocket auth (header-based)
wsAuthMiddleware := service.NewWSAuthMiddleware(authProvider)
serverConfig.Router.GET("/ws/channel/:location/connections/:connectionId",
    serverConfig.TenantMiddleware.MiddlewareFunc(),
    wsAuthMiddleware.MiddlewareFunc(),
    recruitmentWSHandler.Handler)

careWSHandler := carews.NewChatWebSocketHandler(connPool, multitenantService, notificationService, nc, natsStream)

// Query parameter auth for mobile apps
careWSAuthMiddleware := service.NewWSAuthMiddlewareWithQueryParams(authProvider)
serverConfig.Router.GET("/ws/care/chat", careWSAuthMiddleware, careWSHandler.Handler)
```

## Key Changes

1. **No more `serverConfig.FirebaseAuthMiddleware`** - Create auth provider directly
2. **Use helper functions** - `NewWSAuthMiddlewareWithQueryParams()` for query param auth
3. **Provider-agnostic** - Easy to swap Firebase with Kratos or custom provider
4. **Cleaner code** - No custom inline middleware functions needed

## Available Helper Functions

- `NewWSAuthMiddleware(authProvider)` - Standard header-based auth
- `NewWSAuthMiddlewareWithQueryParams(authProvider)` - Query parameter auth
- `NewWSAuthMiddlewareWithHeaderFallback(authProvider)` - Tries header, falls back to query

## Switching to Kratos

To switch from Firebase to Kratos, just change the provider:

```go
// From Firebase
authProvider := service.NewFirebaseAuthProvider(pool, multitenantService)

// To Kratos
authProvider := service.NewKratosAuthProvider(kratosURL)

// Everything else stays the same!
wsAuthMiddleware := service.NewWSAuthMiddleware(authProvider)
```
