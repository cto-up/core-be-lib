# Kratos Multi-Tenant Role Architecture

## Overview

This document describes how user roles are implemented in the Kratos multi-tenant system. The architecture separates **global roles** (system-wide) from **per-tenant roles** (tenant-specific).

## Role Types

### 1. Global Roles (Kratos Metadata)

Stored in `metadata_public.roles` array in Kratos identity:

- **SUPER_ADMIN**: System-wide administrator with access to all tenants
  - Automatically gets OWNER permissions in all tenants
  - Can bypass tenant validation
  - Managed through Kratos metadata

```json
{
  "metadata_public": {
    "roles": ["SUPER_ADMIN"]
  }
}
```

### 2. Per-Tenant Roles (Database)

Stored in `core_user_tenant_memberships.role` column:

- **OWNER**: Full control within the tenant

  - Can manage all members and settings
  - Can delete the tenant
  - Can change other members' roles

- **ADMIN**: Administrative access within the tenant

  - Can manage users and content
  - Cannot change owner or delete tenant
  - Can invite new members

- **USER**: Basic user access within the tenant
  - Can use features
  - Cannot manage other users
  - Cannot change settings

## Data Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kratos Identity                           │
│  email: user@example.com                                     │
│  id: user-123                                                │
│  metadata_public: {                                          │
│    primary_tenant_id: "tenant-1",                           │
│    tenant_memberships: ["tenant-1", "tenant-2"],            │
│    roles: ["SUPER_ADMIN"]  ← Global roles only              │
│  }                                                           │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│              Application Database                            │
│                                                              │
│  core_user_tenant_memberships table:                        │
│  ┌──────────┬───────────┬─────────┬────────────┐           │
│  │ user_id  │ tenant_id │ role    │ status     │           │
│  ├──────────┼───────────┼─────────┼────────────┤           │
│  │ user-123 │ tenant-1  │ OWNER   │ active     │           │
│  │ user-123 │ tenant-2  │ ADMIN   │ active     │           │
│  │ user-123 │ tenant-3  │ USER    │ pending    │           │
│  └──────────┴───────────┴─────────┴────────────┘           │
│                                                              │
│  ← Per-tenant roles (source of truth)                       │
└─────────────────────────────────────────────────────────────┘
```

## Implementation

### Middleware Flow

The Kratos tenant middleware now automatically loads tenant roles:

```
1. AuthMiddleware
   ↓ Validates Kratos session
   ↓ Extracts global roles (SUPER_ADMIN)
   ↓ Sets user_id in context

2. KratosTenantMiddleware (UPDATED)
   ↓ Validates tenant access
   ↓ Queries database for user's role in tenant
   ↓ Sets tenant_id, tenant_subdomain, and tenant_role in context
   ↓ SUPER_ADMIN automatically gets OWNER role

3. Handler
   ↓ Can check role via helper functions
   ↓ Or use role-specific middleware
```

### Automatic Role Loading

The `KratosTenantMiddleware` now calls `GetUserTenantRole` automatically:

```go
// In kratos_tenant_middleware.go
if ktm.membershipService != nil {
    // Get user's role in this tenant (also validates access)
    role, err := ktm.membershipService.GetUserTenantRole(
        c.Request.Context(), userID, tenantID)

    if err != nil {
        // User has no access to this tenant
        c.JSON(403, gin.H{"error": "Access denied"})
        c.Abort()
        return
    }

    // Set context
    c.Set("tenant_id", tenantID)
    c.Set("tenant_subdomain", subdomain)
    c.Set("tenant_role", role)  // ← Role automatically loaded
}
```

This means you **no longer need** the separate `LoadTenantRoleMiddleware()` if you're using `KratosTenantMiddleware`.

```sql
CREATE TABLE core_user_tenant_memberships (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    user_id VARCHAR(128) NOT NULL,
    tenant_id VARCHAR(64) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'USER',  -- OWNER, ADMIN, USER
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    invited_by VARCHAR(128),
    invited_at timestamptz,
    joined_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT user_tenant_memberships_pk PRIMARY KEY (id),
    CONSTRAINT user_tenant_memberships_unique UNIQUE (user_id, tenant_id)
);
```

### SQLC Queries

```sql
-- Get user's role in a specific tenant
-- name: GetUserTenantRole :one
SELECT role FROM core_user_tenant_memberships
WHERE user_id = $1 AND tenant_id = $2 AND status = 'active'
LIMIT 1;

