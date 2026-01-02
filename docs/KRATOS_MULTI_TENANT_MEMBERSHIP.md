# Multi-Tenant Membership Pattern (One Email, Multiple Tenants)

## Overview

This pattern allows a single user (one email/identity) to belong to multiple tenants. The user has **one Kratos identity** but can be a member of multiple organizations/tenants.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kratos Identity                           │
│  email: user@example.com                                     │
│  id: user-123                                                │
│  metadata_public: {                                          │
│    primary_tenant_id: "tenant-1",                           │
│    tenant_memberships: ["tenant-1", "tenant-2", "tenant-3"] │
│  }                                                           │
└─────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│              Application Database                            │
│                                                              │
│  user_tenant_memberships table:                             │
│  ┌──────────┬───────────┬─────────┬────────────┐           │
│  │ user_id  │ tenant_id │ role    │ status     │           │
│  ├──────────┼───────────┼─────────┼────────────┤           │
│  │ user-123 │ tenant-1  │ ADMIN   │ active     │           │
│  │ user-123 │ tenant-2  │ USER    │ active     │           │
│  │ user-123 │ tenant-3  │ USER    │ pending    │           │
│  └──────────┴───────────┴─────────┴────────────┘           │
└─────────────────────────────────────────────────────────────┘
```

## Database Schema

### User-Tenant Membership Table

```sql
-- core-be-lib/pkg/core/db/migration/000006_user_tenant_memberships.up.sql

CREATE TABLE core_user_tenant_memberships (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    user_id VARCHAR(128) NOT NULL,
    tenant_id VARCHAR(64) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'USER',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    invited_by VARCHAR(128),
    invited_at timestamptz,
    joined_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT user_tenant_memberships_pk PRIMARY KEY (id),
    CONSTRAINT user_tenant_memberships_unique UNIQUE (user_id, tenant_id),
    CONSTRAINT fk_tenant FOREIGN KEY (tenant_id) REFERENCES core_tenants(tenant_id) ON DELETE CASCADE
);

CREATE INDEX idx_user_tenant_memberships_user_id ON core_user_tenant_memberships(user_id);
CREATE INDEX idx_user_tenant_memberships_tenant_id ON core_user_tenant_memberships(tenant_id);
CREATE INDEX idx_user_tenant_memberships_status ON core_user_tenant_memberships(status);

CREATE TRIGGER update_user_tenant_memberships_modtime
BEFORE UPDATE ON core_user_tenant_memberships
FOR EACH ROW
EXECUTE FUNCTION update_modified_column();

-- Possible roles: USER, ADMIN, OWNER
-- Possible statuses: pending, active, suspended, removed
```

### SQLC Queries

```sql
-- core-be-lib/pkg/core/db/query/user_tenant_memberships.sql

