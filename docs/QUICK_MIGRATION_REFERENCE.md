# Quick Migration Reference

## Replace This Pattern

### ❌ Old Code

```go
wsAuthMiddleWare := access.NewWSAuthMiddleware(serverConfig.FirebaseAuthMiddleware)
```

### ✅ New Code

```go
authProvider := service.NewFirebaseAuthProvider(
    serverConfig.TenantClientPool,
    multitenantService,
)
wsAuthMiddleware := service.NewWSAuthMiddleware(authProvider)
```

---

## Custom Query Parameter Auth

### ❌ Old Code (Custom Inline Function)

```go
careWSAuthMiddleware := func(c *gin.Context) {
    token := c.Query("token")
    if token == "" {
        c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized: Token required"})
        c.Abort()
        return
    }

    subdomain := c.Query("subdomain")
    if subdomain == "" {
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
```

### ✅ New Code (One Line)

```go
authProvider := service.NewFirebaseAuthProvider(serverConfig.TenantClientPool, multitenantService)
careWSAuthMiddleware := service.NewWSAuthMiddlewareWithQueryParams(authProvider)
```

---

## Accessing ServerConfig Fields

### ❌ Old Code

```go
serverConfig.FirebaseAuthMiddleware  // REMOVED - doesn't exist anymore
```

### ✅ New Code

```go
serverConfig.TenantClientPool  // Still available
serverConfig.AuthMiddleware    // Still available
serverConfig.TenantMiddleware  // Still available
```

---

## Three WebSocket Auth Options

```go
authProvider := service.NewFirebaseAuthProvider(pool, multitenantService)

// Option 1: Header-based (standard)
ws1 := service.NewWSAuthMiddleware(authProvider)

// Option 2: Query parameter only (mobile apps)
ws2 := service.NewWSAuthMiddlewareWithQueryParams(authProvider)

// Option 3: Header with query fallback (flexible)
ws3 := service.NewWSAuthMiddlewareWithHeaderFallback(authProvider)
```

---

## Complete Example

```go
// Create auth provider once
authProvider := service.NewFirebaseAuthProvider(
    serverConfig.TenantClientPool,
    multitenantService,
)

// Use for multiple routes
router.GET("/ws/standard",
    service.NewWSAuthMiddleware(authProvider).MiddlewareFunc(),
    handler1)

router.GET("/ws/mobile",
    service.NewWSAuthMiddlewareWithQueryParams(authProvider),
    handler2)

router.GET("/ws/flexible",
    service.NewWSAuthMiddlewareWithHeaderFallback(authProvider),
    handler3)
```
