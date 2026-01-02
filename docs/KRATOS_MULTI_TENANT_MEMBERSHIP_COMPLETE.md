# Kratos Multi-Tenant Membership Implementation - Complete Guide

## Overview

This document describes the complete implementation of the multi-tenant membership pattern for Ory Kratos authentication, where:

- **One email = One Kratos identity** (globally unique)
- **Users can belong to multiple tenants** via membership table
- **Tenant memberships are cached in session** for efficient validation (no DB hit on every request)

## Architecture

### Database Layer

**Table: `core_user_tenant_memberships`**

```sql
- id (uuid, primary key)
- user_id (varchar, Kratos identity ID)
- tenant_id (varchar, references core_tenants)
- role (varchar: USER, ADMIN, OWNER)
- status (varchar: pending, active, suspended, removed)
- invited_by (varchar, nullable)
- invited_at (timestamptz, nullable)
- joined_at (timestamptz, nullable)
- created_at (timestamptz)
- updated_at (timestamptz)
```

**Indexes:**

- `idx_user_tenant_memberships_user_id`
- `idx_user_tenant_memberships_tenant_id`
- `idx_user_tenant_memberships_status`

### Session-Based Validation (No DB Hits)

The key optimization is storing tenant memberships in the Kratos session metadata, eliminating database queries on every request.

**Flow:**

1. **User Registration/Membership Change**

   - Membership entry created in database
   - `UserTenantMembershipService.updateKratosTenantMemberships()` updates Kratos `metadata_public`:
     ```json
     {
       "tenant_memberships": ["tenant-id-1", "tenant-id-2"],
       "primary_tenant_id": "tenant-id-1"
     }
     ```

2. **Session Creation (Login)**

   - Kratos creates session with identity metadata
   - `KratosAuthClient.VerifyIDToken()` extracts `tenant_memberships` from `metadata_public`
   - Adds to token claims

3. **Request Authentication**

   - `AuthMiddleware` calls `authProvider.VerifyToken()`
   - Extracts `TenantMemberships` from claims
   - Stores in gin context: `c.Set("tenant_memberships", []string{...})`

4. **Tenant Validation**
   - `KratosTenantMiddleware` reads `tenant_memberships` from context
   - Validates tenant access **without database query**
   - Only falls back to DB if memberships not in session (legacy sessions)

## Three Registration/Assignment Options

### Option 1: User Registers at Tenant Subdomain

**Flow:**

1. User visits `tenant1.example.com/register`
2. Frontend includes `subdomain` in registration traits
3. Kratos creates identity
4. Webhook handler receives registration event
5. `HandleRegistrationWebhook()` creates membership entry:
   ```go
   membershipService.AddUserToTenant(userID, tenantID, "USER", "system")
   ```
6. Updates Kratos metadata with tenant memberships

**Implementation:**

- `kratos_webhook_handler.go`: `HandleRegistrationWebhook()`
- Automatically adds user to tenant based on subdomain

### Option 2: Admin Invites Existing User by Email

**Flow:**

1. Admin calls `POST /api/v1/tenants/{tenantId}/members`
2. `InviteUserToTenant()` checks if user exists by email
3. If exists: Creates `pending` membership entry
4. User sees invitation in `GET /api/v1/users/me/tenants/pending`
5. User accepts: `POST /api/v1/users/me/tenants/{tenantId}/accept`
6. Status changes to `active`, metadata updated

**Implementation:**

- `tenant_membership_handler.go`: `InviteUserToTenant()`, `AcceptTenantInvitation()`
- `user_tenant_membership_service.go`: `InviteUserToTenant()`, `AcceptTenantInvitation()`

### Option 3: SUPER_ADMIN Directly Assigns User

**Flow:**

1. SUPER_ADMIN calls `POST /api/v1/tenants/{tenantId}/members`
2. `AddUserToTenant()` creates `active` membership immediately
3. No invitation/acceptance needed
4. Metadata updated immediately