-- name: CreateUserTenantMembership :one
INSERT INTO core_user_tenant_memberships (
    user_id,
    tenant_id,
    role,
    status,
    invited_by,
    invited_at
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetUserTenantMembership :one
SELECT * FROM core_user_tenant_memberships
WHERE user_id = $1 AND tenant_id = $2
LIMIT 1;

-- name: ListUserTenantMemberships :many
SELECT utm.*, t.name as tenant_name, t.subdomain
FROM core_user_tenant_memberships utm
JOIN core_tenants t ON utm.tenant_id = t.tenant_id
WHERE utm.user_id = $1 AND utm.status = $2
ORDER BY utm.created_at DESC;

-- name: ListTenantMembers :many
SELECT utm.*, u.email, u.name
FROM core_user_tenant_memberships utm
WHERE utm.tenant_id = $1 AND utm.status = $2
ORDER BY utm.created_at DESC;

-- name: UpdateUserTenantMembershipStatus :one
UPDATE core_user_tenant_memberships
SET status = $3, updated_at = clock_timestamp()
WHERE user_id = $1 AND tenant_id = $2
RETURNING *;

-- name: UpdateUserTenantMembershipRole :one
UPDATE core_user_tenant_memberships
SET role = $3, updated_at = clock_timestamp()
WHERE user_id = $1 AND tenant_id = $2
RETURNING *;

-- name: DeleteUserTenantMembership :exec
DELETE FROM core_user_tenant_memberships
WHERE user_id = $1 AND tenant_id = $2;

-- name: CheckUserTenantAccess :one
SELECT EXISTS(
    SELECT 1 FROM core_user_tenant_memberships
    WHERE user_id = $1 AND tenant_id = $2 AND status = 'active'
) as has_access;
```

## Backend Implementation

### 1. User Tenant Membership Service

**File**: `pkg/shared/service/user_tenant_membership_service.go`

```go
package service

import (
	"context"
	"fmt"
	"time"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/auth/kratos"
	"github.com/rs/zerolog/log"
)

type UserTenantMembershipService struct {
	store        *db.Store
	authProvider auth.AuthProvider
}

func NewUserTenantMembershipService(store *db.Store, authProvider auth.AuthProvider) *UserTenantMembershipService {
	return &UserTenantMembershipService{
		store:        store,
		authProvider: authProvider,
	}
}

// AddUserToTenant adds a user to a tenant with a specific role
func (s *UserTenantMembershipService) AddUserToTenant(
	ctx context.Context,
	userID string,
	tenantID string,
	role string,
	invitedBy string,
) error {
	// Create membership in database
	_, err := s.store.CreateUserTenantMembership(ctx, db.CreateUserTenantMembershipParams{
		UserID:    userID,
		TenantID:  tenantID,
		Role:      role,
		Status:    "active",
		InvitedBy: &invitedBy,
		InvitedAt: time.Now(),
		JoinedAt:  time.Now(),
	})

	if err != nil {
		return fmt.Errorf("failed to create membership: %w", err)
	}

	// Update Kratos metadata with tenant memberships
	err = s.updateKratosTenantMemberships(ctx, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update Kratos metadata")
		// Don't fail - membership is in database
	}

	log.Info().
		Str("user_id", userID).
		Str("tenant_id", tenantID).
		Str("role", role).
		Msg("User added to tenant")

	return nil
}

// RemoveUserFromTenant removes a user from a tenant
func (s *UserTenantMembershipService) RemoveUserFromTenant(
	ctx context.Context,
	userID string,
	tenantID string,
) error {
	// Update status to removed
	_, err := s.store.UpdateUserTenantMembershipStatus(ctx, db.UpdateUserTenantMembershipStatusParams{
		UserID:   userID,
		TenantID: tenantID,
		Status:   "removed",
	})

	if err != nil {
		return fmt.Errorf("failed to remove membership: %w", err)
	}

	// Update Kratos metadata
	err = s.updateKratosTenantMemberships(ctx, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update Kratos metadata")
	}

	log.Info().
		Str("user_id", userID).
		Str("tenant_id", tenantID).
		Msg("User removed from tenant")

	return nil
}

// GetUserTenants returns all tenants a user belongs to
func (s *UserTenantMembershipService) GetUserTenants(
	ctx context.Context,
	userID string,
) ([]db.ListUserTenantMembershipsRow, error) {
	return s.store.ListUserTenantMemberships(ctx, db.ListUserTenantMembershipsParams{
		UserID: userID,
		Status: "active",
	})
}

// GetTenantMembers returns all members of a tenant
func (s *UserTenantMembershipService) GetTenantMembers(
	ctx context.Context,
	tenantID string,
) ([]db.CoreUserTenantMembership, error) {
	return s.store.ListTenantMembers(ctx, db.ListTenantMembersParams{
		TenantID: tenantID,
		Status:   "active",
	})
}

// CheckUserTenantAccess checks if a user has access to a tenant
func (s *UserTenantMembershipService) CheckUserTenantAccess(
	ctx context.Context,
	userID string,
	tenantID string,
) (bool, error) {
	result, err := s.store.CheckUserTenantAccess(ctx, db.CheckUserTenantAccessParams{
		UserID:   userID,
		TenantID: tenantID,
	})

	if err != nil {
		return false, err
	}

	return result, nil
}

// InviteUserToTenant creates a pending membership invitation
func (s *UserTenantMembershipService) InviteUserToTenant(
	ctx context.Context,
	email string,
	tenantID string,
	role string,
	invitedBy string,
) error {
	// Check if user exists
	authClient := s.authProvider.GetAuthClient()
	user, err := authClient.GetUserByEmail(ctx, email)

	if err != nil {
		// User doesn't exist - create pending invitation
		// Store in separate invitations table or send email
		return fmt.Errorf("user not found, invitation email should be sent")
	}

	// User exists - create pending membership
	_, err = s.store.CreateUserTenantMembership(ctx, db.CreateUserTenantMembershipParams{
		UserID:    user.UID,
		TenantID:  tenantID,
		Role:      role,
		Status:    "pending",
		InvitedBy: &invitedBy,
		InvitedAt: time.Now(),
	})

	if err != nil {
		return fmt.Errorf("failed to create invitation: %w", err)
	}

	log.Info().
		Str("email", email).
		Str("user_id", user.UID).
		Str("tenant_id", tenantID).
		Msg("User invited to tenant")

	return nil
}

// AcceptTenantInvitation accepts a pending invitation
func (s *UserTenantMembershipService) AcceptTenantInvitation(
	ctx context.Context,
	userID string,
	tenantID string,
) error {
	// Update status to active
	_, err := s.store.UpdateUserTenantMembershipStatus(ctx, db.UpdateUserTenantMembershipStatusParams{
		UserID:   userID,
		TenantID: tenantID,
		Status:   "active",
	})

	if err != nil {
		return fmt.Errorf("failed to accept invitation: %w", err)
	}

	// Update Kratos metadata
	err = s.updateKratosTenantMemberships(ctx, userID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update Kratos metadata")
	}

	log.Info().
		Str("user_id", userID).
		Str("tenant_id", tenantID).
		Msg("User accepted tenant invitation")

	return nil
}

// updateKratosTenantMemberships updates the user's Kratos metadata with current tenant memberships
func (s *UserTenantMembershipService) updateKratosTenantMemberships(
	ctx context.Context,
	userID string,
) error {
	// Get all active memberships
	memberships, err := s.GetUserTenants(ctx, userID)
	if err != nil {
		return err
	}

	// Extract tenant IDs
	tenantIDs := make([]string, len(memberships))
	var primaryTenantID string
	for i, m := range memberships {
		tenantIDs[i] = m.TenantID
		if i == 0 {
			primaryTenantID = m.TenantID
		}
	}

	// Update Kratos metadata
	authClient := s.authProvider.GetAuthClient()
	kratosClient, ok := authClient.(*kratos.KratosAuthClient)
	if !ok {
		return fmt.Errorf("auth provider is not Kratos")
	}

	// Get existing identity
	existing, _, err := kratosClient.adminClient.IdentityAPI.GetIdentity(ctx, userID).Execute()
	if err != nil {
		return err
	}

	// Update metadata_public
	metadataPublic := existing.MetadataPublic
	if metadataPublic == nil {
		metadataPublic = make(map[string]interface{})
	}

	metadataPublic["tenant_memberships"] = tenantIDs
	metadataPublic["primary_tenant_id"] = primaryTenantID

	// Update identity
	state := ""
	if existing.State != nil {
		state = string(*existing.State)
	}

	updateBody := *ory.NewUpdateIdentityBody(existing.SchemaId, state, existing.Traits)
	updateBody.MetadataPublic = metadataPublic

	_, _, err = kratosClient.adminClient.IdentityAPI.UpdateIdentity(ctx, userID).
		UpdateIdentityBody(updateBody).
		Execute()

	return err
}

// SwitchPrimaryTenant changes the user's primary tenant
func (s *UserTenantMembershipService) SwitchPrimaryTenant(
	ctx context.Context,
	userID string,
	tenantID string,
) error {
	// Verify user has access to this tenant
	hasAccess, err := s.CheckUserTenantAccess(ctx, userID, tenantID)
	if err != nil {
		return err
	}

	if !hasAccess {
		return fmt.Errorf("user does not have access to tenant")
	}

	// Update Kratos metadata with new primary tenant
	authClient := s.authProvider.GetAuthClient()
	kratosClient, ok := authClient.(*kratos.KratosAuthClient)
	if !ok {
		return fmt.Errorf("auth provider is not Kratos")
	}

	existing, _, err := kratosClient.adminClient.IdentityAPI.GetIdentity(ctx, userID).Execute()
	if err != nil {
		return err
	}

	metadataPublic := existing.MetadataPublic
	if metadataPublic == nil {
		metadataPublic = make(map[string]interface{})
	}

	metadataPublic["primary_tenant_id"] = tenantID

	state := ""
	if existing.State != nil {
		state = string(*existing.State)
	}

	updateBody := *ory.NewUpdateIdentityBody(existing.SchemaId, state, existing.Traits)
	updateBody.MetadataPublic = metadataPublic

	_, _, err = kratosClient.adminClient.IdentityAPI.UpdateIdentity(ctx, userID).
		UpdateIdentityBody(updateBody).
		Execute()

	if err != nil {
		return err
	}

	log.Info().
		Str("user_id", userID).
		Str("tenant_id", tenantID).
		Msg("User switched primary tenant")

	return nil
}
```

### 2. Update Tenant Middleware

**File**: `pkg/shared/service/kratos_tenant_middleware.go`

Update to check membership table:

```go
// MiddlewareFunc validates tenant context from subdomain and user membership
func (ktm *KratosTenantMiddleware) MiddlewareFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract subdomain from request
		subdomain, err := utils.GetSubdomain(c)
		if err != nil {
			log.Error().Err(err).Msg("Failed to extract subdomain")
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid subdomain"})
			c.Abort()
			return
		}

		// Check if user is SUPER_ADMIN
		isSuperAdmin := IsSuperAdmin(c)

		// Skip tenant validation for root domain or SUPER_ADMIN
		if subdomain == "" || subdomain == "www" {
			if isSuperAdmin {
				log.Debug().
					Str("user_id", c.GetString(auth.AUTH_USER_ID)).
					Msg("SUPER_ADMIN accessing root domain - no tenant required")
			}
			c.Next()
			return
		}

		// Get tenant ID from database using subdomain
		tenantID, err := ktm.multitenantService.GetFirebaseTenantID(c, subdomain)
		if err != nil {
			log.Error().Err(err).Str("subdomain", subdomain).Msg("Tenant not found")
			c.JSON(http.StatusNotFound, gin.H{
				"status":  http.StatusNotFound,
				"message": "Tenant not found",
			})
			c.Abort()
			return
		}

		// SUPER_ADMIN can access any tenant
		if isSuperAdmin {
			c.Set(auth.AUTH_TENANT_ID_KEY, tenantID)
			c.Set("tenant_subdomain", subdomain)

			log.Debug().
				Str("tenant_id", tenantID).
				Str("subdomain", subdomain).
				Str("user_id", c.GetString(auth.AUTH_USER_ID)).
				Msg("SUPER_ADMIN accessing tenant - validation bypassed")

			c.Next()
			return
		}

		// Check if user has membership in this tenant
		userID := c.GetString(auth.AUTH_USER_ID)
		hasAccess, err := ktm.membershipService.CheckUserTenantAccess(c.Request.Context(), userID, tenantID)

		if err != nil {
			log.Error().Err(err).Msg("Failed to check tenant access")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal error"})
			c.Abort()
			return
		}

		if !hasAccess {
			log.Error().
				Str("user_id", userID).
				Str("tenant_id", tenantID).
				Str("subdomain", subdomain).
				Msg("User does not have access to tenant")

			c.JSON(http.StatusForbidden, gin.H{
				"status":  http.StatusForbidden,
				"message": "Access denied: no membership in this tenant",
			})
			c.Abort()
			return
		}

		// Set tenant context for downstream handlers
		c.Set(auth.AUTH_TENANT_ID_KEY, tenantID)
		c.Set("tenant_subdomain", subdomain)

		log.Debug().
			Str("tenant_id", tenantID).
			Str("subdomain", subdomain).
			Str("user_id", userID).
			Msg("Tenant access validated via membership")

		c.Next()
	}
}
```

## Frontend Implementation

### 1. Tenant Selector Component

**File**: `src/components/TenantSelector.vue`

```vue
<script setup lang="ts">
import { ref, computed, onMounted } from "vue";
import { useRouter } from "vue-router";
import { useKratosAuth } from "@/composables/kratos-auth.composable";
import axios from "axios";

