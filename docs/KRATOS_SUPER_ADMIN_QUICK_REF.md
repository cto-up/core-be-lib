# SUPER_ADMIN Quick Reference

## Overview

SUPER_ADMIN users have elevated privileges that bypass normal tenant restrictions in the Kratos multi-tenancy implementation.

## Quick Check

```go
import "ctoup.com/coreapp/pkg/shared/service"

// Check if current user is SUPER_ADMIN
if service.IsSuperAdmin(c) {
    // User has SUPER_ADMIN privileges
}

// Check if tenant context is required
if service.RequiresTenantContext(c) {
    // User needs tenant context (not SUPER_ADMIN)
}

// Get tenant context (may be empty for SUPER_ADMIN on root domain)
tenantID, subdomain, ok := service.GetTenantContext(c)
```

## Access Matrix

| Scenario               | SUPER_ADMIN | Regular User        |
| ---------------------- | ----------- | ------------------- |
| Root domain (app.com)  | ✅ Allowed  | ✅ Allowed          |
| Own tenant subdomain   | ✅ Allowed  | ✅ Allowed          |
| Other tenant subdomain | ✅ Allowed  | ❌ Forbidden (403)  |
| Query all tenants      | ✅ Allowed  | ❌ Not allowed      |
| No tenant context      | ✅ Allowed  | ⚠️ Depends on route |

## Common Patterns

### Pattern 1: Optional Tenant Scope

Use when endpoint should return all data for SUPER_ADMIN, scoped data for regular users.

```go
func ListResources(c *gin.Context) {
    var resources []Resource
    var err error

    if service.IsSuperAdmin(c) {
        resources, err = store.ListAllResources(c)
    } else {
        tenantID, _, ok := service.GetTenantContext(c)
        if !ok {
            c.JSON(400, gin.H{"error": "Tenant context required"})
            return
        }
        resources, err = store.ListResourcesByTenant(c, tenantID)
    }

    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(200, resources)
}
```

### Pattern 2: Tenant Validation with Override

Use when accessing specific resource - validate ownership for regular users.

```go
func GetResource(c *gin.Context) {
    resourceID := c.Param("id")

    resource, err := store.GetResource(c, resourceID)
    if err != nil {
        c.JSON(404, gin.H{"error": "Not found"})
        return
    }

    // SUPER_ADMIN can access any resource
    if service.IsSuperAdmin(c) {
        c.JSON(200, resource)
        return
    }

    // Regular users must own the resource
    tenantID, _, ok := service.GetTenantContext(c)
    if !ok || resource.TenantID != tenantID {
        c.JSON(403, gin.H{"error": "Access denied"})
        return
    }

    c.JSON(200, resource)
}
```

### Pattern 3: SUPER_ADMIN Only

Use for admin-only operations.

```go
func ManageAllTenants(c *gin.Context) {
    if !service.IsSuperAdmin(c) {
        c.JSON(403, gin.H{"error": "SUPER_ADMIN required"})
        return
    }

    tenants, err := store.ListAllTenants(c)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(200, tenants)
}
```

### Pattern 4: Conditional Query Helper

Create helper functions for common query patterns.

```go
func GetResourcesForUser(c *gin.Context) ([]Resource, error) {
    if service.IsSuperAdmin(c) {
        return store.ListAllResources(c)
    }

    tenantID, _, ok := service.GetTenantContext(c)
    if !ok {
        return nil, errors.New("tenant context required")
    }

    return store.ListResourcesByTenant(c, tenantID)
}

// Usage in handler
func ListResourcesHandler(c *gin.Context) {
    resources, err := GetResourcesForUser(c)
    if err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, resources)
}
```

## Database Queries

### All Tenants Query (SUPER_ADMIN)

```sql
-- name: ListAllResources :many
SELECT * FROM resources
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;
```

### Single Tenant Query (Regular Users)

```sql
-- name: ListResourcesByTenant :many
SELECT * FROM resources
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;
```

### Conditional Query in Code

```go
func GetResources(c *gin.Context, limit, offset int32) ([]Resource, error) {
    if service.IsSuperAdmin(c) {
        return store.ListAllResources(c, limit, offset)
    }

    tenantID, _, ok := service.GetTenantContext(c)
    if !ok {
        return nil, errors.New("tenant context required")
    }

    return store.ListResourcesByTenant(c, tenantID, limit, offset)
}
```

## Logging & Auditing

Always log SUPER_ADMIN actions:

```go
func LogAction(c *gin.Context, action, resource string) {
    log.Info().
        Str("action", action).
        Str("resource", resource).
        Str("user_id", c.GetString(auth.AUTH_USER_ID)).
        Bool("is_super_admin", service.IsSuperAdmin(c)).
        Str("tenant_id", c.GetString(auth.AUTH_TENANT_ID_KEY)).
        Msg("User action")
}

func DeleteResource(c *gin.Context) {
    resourceID := c.Param("id")
    LogAction(c, "delete", resourceID)

    // ... deletion logic ...
}
```

## Route Organization

```go
// Public routes (no auth)
public := router.Group("/public")

// Tenant-scoped routes (auth + tenant validation)
api := router.Group("/api/v1")
api.Use(authMiddleware.MiddlewareFunc())
api.Use(tenantMiddleware.MiddlewareFunc())
{
    api.GET("/resources", listResources)      // Scoped by tenant
    api.POST("/resources", createResource)    // Scoped by tenant
}

// Admin routes (ADMIN or SUPER_ADMIN, no tenant validation)
admin := router.Group("/admin-api")
admin.Use(authMiddleware.MiddlewareFunc())
{
    admin.GET("/tenants", listAllTenants)     // Cross-tenant
    admin.POST("/tenants", createTenant)      // Cross-tenant
}

// Super admin only routes
superAdmin := router.Group("/superadmin-api")
superAdmin.Use(authMiddleware.MiddlewareFunc())
{
    superAdmin.GET("/stats", getSystemStats)  // System-wide
    superAdmin.POST("/tenants/:id/disable", disableTenant)
}
```