**Implementation:**

- Same endpoints as Option 2, but with `active` status from start
- SUPER_ADMIN bypass logic in middleware

## API Endpoints

### User Endpoints (Self-Service)

```
GET    /api/v1/users/me/tenants              # List user's tenants
GET    /api/v1/users/me/tenants/pending      # List pending invitations
POST   /api/v1/users/me/tenants/{id}/accept  # Accept invitation
POST   /api/v1/users/me/tenants/{id}/reject  # Reject invitation
POST   /api/v1/users/me/primary-tenant       # Set primary tenant
```

### Admin Endpoints (Tenant Management)

```
GET    /api/v1/tenants/{id}/members           # List tenant members
POST   /api/v1/tenants/{id}/members           # Invite user to tenant
PATCH  /api/v1/tenants/{id}/members/{userId}  # Update member role
DELETE /api/v1/tenants/{id}/members/{userId}  # Remove member
```

## Key Components

### 1. UserTenantMembershipService

**Location:** `pkg/shared/service/user_tenant_membership_service.go`

**Key Methods:**

- `AddUserToTenant()` - Create active membership
- `InviteUserToTenant()` - Create pending invitation
- `AcceptTenantInvitation()` - Accept pending invitation
- `GetUserTenants()` - List user's active tenants
- `CheckUserTenantAccess()` - Validate access (fallback)
- `updateKratosTenantMemberships()` - Sync to Kratos metadata

### 2. KratosAuthClient

**Location:** `pkg/shared/auth/kratos/provider.go`

**Key Changes:**

- `VerifyIDToken()` - Extracts `tenant_memberships` from `metadata_public`
- Adds memberships to token claims

### 3. AuthMiddleware

**Location:** `pkg/shared/service/auth_middleware.go`

**Key Changes:**

- `setAuthenticatedUser()` - Stores `tenant_memberships` in context
- `GetAuthenticatedUser()` - Retrieves memberships from context

### 4. KratosTenantMiddleware

**Location:** `pkg/shared/service/kratos_tenant_middleware.go`

**Validation Logic:**

1. Check if SUPER_ADMIN → bypass validation
2. Get tenant ID from subdomain
3. **Check `tenant_memberships` from context** (session-based, no DB)
4. Fallback to database check if not in session
5. Final fallback to legacy metadata validation

### 5. KratosWebhookHandler

**Location:** `pkg/shared/service/kratos_webhook_handler.go`

**Key Changes:**

- `HandleRegistrationWebhook()` - Creates membership on registration
- Requires `MultitenantService` and `UserTenantMembershipService`

### 6. TenantMembershipHandler

**Location:** `pkg/core/api/tenant_membership_handler.go`

**Implements all API endpoints** for membership management

## SQLC Queries

**Location:** `pkg/core/db/query/user_tenant_memberships.sql`

**Key Queries:**

- `CreateUserTenantMembership` - Create membership
- `ListUserTenantMemberships` - Get user's tenants (with JOIN)
- `ListPendingInvitations` - Get pending invitations
- `CheckUserTenantAccess` - Validate access (EXISTS query)
- `UpdateUserTenantMembershipStatus` - Change status
- `UpdateUserTenantMembershipRole` - Change role
- `DeleteUserTenantMembership` - Remove membership

## Performance Optimization

### Session-Based Validation (Primary Path)

**No Database Queries:**

```
Request → AuthMiddleware → Extract tenant_memberships from session
       → KratosTenantMiddleware → Check memberships in memory
       → ✅ Access granted (0 DB queries)
```

**Benefits:**

- **Fast:** No DB roundtrip on every request
- **Scalable:** Works with load balancing
- **Consistent:** Session is source of truth

### Database Fallback (Legacy Sessions)

Only used when:

- Session created before membership feature
- Metadata not yet synced
- Session refresh needed

**Triggers metadata update** for next request.

## SUPER_ADMIN Support