-- Update user's role in a tenant
-- name: UpdateUserTenantMembershipRole :one
UPDATE core_user_tenant_memberships
SET role = $3, updated_at = clock_timestamp()
WHERE user_id = $1 AND tenant_id = $2
RETURNING *;
```

### Service Layer

```go
// Get user's role in a tenant
func (s *UserTenantMembershipService) GetUserTenantRole(
    ctx context.Context,
    userID string,
    tenantID string,
) (string, error) {
    role, err := s.store.GetUserTenantRole(ctx, repository.GetUserTenantRoleParams{
        UserID:   userID,
        TenantID: tenantID,
    })
    return role, err
}

// Update user's role in a tenant
func (s *UserTenantMembershipService) UpdateMemberRole(
    ctx context.Context,
    userID string,
    tenantID string,
    role string,
) error {
    _, err := s.store.UpdateUserTenantMembershipRole(ctx,
        repository.UpdateUserTenantMembershipRoleParams{
            UserID:   userID,
            TenantID: tenantID,
            Role:     role,
        })
    return err
}
```

### Role Service

```go
// TenantRoleService handles tenant-specific role operations
type TenantRoleService struct {
    membershipService *UserTenantMembershipService
}

// Load tenant role into context (middleware)
func (s *TenantRoleService) LoadTenantRoleMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        userID := c.GetString("user_id")
        tenantID := c.GetString("tenant_id")

        // SUPER_ADMIN gets OWNER role in all tenants
        if IsSuperAdmin(c) {
            c.Set("tenant_role", "OWNER")
            c.Next()
            return
        }

        // Get role from database
        role, err := s.membershipService.GetUserTenantRole(
            c.Request.Context(), userID, tenantID)

        if err != nil {
            c.Set("tenant_role", "USER") // Default
        } else {
            c.Set("tenant_role", role)
        }

        c.Next()
    }
}

// Require specific role
func (s *TenantRoleService) RequireTenantRole(requiredRole TenantRole) gin.HandlerFunc {
    return func(c *gin.Context) {
        role, _ := c.Get("tenant_role")
        if role != string(requiredRole) {
            c.JSON(403, gin.H{"error": "Insufficient permissions"})
            c.Abort()
            return
        }
        c.Next()
    }
}

// Require minimum role level (OWNER > ADMIN > USER)
func (s *TenantRoleService) RequireMinimumTenantRole(minimumRole TenantRole) gin.HandlerFunc {
    return func(c *gin.Context) {
        role, _ := c.Get("tenant_role")
        if getRoleLevel(TenantRole(role.(string))) < getRoleLevel(minimumRole) {
            c.JSON(403, gin.H{"error": "Insufficient permissions"})
            c.Abort()
            return
        }
        c.Next()
    }
}
```

## Usage Examples

### 1. Middleware Setup (Simplified)

```go
// Initialize services
membershipService := service.NewUserTenantMembershipService(store, authProvider)
roleService := service.NewTenantRoleService(membershipService)

// KratosTenantMiddleware now loads roles automatically
kratosTenantMiddleware := service.NewKratosTenantMiddleware(
    multitenantService,
    authProvider,
    membershipService,  // ← Enables automatic role loading
)

// Apply middleware - role is already loaded!
router.Use(
    authMiddleware.MiddlewareFunc(),
    kratosTenantMiddleware.MiddlewareFunc(),  // ← Loads tenant_role automatically
)

// No need for separate LoadTenantRoleMiddleware() anymore!
```

### 2. Route Protection

```go
// Require specific role
router.POST("/admin/settings",
    roleService.RequireTenantRole(service.TenantRoleAdmin),
    updateSettingsHandler)

// Require minimum role level
router.DELETE("/members/:id",
    roleService.RequireMinimumTenantRole(service.TenantRoleAdmin),
    removeMemberHandler)

