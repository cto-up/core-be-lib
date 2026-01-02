# Kratos Soft Multi-Tenancy Implementation

This document describes the soft multi-tenancy implementation for Ory Kratos in the core-be-lib backend and frontend-shadcn frontend.

## Overview

Soft multi-tenancy allows multiple tenants to share the same Kratos instance and database while maintaining logical data separation. Tenant information is stored in Kratos identity metadata and validated on every request.

## Architecture

### Key Components

1. **Tenant Metadata Storage**: Stored in Kratos `metadata_public` field
2. **Tenant Middleware**: Validates tenant context on each request
3. **Subdomain-based Routing**: Tenants accessed via subdomains (e.g., `tenant1.app.com`)
4. **Database Tenant Mapping**: Maps subdomains to tenant IDs in PostgreSQL
5. **SUPER_ADMIN Bypass**: SUPER_ADMIN users can access any tenant or root domain without restrictions

### Data Flow

```
┌─────────────────────────────────────────────────────────────┐
│                    Client Request                            │
│              (tenant1.app.com/api/resource)                  │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                  Auth Middleware                             │
│  1. Extract Kratos session from cookie/header               │
│  2. Verify session with Kratos                              │
│  3. Extract tenant_id from metadata_public                  │
│  4. Set user context (user_id, email, tenant_id, roles)    │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│              Tenant Middleware                               │
│  1. Extract subdomain from request                          │
│  2. Check if user is SUPER_ADMIN                           │
│     - If SUPER_ADMIN: bypass validation, allow access      │
│     - If root domain: allow access (no tenant required)    │
│  3. Lookup tenant_id from database                          │
│  4. Validate user's tenant_id matches subdomain's tenant    │
│  5. Set tenant context for request                          │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                  Business Logic                              │
│  - All queries scoped by tenant_id from context             │
│  - SUPER_ADMIN can query across tenants                     │
│  - Cross-tenant access prevented for regular users          │
└─────────────────────────────────────────────────────────────┘
```

## Backend Implementation

### 1. Tenant Metadata in Kratos

Tenant information is stored in the identity's `metadata_public` field:

```json
{
  "tenant_id": "tenant-uuid-123",
  "subdomain": "acme",
  "tenant_name": "Acme Corporation",
  "roles": ["USER", "ADMIN"]
}
```

### 2. Enhanced Kratos Provider

**File**: `pkg/shared/auth/kratos/provider.go`

The `VerifyToken` method extracts tenant information from session:

```go
func (k *KratosAuthProvider) VerifyToken(c *gin.Context) (*auth.AuthenticatedUser, error) {
    // ... session verification ...

    // Extract tenant information from metadata_public
    tenantID, _ := token.Claims["tenant_id"].(string)
    subdomain, _ := token.Claims["subdomain"].(string)

    return &auth.AuthenticatedUser{
        UserID:   token.UID,
        Email:    email,
        TenantID: tenantID,
        Subdomain: subdomain,
        // ... other fields ...
    }, nil
}
```

### 3. Tenant Metadata Management

**File**: `pkg/shared/auth/kratos/tenant_metadata.go`

Provides functions to manage tenant associations:

```go
// Set tenant metadata for a user
func (k *KratosAuthClient) SetTenantMetadata(ctx context.Context, uid string, metadata TenantMetadata) error

// Get tenant metadata for a user
func (k *KratosAuthClient) GetTenantMetadata(ctx context.Context, uid string) (*TenantMetadata, error)

// Create user with tenant association
func (k *KratosAuthClient) CreateUserWithTenant(ctx context.Context, user *auth.UserToCreate, tenantID string, subdomain string) (*auth.UserRecord, error)

// List all users in a tenant
func (k *KratosAuthClient) ListUsersByTenant(ctx context.Context, tenantID string) ([]*auth.UserRecord, error)
```

### 4. Tenant Middleware

**File**: `pkg/shared/service/kratos_tenant_middleware.go`

Validates tenant context on each request:

```go
func (ktm *KratosTenantMiddleware) MiddlewareFunc() gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. Extract subdomain from request
        subdomain, err := utils.GetSubdomain(c)

        // 2. Get tenant ID from database
        tenantID, err := ktm.multitenantService.GetFirebaseTenantID(c, subdomain)

        // 3. Validate user's tenant matches subdomain's tenant
        userTenantID := c.GetString(auth.AUTH_TENANT_ID_KEY)
        if userTenantID != tenantID {
            // Reject request - tenant mismatch
        }

        // 4. Set tenant context
        c.Set(auth.AUTH_TENANT_ID_KEY, tenantID)
        c.Next()
    }
}
```

### 5. Tenant Service

**File**: `pkg/shared/service/kratos_tenant_service.go`

High-level service for tenant operations:

```go
type KratosTenantService struct {
    store        *db.Store
    authProvider auth.AuthProvider
}

// Assign user to tenant
func (kts *KratosTenantService) AssignUserToTenant(ctx context.Context, userID string, subdomain string) error

// Create user with tenant
func (kts *KratosTenantService) CreateUserWithTenant(ctx context.Context, email string, password string, subdomain string, roles []string) (*auth.UserRecord, error)

// Validate user has access to tenant
func (kts *KratosTenantService) ValidateUserTenantAccess(ctx context.Context, userID string, subdomain string) (bool, error)
```

### 6. Middleware Setup

In your server initialization:

```go
// Initialize auth provider
authProvider, err := auth.InitializeAuthProvider(ctx, multitenantService)

// Create middlewares
authMiddleware := service.NewAuthMiddleware(authProvider, clientAppService)
tenantMiddleware := service.NewKratosTenantMiddleware(multitenantService, authProvider)

// Apply to router
router.Use(authMiddleware.MiddlewareFunc())
router.Use(tenantMiddleware.MiddlewareFunc())
```

## Frontend Implementation

### 1. API Client Configuration

**File**: `hub/frontend-shadcn/src/boot/api.ts`

Configured to send Kratos session cookies and tenant headers:

```typescript
export function initializeApiClient() {
  // Enable credentials for session cookies
  axios.defaults.withCredentials = true;
  OpenAPI.WITH_CREDENTIALS = true;

  // Add session token and tenant ID to requests
  axios.interceptors.request.use(async (config) => {
    const session = await kratosService.getSession();

    if (session && session.active) {
      config.headers["X-Session-Token"] = session.id;

      const tenantID = session.identity.metadata_public?.tenant_id;
      if (tenantID) {
        config.headers["X-Tenant-ID"] = tenantID;
      }
    }

    return config;
  });
}
```

### 2. Tenant Composable

**File**: `hub/frontend-shadcn/src/composables/useTenant.ts`

Provides tenant context from Kratos session:

```typescript
export function useTenant() {
  const { session } = useKratosAuth();

  const tenantID = computed(() => {
    return session.value?.identity.metadata_public?.tenant_id || null;
  });

  const subdomain = computed(() => {
    return session.value?.identity.metadata_public?.subdomain || null;
  });

  const hasTenant = computed(() => {
    return !!tenantID.value;
  });

  return {
    tenantID,
    subdomain,
    hasTenant,
  };
}
```

### 3. Registration with Tenant

**File**: `hub/frontend-shadcn/src/composables/kratos-auth.composable.ts`

Registration flow includes tenant subdomain:

```typescript
const signMeUp = async (
  email: string,
  password: string,
  name?: string,
  tenantSubdomain?: string
) => {
  // Store tenant context for post-registration
  if (tenantSubdomain) {
    sessionStorage.setItem("pending_tenant", tenantSubdomain);
  }

  const flow = await kratosService.initRegistrationFlow();

  const traits: any = {
    email,
    name: name || "",
  };

  // Include subdomain in traits
  if (tenantSubdomain) {
    traits.subdomain = tenantSubdomain;
  }

  await kratosService.submitRegistrationFlow(flow.id, {
    traits,
    password,
    method: "password",
  });
};
```

## Database Schema

### Tenant Table

```sql
CREATE TABLE core_tenants (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(128) NOT NULL UNIQUE,
    subdomain VARCHAR(255) NOT NULL UNIQUE,
    enable_email_link_sign_in boolean NOT NULL,
    allow_password_sign_up boolean NOT NULL,
    user_id varchar(128) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT tenants_pk PRIMARY KEY (id)
);

CREATE INDEX idx_tenants_subdomain ON core_tenants ("subdomain");
```