interface TenantMembership {
  tenant_id: string;
  tenant_name: string;
  subdomain: string;
  role: string;
  status: string;
}

const router = useRouter();
const { session } = useKratosAuth();

const tenants = ref<TenantMembership[]>([]);
const currentTenant = ref<string | null>(null);
const isLoading = ref(false);

const primaryTenantId = computed(() => {
  return session.value?.identity.metadata_public?.primary_tenant_id || null;
});

const tenantMemberships = computed(() => {
  return session.value?.identity.metadata_public?.tenant_memberships || [];
});

onMounted(async () => {
  await loadTenants();
});

async function loadTenants() {
  try {
    isLoading.value = true;
    const response = await axios.get("/api/v1/users/me/tenants");
    tenants.value = response.data;

    // Set current tenant from URL
    const hostname = window.location.hostname;
    const parts = hostname.split(".");
    if (parts.length > 2) {
      currentTenant.value = parts[0];
    }
  } catch (error) {
    console.error("Failed to load tenants:", error);
  } finally {
    isLoading.value = false;
  }
}

async function switchTenant(subdomain: string) {
  // Redirect to tenant subdomain
  const protocol = window.location.protocol;
  const domain = window.location.hostname.split(".").slice(-2).join(".");
  const port = window.location.port ? `:${window.location.port}` : "";

  window.location.href = `${protocol}//${subdomain}.${domain}${port}`;
}

