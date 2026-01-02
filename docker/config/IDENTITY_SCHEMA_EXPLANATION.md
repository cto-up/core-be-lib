# Kratos Identity Schema for Multi-Tenant Membership

## Schema Comparison

### ❌ Old Schema 1 (`docker/config/identity.schema.json`)

```json
{
  "traits": {
    "email": "...",
    "roles": ["ADMIN"], // ❌ Roles in traits (wrong place)
    "name": {
      "first": "John",
      "last": "Doe"
    }
  }
}
```

**Problems:**

- Roles in `traits` instead of `metadata_public`
- No support for tenant memberships
- No subdomain field for registration
- Name as object (first/last) instead of single string

### ❌ Old Schema 2 (`docker/kratos-1/identity.schema.json`)

```json
{
  "traits": {
    "email": "...",
    "name": "John Doe" // ✅ Better
  },
  "metadata_public": {
    "roles": ["ADMIN"] // ✅ Correct location
  }
}
```

**Problems:**

- No tenant membership support
- No subdomain field for registration
- Missing tenant_id fields

### ✅ New Schema (Correct for Multi-Tenant Membership)

```json
{
  "traits": {
    "email": "user@example.com",
    "name": "John Doe",
    "subdomain": "tenant1" // ✅ For registration flow
  },
  "metadata_public": {
    "tenant_id": "tenant-123", // ✅ Primary tenant (backward compat)
    "subdomain": "tenant1", // ✅ Primary subdomain (backward compat)
    "tenant_memberships": [
      // ✅ All tenant IDs user has access to
      "tenant-123",
      "tenant-456"
    ],
    "primary_tenant_id": "tenant-123", // ✅ User's default tenant
    "roles": ["SUPER_ADMIN"] // ✅ Global roles
  }
}
```

## Why This Schema?

### 1. Traits (User Input During Registration)

```json
"traits": {
  "email": "user@example.com",     // ✅ Required - login identifier
  "name": "John Doe",              // ✅ Optional - display name
  "subdomain": "tenant1"           // ✅ Optional - tenant assignment
}
```

**Purpose:**

- `email` - Login identifier, verification, recovery
- `name` - Display name (single string, not object)
- `subdomain` - Captured during registration for tenant assignment

**Flow:**

```
User registers at: http://tenant1.localhost:5173/signup
  ↓
Frontend includes: traits.subdomain = "tenant1"
  ↓
Webhook receives: identity.traits.subdomain
  ↓
Creates membership: user → tenant1
```

### 2. Metadata Public (Server-Managed, Cached in Session)

```json
"metadata_public": {
  "tenant_memberships": ["tenant-123", "tenant-456"],  // ✅ Cached for middleware
  "primary_tenant_id": "tenant-123",                   // ✅ User's default tenant
  "roles": ["SUPER_ADMIN"],                            // ✅ Global roles
  "tenant_id": "tenant-123",                           // ✅ Backward compatibility
  "subdomain": "tenant1"                               // ✅ Backward compatibility
}
```

**Purpose:**

- `tenant_memberships` - **Key field** for session-based validation (no DB hit)
- `primary_tenant_id` - User's default/preferred tenant
- `roles` - Global roles (SUPER_ADMIN, ADMIN)
- `tenant_id`, `subdomain` - Backward compatibility with old code

**Updated by:**

- `UserTenantMembershipService.updateKratosTenantMemberships()`
- Called after membership changes (add, remove, accept invitation)

## How It Works in Your Implementation

### Registration Flow

```typescript
// 1. User registers at tenant subdomain
const traits = {
  email: "user@example.com",
  name: "John Doe",
  subdomain: "tenant1", // ✅ Captured from URL
};

// 2. Kratos creates identity with traits
// 3. Webhook receives registration event
// 4. Backend creates membership entry
await membershipService.AddUserToTenant(userID, tenantID, "USER", "system");

// 5. Backend updates metadata_public
await membershipService.updateKratosTenantMemberships(userID);
// Sets: tenant_memberships = ["tenant-123"]
//       primary_tenant_id = "tenant-123"
```

### Session Validation (No DB Hit)

```go
// Middleware extracts from session
tenantMemberships := session.Identity.MetadataPublic["tenant_memberships"]

// Check if user has access to requested tenant
hasAccess := contains(tenantMemberships, requestedTenantID)
// ✅ No database query needed!
```

### Membership Changes