### Tenant-Scoped Tables

All tenant-scoped tables should include `tenant_id`:

```sql
CREATE TABLE example_resources (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id VARCHAR(64) NOT NULL,
    name VARCHAR(255) NOT NULL,
    -- other fields --
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT example_resources_pk PRIMARY KEY (id),
    CONSTRAINT fk_tenant FOREIGN KEY (tenant_id) REFERENCES core_tenants(tenant_id)
);

CREATE INDEX idx_example_resources_tenant ON example_resources(tenant_id);
```

## Usage Examples

### Backend: Create User with Tenant

```go
tenantService := service.NewKratosTenantService(store, authProvider)

user, err := tenantService.CreateUserWithTenant(
    ctx,
    "user@example.com",
    "securepassword",
    "acme",
    []string{"USER"},
)
```

### Backend: Assign Existing User to Tenant

```go
err := tenantService.AssignUserToTenant(ctx, userID, "acme")
```

### Backend: Get Tenant Context in Handler

```go
func MyHandler(c *gin.Context) {
    // Check if user is SUPER_ADMIN
    if service.IsSuperAdmin(c) {
        // SUPER_ADMIN can access all data
        resources, err := store.GetAllResources(c)
        c.JSON(200, resources)
        return
    }

    // Regular users must have tenant context
    tenantID, subdomain, ok := service.GetTenantContext(c)
    if !ok {
        c.JSON(400, gin.H{"error": "No tenant context"})
        return
    }

    // Query data scoped by tenantID
    resources, err := store.GetResourcesByTenant(c, tenantID)
}
```

### Backend: SUPER_ADMIN Access Pattern

```go
func ListResources(c *gin.Context) {
    var resources []Resource
    var err error

    if service.IsSuperAdmin(c) {
        // SUPER_ADMIN sees all resources across all tenants
        resources, err = store.ListAllResources(c)
    } else {
        // Regular users see only their tenant's resources
        tenantID, _, ok := service.GetTenantContext(c)
        if !ok {
            c.JSON(400, gin.H{"error": "No tenant context"})
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

### Frontend: Check Tenant Context

```vue
<script setup lang="ts">
import { useTenant } from "@/composables/useTenant";

const { tenantID, subdomain, hasTenant } = useTenant();
</script>

<template>
  <div v-if="hasTenant">
    <p>Tenant: {{ subdomain }}</p>
    <p>ID: {{ tenantID }}</p>
  </div>
</template>
```

### Frontend: Register with Tenant

```typescript
import { useKratosAuth } from "@/composables/kratos-auth.composable";

const { signMeUp } = useKratosAuth();

// Extract subdomain from URL
const subdomain = window.location.hostname.split(".")[0];