async function setPrimaryTenant(tenantId: string) {
  try {
    await axios.post("/api/v1/users/me/primary-tenant", {
      tenant_id: tenantId,
    });
    await loadTenants();
  } catch (error) {
    console.error("Failed to set primary tenant:", error);
  }
}
</script>

<template>
  <div class="tenant-selector">
    <h3>Your Organizations</h3>

    <div v-if="isLoading">Loading...</div>

    <div v-else class="tenant-list">
      <div
        v-for="tenant in tenants"
        :key="tenant.tenant_id"
        class="tenant-item"
        :class="{ active: tenant.subdomain === currentTenant }"
      >
        <div class="tenant-info">
          <h4>{{ tenant.tenant_name }}</h4>
          <p>{{ tenant.subdomain }}.app.com</p>
          <span class="role-badge">{{ tenant.role }}</span>
          <span
            v-if="tenant.tenant_id === primaryTenantId"
            class="primary-badge"
          >
            Primary
          </span>
        </div>

        <div class="tenant-actions">
          <button
            v-if="tenant.subdomain !== currentTenant"
            @click="switchTenant(tenant.subdomain)"
          >
            Switch
          </button>

          <button
            v-if="tenant.tenant_id !== primaryTenantId"
            @click="setPrimaryTenant(tenant.tenant_id)"
          >
            Set as Primary
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tenant-selector {
  padding: 1rem;
}