```typescript
// User accepts invitation to tenant2
await membershipService.AcceptTenantInvitation(userID, "tenant-456");

// Updates metadata_public
await membershipService.updateKratosTenantMemberships(userID);
// Sets: tenant_memberships = ["tenant-123", "tenant-456"]
//       primary_tenant_id = "tenant-123" (unchanged)

// Next login: session includes both tenants
// User can access both tenant1 and tenant2 ✅
```

## Field Purposes

| Field                | Location        | Purpose                      | Updated By                   |
| -------------------- | --------------- | ---------------------------- | ---------------------------- |
| `email`              | traits          | Login identifier             | User (registration)          |
| `name`               | traits          | Display name                 | User (registration/settings) |
| `subdomain`          | traits          | Tenant assignment hint       | User (registration)          |
| `tenant_memberships` | metadata_public | **Session-based validation** | Backend (membership service) |
| `primary_tenant_id`  | metadata_public | User's default tenant        | Backend (user preference)    |
| `roles`              | metadata_public | Global roles (SUPER_ADMIN)   | Backend (admin)              |
| `tenant_id`          | metadata_public | Backward compatibility       | Backend (legacy)             |

## Migration from Old Schema

### If Using Schema 1 (roles in traits)

**Problem:** Roles in wrong location

**Fix:**

```typescript
// Move roles from traits to metadata_public
const identity = await kratosClient.GetIdentity(userID);
const roles = identity.traits.roles || [];

await kratosClient.UpdateIdentity(userID, {
  traits: {
    email: identity.traits.email,
    name: `${identity.traits.name.first} ${identity.traits.name.last}`,
  },
  metadata_public: {
    roles: roles, // ✅ Move here
  },
});
```

### If Using Schema 2 (no tenant support)

**Problem:** Missing tenant fields

**Fix:**

```typescript
// Add tenant memberships to existing identities
const memberships = await db.GetUserTenantMemberships(userID);
const tenantIDs = memberships.map((m) => m.tenant_id);

await kratosClient.UpdateIdentity(userID, {
  metadata_public: {
    ...identity.metadata_public,
    tenant_memberships: tenantIDs, // ✅ Add
    primary_tenant_id: tenantIDs[0], // ✅ Add
  },
});
```

## Validation Rules

### Traits

- `email` - **Required**, must be valid email, unique globally
- `name` - Optional, 1-100 characters
- `subdomain` - Optional, used during registration only

### Metadata Public

- `tenant_memberships` - Array of strings (tenant IDs)
- `primary_tenant_id` - String (must be in tenant_memberships)
- `roles` - Array of strings (SUPER_ADMIN, ADMIN, etc.)

## Best Practices

### ✅ Do

1. **Always update metadata after membership changes**

   ```typescript
   await membershipService.AddUserToTenant(...)
   await membershipService.updateKratosTenantMemberships(userID) // ✅
   ```

2. **Use tenant_memberships for validation**

   ```go
   memberships := session.Identity.MetadataPublic["tenant_memberships"]
   // ✅ Fast, no DB query
   ```

3. **Keep traits minimal**

   - Only user-editable fields
   - Only fields needed for login/recovery

4. **Use metadata_public for server-managed data**
   - Tenant memberships
   - Roles
   - Preferences

### ❌ Don't

1. **Don't put roles in traits**

   ```json
   {
     "traits": {
       "roles": ["ADMIN"] // ❌ Wrong location
     }
   }
   ```

2. **Don't query DB for every request**

   ```go
   // ❌ Slow
   hasAccess := db.CheckUserTenantAccess(userID, tenantID)

   // ✅ Fast
   hasAccess := contains(session.tenant_memberships, tenantID)
   ```

3. **Don't forget to update metadata**
   ```typescript
   await membershipService.AddUserToTenant(...)
   // ❌ Missing: updateKratosTenantMemberships()
   // User won't have access until next manual update!
   ```

## Summary

The new schema supports:

✅ **Multi-tenant membership** - One user, multiple tenants  
✅ **Session-based validation** - No DB queries on every request  
✅ **Registration flow** - Subdomain captured in traits  
✅ **Global roles** - SUPER_ADMIN, ADMIN in metadata_public  
✅ **Backward compatibility** - Old tenant_id/subdomain fields preserved  
✅ **Standard Kratos patterns** - Email/password auth, verification, recovery

This is the correct schema for your implementation!