await signMeUp("user@example.com", "password123", "John Doe", subdomain);
```

## Security Considerations

### 1. Tenant Isolation

- **Middleware Validation**: Every request validates tenant context
- **Database Queries**: Always scope by `tenant_id` for regular users
- **Cross-Tenant Prevention**: Middleware rejects requests with tenant mismatch
- **SUPER_ADMIN Exception**: SUPER_ADMIN users bypass tenant validation

### 2. SUPER_ADMIN Access Control

**SUPER_ADMIN users have special privileges:**

- ✅ Can access root domain without tenant context
- ✅ Can access any tenant's subdomain
- ✅ Bypass tenant validation in middleware
- ✅ Can query across all tenants
- ⚠️ Must explicitly check `IsSuperAdmin()` in handlers for cross-tenant queries

**Implementation Pattern:**

```go
func GetResources(c *gin.Context) {
    if service.IsSuperAdmin(c) {
        // SUPER_ADMIN: return all resources across tenants
        resources, err := store.GetAllResources(c)
    } else {
        // Regular user: return only tenant's resources
        tenantID, _, ok := service.GetTenantContext(c)
        if !ok {
            c.JSON(400, gin.H{"error": "Tenant context required"})
            return
        }
        resources, err := store.GetResourcesByTenant(c, tenantID)
    }
}
```

**Security Best Practices:**

1. **Always check role in handlers** - Don't rely solely on middleware
2. **Log SUPER_ADMIN actions** - Audit trail for cross-tenant access
3. **Limit SUPER_ADMIN users** - Only assign to trusted administrators
4. **Use separate endpoints** - Consider `/superadmin-api/*` for admin-only operations
5. **Validate tenant exists** - Even for SUPER_ADMIN, validate tenant_id is valid

**Helper Functions:**

```go
// Check if user is SUPER_ADMIN
if service.IsSuperAdmin(c) {
    // Allow cross-tenant access
}

// Check if tenant context is required
if service.RequiresTenantContext(c) {
    // Enforce tenant validation
}

// Get tenant context (returns empty for SUPER_ADMIN on root domain)
tenantID, subdomain, ok := service.GetTenantContext(c)
```

### 2. Metadata Security

- **metadata_public**: Visible to user, used for tenant context
- **metadata_admin**: Not exposed to users, for internal use only
- **Validation**: Backend validates tenant associations, not just trusts client

### 3. Session Security

- **HttpOnly Cookies**: Kratos sessions use secure cookies
- **CSRF Protection**: Kratos provides built-in CSRF protection
- **Session Expiry**: Configure appropriate session timeouts

### 4. Subdomain Validation

```go
func GetSubdomain(c *gin.Context) (string, error) {
    host := c.Request.Host

    // Validate subdomain format
    if !isValidSubdomain(host) {
        return "", errors.New("invalid subdomain")
    }

    // Extract subdomain
    parts := strings.Split(host, ".")
    if len(parts) > 2 {
        return parts[0], nil
    }

    return "", nil
}
```

## Testing

### Backend Tests

```go
func TestTenantMiddleware(t *testing.T) {
    // Setup
    router := gin.New()
    router.Use(tenantMiddleware.MiddlewareFunc())

    // Test valid tenant
    req := httptest.NewRequest("GET", "http://acme.app.com/api/resource", nil)
    req.Header.Set("X-Session-Token", validSessionToken)

    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)

    assert.Equal(t, 200, w.Code)

    // Test tenant mismatch
    req = httptest.NewRequest("GET", "http://other.app.com/api/resource", nil)
    req.Header.Set("X-Session-Token", acmeUserSessionToken)

    w = httptest.NewRecorder()
    router.ServeHTTP(w, req)

    assert.Equal(t, 403, w.Code)
}
```

### Frontend Tests

```typescript
describe("useTenant", () => {
  it("extracts tenant from session", () => {
    const { tenantID, subdomain } = useTenant();

    expect(tenantID.value).toBe("tenant-123");
    expect(subdomain.value).toBe("acme");
  });
});
```

## Migration from Firebase

### 1. Update Auth Provider

```go
// Before
authProvider, err := auth.InitializeAuthProvider(ctx, multitenantService)
// Provider automatically selected based on AUTH_PROVIDER env var

// Set AUTH_PROVIDER=kratos in environment
```

### 2. Migrate User Data

```go
// For each Firebase user
firebaseUser := getFirebaseUser(uid)

// Create in Kratos
kratosUser, err := kratosClient.CreateUser(ctx, &auth.UserToCreate{}.
    Email(firebaseUser.Email).
    DisplayName(firebaseUser.DisplayName))

// Set tenant metadata
err = kratosClient.SetTenantMetadata(ctx, kratosUser.UID, kratos.TenantMetadata{
    TenantID:  firebaseUser.TenantID,
    Subdomain: firebaseUser.Subdomain,
})

// Set roles
err = kratosClient.SetCustomUserClaims(ctx, kratosUser.UID, firebaseUser.CustomClaims)
```

### 3. Update Frontend

```typescript
// Replace Firebase imports
// import { getAuth } from "firebase/auth";
import { kratosService } from "@/services/kratos.service";

// Replace Firebase auth calls
// const user = getAuth().currentUser;
const session = await kratosService.getSession();
```

## Troubleshooting

### Issue: User has no tenant metadata

**Solution**: Assign tenant using `KratosTenantService.AssignUserToTenant()`

### Issue: Tenant mismatch error

**Cause**: User trying to access different tenant's subdomain

**Solution**: Ensure user is assigned to correct tenant

### Issue: Session not found

**Cause**: Session cookie not being sent

**Solution**: Ensure `withCredentials: true` in axios config

### Issue: CORS errors

**Solution**: Configure Kratos CORS settings:

```yaml
# kratos.yml
serve:
  public:
    cors:
      enabled: true
      allowed_origins:
        - https://app.com
        - https://*.app.com
      allowed_methods:
        - GET
        - POST
        - PUT
        - DELETE
      allowed_headers:
        - Authorization
        - Content-Type
        - X-Session-Token
      allow_credentials: true
```

## Best Practices

1. **Always scope queries by tenant_id**
2. **Validate tenant context in middleware, not in handlers**
3. **Use database indexes on tenant_id columns**
4. **Log tenant context in all operations**
5. **Test cross-tenant access prevention**
6. **Monitor for tenant isolation violations**
7. **Use connection pooling for Kratos API calls**
8. **Cache tenant lookups to reduce database queries**

## References

- [Ory Kratos Documentation](https://www.ory.sh/docs/kratos)
- [Kratos Identity Schema](https://www.ory.sh/docs/kratos/manage-identities/identity-schema)
- [Kratos Session Management](https://www.ory.sh/docs/kratos/session-management)
- [Multi-Tenancy Patterns](https://docs.microsoft.com/en-us/azure/architecture/patterns/multi-tenancy)

## SUPER_ADMIN Access Patterns

### Overview

SUPER_ADMIN users have special privileges that bypass normal tenant restrictions:

- ✅ Access root domain without tenant context
- ✅ Access any tenant's subdomain
- ✅ Query data across all tenants
- ✅ Manage multiple tenants

### Middleware Behavior

The `KratosTenantMiddleware` handles SUPER_ADMIN users specially:

```go
// 1. Root domain access (app.com)
// SUPER_ADMIN: ✅ Allowed, no tenant context required
// Regular user: ✅ Allowed, no tenant context

// 2. Tenant subdomain access (acme.app.com)
// SUPER_ADMIN: ✅ Allowed, tenant context set but not validated
// Regular user: ✅ Allowed only if user belongs to "acme" tenant
```

### Handler Implementation Patterns

#### Pattern 1: Optional Tenant Scope

```go
func ListResources(c *gin.Context) {
    var resources []Resource
    var err error

    if service.IsSuperAdmin(c) {
        // SUPER_ADMIN: return all resources
        resources, err = store.ListAllResources(c)
    } else {
        // Regular user: return only tenant's resources
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

#### Pattern 2: Tenant Context with SUPER_ADMIN Override

```go
func GetResource(c *gin.Context) {
    resourceID := c.Param("id")

    // Get resource
    resource, err := store.GetResource(c, resourceID)
    if err != nil {
        c.JSON(404, gin.H{"error": "Resource not found"})
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

#### Pattern 3: SUPER_ADMIN Only Endpoints

```go
func ManageAllTenants(c *gin.Context) {
    // Require SUPER_ADMIN
    if !service.IsSuperAdmin(c) {
        c.JSON(403, gin.H{"error": "SUPER_ADMIN access required"})
        return
    }

    // SUPER_ADMIN operations
    tenants, err := store.ListAllTenants(c)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(200, tenants)
}
```

### Database Query Patterns

#### Query All Tenants (SUPER_ADMIN)

```sql
-- name: ListAllResources :many
SELECT * FROM resources
ORDER BY created_at DESC;
```

#### Query Single Tenant (Regular Users)

```sql
-- name: ListResourcesByTenant :many
SELECT * FROM resources
WHERE tenant_id = $1
ORDER BY created_at DESC;
```

#### Conditional Query in Handler

```go
func GetResourcesQuery(c *gin.Context) ([]Resource, error) {
    if service.IsSuperAdmin(c) {
        return store.ListAllResources(c)
    }

    tenantID, _, ok := service.GetTenantContext(c)
    if !ok {
        return nil, errors.New("tenant context required")
    }

    return store.ListResourcesByTenant(c, tenantID)
}
```

### Logging and Auditing

Always log SUPER_ADMIN actions for security auditing:

```go
func AuditLog(c *gin.Context, action string, resource string) {
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

    // Audit log
    AuditLog(c, "delete_resource", resourceID)

    // ... deletion logic ...
}
```

### Route Organization

Organize routes by access level:

```go
// Public routes (no auth)
public := router.Group("/public")

// Tenant-scoped routes (auth + tenant validation)
api := router.Group("/api/v1")
api.Use(authMiddleware.MiddlewareFunc())
api.Use(tenantMiddleware.MiddlewareFunc())

// Admin routes (ADMIN or SUPER_ADMIN)
admin := router.Group("/admin-api")
admin.Use(authMiddleware.MiddlewareFunc())
// No tenant middleware - admins can access cross-tenant

// Super admin routes (SUPER_ADMIN only)
superAdmin := router.Group("/superadmin-api")
superAdmin.Use(authMiddleware.MiddlewareFunc())
// No tenant middleware - super admins have full access
```

### Testing SUPER_ADMIN Access

```go
func TestSuperAdminAccess(t *testing.T) {
    // Setup
    router := setupTestRouter()

    // Create SUPER_ADMIN user
    superAdminToken := createTestUser(t, map[string]interface{}{
        "SUPER_ADMIN": true,
    })

    // Test 1: Access root domain
    req := httptest.NewRequest("GET", "http://app.com/api/v1/resources", nil)
    req.Header.Set("X-Session-Token", superAdminToken)

    w := httptest.NewRecorder()
    router.ServeHTTP(w, req)

    assert.Equal(t, 200, w.Code)

    // Test 2: Access any tenant
    req = httptest.NewRequest("GET", "http://tenant1.app.com/api/v1/resources", nil)
    req.Header.Set("X-Session-Token", superAdminToken)

    w = httptest.NewRecorder()
    router.ServeHTTP(w, req)

    assert.Equal(t, 200, w.Code)

    // Test 3: Regular user cannot access other tenant
    regularUserToken := createTestUser(t, map[string]interface{}{
        "USER": true,
    })

    req = httptest.NewRequest("GET", "http://tenant2.app.com/api/v1/resources", nil)
    req.Header.Set("X-Session-Token", regularUserToken)

    w = httptest.NewRecorder()
    router.ServeHTTP(w, req)

    assert.Equal(t, 403, w.Code) // Forbidden
}
```

### Security Checklist

- ✅ SUPER_ADMIN role assigned only to trusted administrators
- ✅ All SUPER_ADMIN actions logged for audit trail
- ✅ Handlers explicitly check `IsSuperAdmin()` for cross-tenant operations
- ✅ Database queries conditionally scope by tenant
- ✅ SUPER_ADMIN endpoints protected by role check
- ✅ Regular users cannot escalate to SUPER_ADMIN
- ✅ SUPER_ADMIN access monitored and alerted
- ✅ Tests verify SUPER_ADMIN can access all tenants
- ✅ Tests verify regular users cannot access other tenants

### Common Pitfalls

❌ **Don't assume tenant context always exists:**

```go
// BAD
tenantID, _, _ := service.GetTenantContext(c)
resources := store.GetResourcesByTenant(c, tenantID) // tenantID might be empty!
```

✅ **Always check if tenant context is required:**

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

❌ **Don't forget to validate tenant exists for SUPER_ADMIN:**

```go
// BAD - SUPER_ADMIN accessing non-existent tenant
tenantID := c.Param("tenant_id")
resources := store.GetResourcesByTenant(c, tenantID) // No validation!
```

✅ **Validate tenant exists even for SUPER_ADMIN:**

```go
// GOOD
tenantID := c.Param("tenant_id")
tenant, err := store.GetTenant(c, tenantID)
if err != nil {
    return errors.New("tenant not found")
}
resources := store.GetResourcesByTenant(c, tenantID)
```

## Best Practices Summary

1. **Always scope queries by tenant_id for regular users**
2. **Check IsSuperAdmin() for cross-tenant operations**
3. **Validate tenant in middleware, not in handlers**
4. **Use database indexes on tenant_id columns**
5. **Log tenant context and SUPER_ADMIN actions**
6. **Test cross-tenant access prevention**
7. **Monitor for tenant isolation violations**
8. **Use connection pooling for Kratos API calls**
9. **Cache tenant lookups to reduce database queries**
10. **Audit SUPER_ADMIN access to sensitive operations**
11. **Limit SUPER_ADMIN role to trusted administrators**
12. **Use separate route groups for different access levels**