.tenant-list {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.tenant-item {
  border: 1px solid #ddd;
  border-radius: 8px;
  padding: 1rem;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.tenant-item.active {
  border-color: #3b82f6;
  background-color: #eff6ff;
}

.role-badge {
  display: inline-block;
  padding: 0.25rem 0.5rem;
  background-color: #e5e7eb;
  border-radius: 4px;
  font-size: 0.875rem;
}

.primary-badge {
  display: inline-block;
  padding: 0.25rem 0.5rem;
  background-color: #3b82f6;
  color: white;
  border-radius: 4px;
  font-size: 0.875rem;
  margin-left: 0.5rem;
}
</style>
```

### 2. Update useTenant Composable

**File**: `src/composables/useTenant.ts`

```typescript
import { computed } from "vue";
import { useKratosAuth } from "./kratos-auth.composable";

export function useTenant() {
  const { session } = useKratosAuth();

  const primaryTenantId = computed(() => {
    return session.value?.identity.metadata_public?.primary_tenant_id || null;
  });

  const tenantMemberships = computed(() => {
    return session.value?.identity.metadata_public?.tenant_memberships || [];
  });

  const currentSubdomain = computed(() => {
    const hostname = window.location.hostname;
    const parts = hostname.split(".");

    if (hostname === "localhost" || /^\d+\.\d+\.\d+\.\d+$/.test(hostname)) {
      return null;
    }

    if (parts.length > 2) {
      return parts[0];
    }

    return null;
  });

  const hasTenant = computed(() => {
    return tenantMemberships.value.length > 0;
  });

  const hasMultipleTenants = computed(() => {
    return tenantMemberships.value.length > 1;
  });

  const isMemberOfTenant = (tenantId: string) => {
    return tenantMemberships.value.includes(tenantId);
  };

  return {
    primaryTenantId,
    tenantMemberships,
    currentSubdomain,
    hasTenant,
    hasMultipleTenants,
    isMemberOfTenant,
  };
}
```

## Usage Examples

### Register User (First Tenant)

```go
// User registers at tenant1.app.com
user, err := kratosClient.CreateUser(ctx, &auth.UserToCreate{}.
    Email("john@example.com").
    Password("password123"))

// Add to tenant1
err = membershipService.AddUserToTenant(ctx, user.UID, "tenant1-id", "ADMIN", "system")
```

### Invite User to Additional Tenant

```go
// Admin invites john@example.com to tenant2
err := membershipService.InviteUserToTenant(
    ctx,
    "john@example.com",
    "tenant2-id",
    "USER",
    "admin-user-id",
)

// User accepts invitation
err = membershipService.AcceptTenantInvitation(ctx, "john-user-id", "tenant2-id")
```

### Check Access

```go
// Check if user can access tenant
hasAccess, err := membershipService.CheckUserTenantAccess(ctx, userID, tenantID)
if !hasAccess {
    return errors.New("access denied")
}
```

### List User's Tenants

```go
tenants, err := membershipService.GetUserTenants(ctx, userID)
// Returns all tenants user belongs to
```

## Pros and Cons

### ✅ Pros

- One email = one identity (simpler)
- User can belong to multiple tenants
- Easy to invite existing users
- Single password across all tenants
- Can switch between tenants easily
- Cleaner data model

### ⚠️ Cons

- Requires membership table
- More complex access control
- Need tenant selector UI
- User might confuse which tenant they're in

## Recommendation

**Use this pattern** (one email, multiple tenant memberships) because:

1. **Simpler identity management** - One Kratos identity per person
2. **Better UX** - Users can switch between organizations
3. **Common pattern** - Similar to Slack, GitHub, etc.
4. **Easier invitations** - Just add membership, no new identity
5. **Single sign-on** - One login works across all tenants

This is the **recommended approach** for most multi-tenant applications.