## Testing

```go
func TestSuperAdminAccess(t *testing.T) {
    // Create SUPER_ADMIN token
    superAdminToken := createTestToken(map[string]interface{}{
        "SUPER_ADMIN": true,
    })

    // Test root domain access
    req := httptest.NewRequest("GET", "http://app.com/api/v1/resources", nil)
    req.Header.Set("X-Session-Token", superAdminToken)

    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)

    assert.Equal(t, 200, w.Code)

    // Test cross-tenant access
    req = httptest.NewRequest("GET", "http://tenant1.app.com/api/v1/resources", nil)
    req.Header.Set("X-Session-Token", superAdminToken)

    w = httptest.NewRecorder()
    router.ServeHTTP(w, req)

    assert.Equal(t, 200, w.Code) // SUPER_ADMIN can access
}

func TestRegularUserAccess(t *testing.T) {
    // Create regular user token for tenant1
    userToken := createTestTokenWithTenant("tenant1", map[string]interface{}{
        "USER": true,
    })

    // Test own tenant access
    req := httptest.NewRequest("GET", "http://tenant1.app.com/api/v1/resources", nil)
    req.Header.Set("X-Session-Token", userToken)

    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)

    assert.Equal(t, 200, w.Code) // Own tenant OK

    // Test other tenant access
    req = httptest.NewRequest("GET", "http://tenant2.app.com/api/v1/resources", nil)
    req.Header.Set("X-Session-Token", userToken)

    w = httptest.NewRecorder()
    router.ServeHTTP(w, req)

    assert.Equal(t, 403, w.Code) // Other tenant forbidden
}
```

## Common Mistakes

### ❌ Don't: Assume tenant context always exists

```go
// BAD
tenantID, _, _ := service.GetTenantContext(c)
resources := store.GetResourcesByTenant(c, tenantID) // tenantID might be empty!
```

### ✅ Do: Check if SUPER_ADMIN or validate tenant context

```go
// GOOD
if service.IsSuperAdmin(c) {
    resources := store.GetAllResources(c)
} else {
    tenantID, _, ok := service.GetTenantContext(c)
    if !ok {
        return error
    }
    resources := store.GetResourcesByTenant(c, tenantID)
}
```

### ❌ Don't: Skip tenant validation for SUPER_ADMIN in database

```go
// BAD - No validation that tenant exists
tenantID := c.Param("tenant_id")
resources := store.GetResourcesByTenant(c, tenantID)
```

### ✅ Do: Validate tenant exists even for SUPER_ADMIN

```go
// GOOD
tenantID := c.Param("tenant_id")
tenant, err := store.GetTenant(c, tenantID)
if err != nil {
    return errors.New("tenant not found")
}
resources := store.GetResourcesByTenant(c, tenantID)
```

### ❌ Don't: Forget to log SUPER_ADMIN actions

```go
// BAD - No audit trail
func DeleteResource(c *gin.Context) {
    resourceID := c.Param("id")
    store.DeleteResource(c, resourceID)
}
```

### ✅ Do: Always log SUPER_ADMIN actions

```go
// GOOD
func DeleteResource(c *gin.Context) {
    resourceID := c.Param("id")

    log.Info().
        Str("resource_id", resourceID).
        Bool("is_super_admin", service.IsSuperAdmin(c)).
        Msg("Deleting resource")

    store.DeleteResource(c, resourceID)
}
```

## Security Checklist

- ✅ SUPER_ADMIN role assigned only to trusted administrators
- ✅ All SUPER_ADMIN actions logged for audit
- ✅ Handlers check `IsSuperAdmin()` for cross-tenant operations
- ✅ Database queries conditionally scope by tenant
- ✅ SUPER_ADMIN endpoints protected by role check
- ✅ Regular users cannot escalate to SUPER_ADMIN
- ✅ SUPER_ADMIN access monitored and alerted
- ✅ Tests verify SUPER_ADMIN can access all tenants
- ✅ Tests verify regular users cannot access other tenants
- ✅ Tenant validation still performed for data integrity

## Quick Tips

1. **Use helper functions** - Create `GetResourcesForUser()` style helpers
2. **Log everything** - SUPER_ADMIN actions should be auditable
3. **Test both paths** - Test SUPER_ADMIN and regular user access
4. **Validate tenant exists** - Even SUPER_ADMIN shouldn't access invalid tenants
5. **Use route groups** - Separate tenant-scoped and admin routes
6. **Check role in handlers** - Don't rely solely on middleware
7. **Monitor access patterns** - Alert on unusual SUPER_ADMIN activity
8. **Limit SUPER_ADMIN** - Only assign to necessary personnel
9. **Document exceptions** - Note where SUPER_ADMIN bypasses rules
10. **Review regularly** - Audit SUPER_ADMIN users and their actions

## Related Documentation

- [Full Multi-Tenancy Guide](./KRATOS_MULTI_TENANCY.md)
- [Server Setup Example](./KRATOS_SERVER_SETUP_EXAMPLE.md)
- [Frontend Guide](../../hub/frontend/KRATOS_TENANT_GUIDE.md)