SUPER_ADMIN users bypass all tenant validation:

```go
if IsSuperAdmin(c) {
    // Can access any tenant or root domain
    // No membership check required
    c.Next()
    return
}
```

**Checks:**

- Root domain (`www` or empty subdomain) → No tenant required
- Any tenant subdomain → Access granted without validation

## Migration Path

### From Metadata-Only to Membership Table

1. **Existing users** with `tenant_id` in metadata:

   - Middleware falls back to metadata validation
   - Works without migration

2. **New users** or **membership changes**:

   - Creates membership entry
   - Updates metadata with `tenant_memberships` array
   - Next login uses session-based validation

3. **Gradual migration**:
   - Run script to create membership entries for existing users
   - Update metadata for all users
   - Eventually remove metadata fallback

## Security Considerations

1. **Session Integrity**

   - Memberships stored in Kratos metadata (server-side)
   - Cannot be tampered by client
   - Validated on every session creation

2. **Membership Changes**

   - Immediately update Kratos metadata
   - User must re-login or refresh session to see changes
   - Consider implementing session invalidation

3. **Role-Based Access**

   - Membership table stores role (USER, ADMIN, OWNER)
   - Can be used for fine-grained permissions
   - Currently not enforced in middleware (future enhancement)

4. **Invitation Security**
   - Pending invitations require acceptance
   - Can be rejected by user
   - Inviter tracked in `invited_by` field

## Testing Checklist

- [ ] User registers at tenant subdomain → membership created
- [ ] Admin invites existing user → pending invitation created
- [ ] User accepts invitation → status changes to active
- [ ] User rejects invitation → membership deleted
- [ ] User switches primary tenant → metadata updated
- [ ] SUPER_ADMIN accesses any tenant → no validation
- [ ] User with memberships accesses allowed tenant → success
- [ ] User without membership accesses tenant → 403 Forbidden
- [ ] Legacy user with metadata only → fallback works
- [ ] Session-based validation → no DB queries

## Future Enhancements

1. **Role-Based Permissions**

   - Enforce role checks in middleware
   - Different permissions for USER/ADMIN/OWNER

2. **Session Invalidation**

   - Invalidate sessions on membership removal
   - Force re-login on role change

3. **Invitation Emails**

   - Send email when user invited
   - Include accept/reject links

4. **Audit Log**

   - Track membership changes
   - Log access attempts

5. **Tenant Switching UI**
   - Frontend component to switch between tenants
   - Update primary tenant preference

## Files Modified/Created

### Created

- `pkg/core/db/migration/000016_user_tenant_memberships.up.sql`
- `pkg/core/db/migration/000016_user_tenant_memberships.down.sql`
- `pkg/core/db/query/user_tenant_memberships.sql`
- `pkg/shared/service/user_tenant_membership_service.go`
- `pkg/core/api/tenant_membership_handler.go`
- `api/openapi/tenant-membership-api.yaml`

### Modified

- `pkg/shared/auth/provider.go` - Added `TenantMemberships` field
- `pkg/shared/auth/kratos/provider.go` - Extract memberships from metadata
- `pkg/shared/service/auth_middleware.go` - Store memberships in context
- `pkg/shared/service/kratos_tenant_middleware.go` - Session-based validation
- `pkg/shared/service/kratos_webhook_handler.go` - Create membership on registration

## Summary

This implementation provides:

- ✅ **One email, multiple tenants** via membership table
- ✅ **Efficient validation** using session-based memberships (no DB hits)
- ✅ **Three registration options** (subdomain, invitation, direct assignment)
- ✅ **SUPER_ADMIN bypass** for unrestricted access
- ✅ **Backward compatibility** with metadata-only approach
- ✅ **Complete API** for membership management
- ✅ **Scalable architecture** suitable for production

The key innovation is storing tenant memberships in the Kratos session metadata, eliminating database queries on every request while maintaining security and consistency.