// Owner-only routes
ownerRoutes := router.Group("/owner")
ownerRoutes.Use(roleService.RequireTenantRole(service.TenantRoleOwner))
{
    ownerRoutes.DELETE("/tenant", deleteTenant)
    ownerRoutes.POST("/transfer-ownership", transferOwnership)
}
```

### 3. Handler-Level Checks

```go
func updateSettings(c *gin.Context) {
    // Check if user is admin or owner
    if !service.IsTenantAdminOrOwner(c) {
        c.JSON(403, gin.H{"error": "Requires admin role"})
        return
    }

    // Get specific role for business logic
    roleService := service.NewTenantRoleService(membershipService)
    role, err := roleService.GetUserTenantRole(c)

    if role == service.TenantRoleOwner {
        // Owner-specific operations
    } else {
        // Admin operations
    }
}
```

### 4. Creating Users with Roles

```go
// Register first user as OWNER
func registerTenant(c *gin.Context) {
    // Create user
    user, err := authClient.CreateUser(ctx, &auth.UserToCreate{}.
        Email("owner@company.com").
        Password("password"))

    // Add as OWNER of new tenant
    err = membershipService.AddUserToTenant(
        ctx, user.UID, tenantID, "OWNER", "system")
}

// Invite user as ADMIN
func inviteAdmin(c *gin.Context) {
    tenantID := c.GetString("tenant_id")
    inviterID := c.GetString("user_id")

    err := membershipService.InviteUserToTenant(
        ctx, "admin@company.com", tenantID, "ADMIN", inviterID)
}
```

### 5. Updating Roles

```go
func promoteToAdmin(c *gin.Context) {
    // Only OWNER can promote users
    if !service.IsTenantOwner(c) {
        c.JSON(403, gin.H{"error": "Only owner can promote users"})
        return
    }

    userID := c.Param("user_id")
    tenantID := c.GetString("tenant_id")

    err := membershipService.UpdateMemberRole(ctx, userID, tenantID, "ADMIN")
}
```

## Role Hierarchy

```
SUPER_ADMIN (Global)
    ↓ (acts as OWNER in all tenants)

OWNER (Per-Tenant)
    ↓ (can do everything ADMIN can do, plus:)
    - Delete tenant
    - Transfer ownership
    - Change any member's role

ADMIN (Per-Tenant)
    ↓ (can do everything USER can do, plus:)
    - Invite members
    - Remove members (except OWNER)
    - Change USER roles to ADMIN
    - Manage tenant settings

USER (Per-Tenant)
    - Use tenant features
    - View own data
    - Cannot manage other users
```

## Helper Functions

```go
// Check if user is SUPER_ADMIN (global)
func IsSuperAdmin(c *gin.Context) bool {
    val, exists := c.Get("SUPER_ADMIN")
    return exists && val.(bool)
}

// Check if user is tenant owner
func IsTenantOwner(c *gin.Context) bool {
    role, exists := c.Get("tenant_role")
    return exists && role == "OWNER"
}

// Check if user is tenant admin
func IsTenantAdmin(c *gin.Context) bool {
    role, exists := c.Get("tenant_role")
    return exists && role == "ADMIN"
}

// Check if user is admin or owner
func IsTenantAdminOrOwner(c *gin.Context) bool {
    role, exists := c.Get("tenant_role")
    if !exists {
        return false
    }
    roleStr := role.(string)
    return roleStr == "ADMIN" || roleStr == "OWNER"
}
```

## Best Practices

1. **Always use database as source of truth** for per-tenant roles
2. **Keep global roles minimal** - only SUPER_ADMIN in Kratos metadata
3. **Load roles via middleware** for automatic context injection
4. **Apply role checks at route level** when possible for clarity
5. **Use helper functions** for consistent role checking
6. **Log role checks** for security auditing
7. **Validate role changes** - ensure only authorized users can change roles
8. **Handle SUPER_ADMIN specially** - they bypass normal tenant restrictions

## Migration from company_role

If you previously used `company_role` in Kratos traits:

1. **Remove from identity schema** - no longer needed
2. **Migrate existing roles** to database table
3. **Update middleware** to use database queries
4. **Keep SUPER_ADMIN** in metadata_public.roles only
5. **Test thoroughly** - ensure all role checks work correctly

The `company_role` field was a legacy pattern that didn't support per-tenant roles. The new architecture properly separates global and tenant-specific roles.
