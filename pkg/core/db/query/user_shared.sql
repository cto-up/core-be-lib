-- name: GetSharedUserByID :one
SELECT * FROM core_users
WHERE id = $1
LIMIT 1;

-- name: GetSharedUserByTenantByID :one
SELECT 
    u.*,
    utm.roles as tenant_roles,
    utm.status as membership_status,
    utm.joined_at,
    utm.tenant_id
FROM core_users u
INNER JOIN core_user_tenant_memberships utm ON u.id = utm.user_id
WHERE u.id = $1 
    AND utm.tenant_id = $2
    AND utm.status = 'active'
LIMIT 1;

-- name: GetSharedUserByTenantByEmail :one
SELECT 
    u.*,
    utm.roles as tenant_roles,
    utm.status as membership_status,
    utm.joined_at,
    utm.tenant_id
FROM core_users u
INNER JOIN core_user_tenant_memberships utm ON u.id = utm.user_id
WHERE u.email = sqlc.arg(email)::text
    AND utm.tenant_id = sqlc.arg(tenant_id)
    AND utm.status = 'active'
LIMIT 1;

-- name: ListSharedUsersByTenant :many
SELECT 
    u.*,
    utm.roles as tenant_roles,
    utm.status as membership_status,
    utm.joined_at
FROM core_users u
INNER JOIN core_user_tenant_memberships utm ON u.id = utm.user_id
WHERE utm.tenant_id = sqlc.arg(tenant_id)
    AND utm.status = 'active'
    AND (
        email ILIKE sqlc.narg('search_prefix')::text || '%'
        OR sqlc.narg('search_prefix') IS NULL
    )
ORDER BY u.created_at
LIMIT $1
OFFSET $2;


-- name: CreateSharedUser :one
-- USED
INSERT INTO core_users (
  "id", "email", "profile", roles
) VALUES (
  $1, sqlc.arg(email)::text, $2, sqlc.arg(roles)::VARCHAR[]
)
RETURNING *;

-- name: CreateSharedUserWithTenant :one
-- USED
WITH new_user AS (
    INSERT INTO core_users (
        "id", "email", "profile"
    ) VALUES (
        $1, sqlc.arg(email)::text, $2
    )
    RETURNING *
),
new_membership AS (
    INSERT INTO core_user_tenant_memberships (
        user_id, 
        tenant_id, 
        roles,
        status,
        invited_by,
        invited_at,
        joined_at
    ) VALUES (
        $1,
        sqlc.arg(tenant_id),
        sqlc.arg(tenant_roles)::TEXT[],
        'active',
        sqlc.narg(invited_by),
        sqlc.narg(invited_at),
        NOW()
    )
    ON CONFLICT (user_id, tenant_id) 
    DO UPDATE SET
        status = 'active',
        roles = EXCLUDED.roles,
        invited_by = COALESCE(core_user_tenant_memberships.invited_by, EXCLUDED.invited_by),
        invited_at = COALESCE(core_user_tenant_memberships.invited_at, EXCLUDED.invited_at),
        joined_at = NOW(),
        updated_at = NOW()
    RETURNING roles as tenant_roles, status as membership_status, joined_at, tenant_id
)
SELECT 
    new_user.*,
    new_membership.tenant_roles,
    new_membership.membership_status,
    new_membership.joined_at,
    new_membership.tenant_id
FROM new_user
CROSS JOIN new_membership;

-- name: UpdateSharedProfileByTenant :one
UPDATE core_users 
SET profile = $1
WHERE core_users.id = $2
    AND EXISTS (
        SELECT 1 FROM core_user_tenant_memberships
        WHERE user_id = $2 
            AND core_user_tenant_memberships.tenant_id = sqlc.arg(tenant_id)
            AND status = 'active'
    )
RETURNING id;

-- name: UpdateSharedProfile :one
UPDATE core_users 
SET profile = $1
WHERE core_users.id = $2
RETURNING id;


-- name: UpdateSharedUser :one
UPDATE core_users 
SET 
    roles = sqlc.arg(roles)::VARCHAR[],
    profile = jsonb_set(
        profile, 
        '{name}', 
        to_jsonb(sqlc.arg(name)::text), 
        true
    )
WHERE id = $1
RETURNING id;



-- name: UpdateSharedUserByTenant :one
WITH updated_user AS (
    UPDATE core_users 
    SET 
        roles = sqlc.arg(roles)::VARCHAR[],
        profile = jsonb_set(
            profile, 
            '{name}', 
            to_jsonb(sqlc.arg(name)::text), 
            true
        )
    WHERE core_users.id = $1
        AND EXISTS (
            SELECT 1 FROM core_user_tenant_memberships
            WHERE user_id = $1 
                AND core_user_tenant_memberships.tenant_id = sqlc.arg(tenant_id)
                AND status = 'active'
        )
    RETURNING id
),
updated_membership AS (
    UPDATE core_user_tenant_memberships
    SET roles = sqlc.arg(tenant_roles)::TEXT[],
        updated_at = NOW()
    WHERE user_id = $1 
        AND tenant_id = sqlc.arg(tenant_id)
    RETURNING user_id
)
SELECT COALESCE(updated_user.id, updated_membership.user_id) as id
FROM updated_user
FULL OUTER JOIN updated_membership ON updated_user.id = updated_membership.user_id;

-- name: DeleteSharedUserByTenant :one
-- This removes the user's membership from the tenant
-- The user record itself remains (they may belong to other tenants)
UPDATE core_user_tenant_memberships
SET status = 'inactive',
    updated_at = NOW(),
    left_at = NOW()
WHERE user_id = $1 
    AND tenant_id = sqlc.arg(tenant_id)
RETURNING user_id as id;

-- name: DeleteSharedUser :one
-- This removes the user globally
-- The user record itself is deleted
DELETE FROM core_users
WHERE id = $1
RETURNING id;

-- name: AddSharedUserToTenant :one
-- Add an existing user to a tenant (insert or reactivate if soft-deleted)
INSERT INTO core_user_tenant_memberships (
    user_id,
    tenant_id,
    roles,
    status,
    invited_by,
    invited_at,
    joined_at
) VALUES (
    $1,
    sqlc.arg(tenant_id),
    sqlc.arg(tenant_roles)::TEXT[],
    sqlc.arg(status),
    sqlc.narg(invited_by),
    sqlc.narg(invited_at),
    NOW()
)
ON CONFLICT (user_id, tenant_id) 
DO UPDATE SET
    status = EXCLUDED.status,
    roles = EXCLUDED.roles,
    invited_by = COALESCE(core_user_tenant_memberships.invited_by, EXCLUDED.invited_by),
    invited_at = COALESCE(core_user_tenant_memberships.invited_at, EXCLUDED.invited_at),
    joined_at = NOW(),
    updated_at = NOW()
RETURNING *;

-- name: UpdateSharedUserRolesInTenant :one
-- Update a user's tenant-specific roles only
UPDATE core_user_tenant_memberships
SET roles = sqlc.arg(tenant_roles)::TEXT[],
    updated_at = NOW()
WHERE user_id = $1 
    AND tenant_id = sqlc.arg(tenant_id)
RETURNING *;

-- name: UpdateSharedUserGlobalRoles :one
-- Update a user's global roles only
UPDATE core_users
SET roles = sqlc.arg(roles)::VARCHAR[]
WHERE id = $1
RETURNING id;


-- name: UpdateUserTenantMembershipRoles :one
UPDATE core_user_tenant_memberships
SET roles = $3, updated_at = NOW()
WHERE user_id = $1 AND tenant_id = $2
RETURNING *;

-- name: ListUserTenantMemberships :many
SELECT 
    utm.*,
    t.name as tenant_name,
    t.subdomain
FROM core_user_tenant_memberships utm
JOIN core_tenants t ON utm.tenant_id = t.tenant_id
WHERE utm.user_id = $1 AND utm.status = $2
ORDER BY utm.created_at DESC;

-- name: ListTenantMembers :many
SELECT utm.*
FROM core_user_tenant_memberships utm
WHERE utm.tenant_id = $1 AND utm.status = $2
ORDER BY utm.created_at DESC;


-- name: ListSharedUsersByRoles :many
SELECT 
    id, 
    email, 
    profile, 
    roles, 
    created_at
FROM core_users
WHERE 
    -- Use GIN index for array overlap
    roles && sqlc.arg(requested_roles)::VARCHAR[]
    -- Optimize email search
    AND (
        email ILIKE sqlc.narg('search_prefix')::text || '%'
        OR sqlc.narg('search_prefix') IS NULL
    )
ORDER BY email ASC
LIMIT $1
OFFSET $2;


-- name: UpdateUserTenantMembershipStatus :one
UPDATE core_user_tenant_memberships
SET status = $3, updated_at = NOW()
WHERE user_id = $1 AND tenant_id = $2
RETURNING *;

-- name: CheckUserHasTenantRole :one
SELECT EXISTS(
    SELECT 1 FROM core_user_tenant_memberships
    WHERE user_id = sqlc.arg(user_id) 
      AND tenant_id = sqlc.arg(tenant_id) 
      AND status = 'active' 
      AND sqlc.arg(role)::TEXT = ANY(roles)
) as has_role;

-- name: RemoveSharedUserFromTenant :exec
-- Hard delete: completely remove user from tenant
DELETE FROM core_user_tenant_memberships
WHERE user_id = $1 
    AND tenant_id = sqlc.arg(tenant_id);

-- name: UpdateUserTenantMembershipJoinedAt :one
UPDATE core_user_tenant_memberships
SET joined_at = $3, status = 'active', updated_at = NOW()
WHERE user_id = $1 AND tenant_id = $2
RETURNING *;

-- name: GetUserTenantRoles :one
SELECT roles FROM core_user_tenant_memberships
WHERE user_id = $1 AND tenant_id = $2 AND status = 'active'
LIMIT 1;


-- name: AddRoleToUserTenantMembership :one
UPDATE core_user_tenant_memberships
SET roles = array_append(roles, sqlc.arg(role)::TEXT), updated_at = NOW()
WHERE user_id = sqlc.arg(user_id) 
  AND tenant_id = sqlc.arg(tenant_id) 
  AND NOT (sqlc.arg(role)::TEXT = ANY(roles))
RETURNING *;

-- name: RemoveRoleFromUserTenantMembership :one
UPDATE core_user_tenant_memberships
SET roles = array_remove(roles, sqlc.arg(role)::TEXT), updated_at = NOW()
WHERE user_id = sqlc.arg(user_id) 
  AND tenant_id = sqlc.arg(tenant_id) 
  AND sqlc.arg(role)::TEXT = ANY(roles)
RETURNING *;

-- name: CheckUserTenantAccess :one
SELECT EXISTS(
    SELECT 1 FROM core_user_tenant_memberships
    WHERE user_id = $1 AND tenant_id = $2 AND status = 'active'
) as has_access;


-- name: GetSharedUserWithAllTenants :one
-- Get user with all their tenant memberships
SELECT 
    u.id,
    u.email,
    u.profile,
    u.roles as global_roles,
    u.created_at,
    COALESCE(
        json_agg(
            json_build_object(
                'tenant_id', utm.tenant_id,
                'roles', utm.roles,
                'status', utm.status,
                'joined_at', utm.joined_at
            )
        ) FILTER (WHERE utm.tenant_id IS NOT NULL),
        '[]'
    ) as tenants
FROM core_users u
LEFT JOIN core_user_tenant_memberships utm 
    ON u.id = utm.user_id 
    AND utm.status = 'active'
LEFT JOIN core_tenants t 
    ON utm.tenant_id = t.tenant_id
WHERE u.id = $1
GROUP BY u.id;


-- name: GetSharedUserTenantMembership :one
SELECT * FROM core_user_tenant_memberships
WHERE user_id = $1 AND tenant_id = $2
LIMIT 1;

-- name: ListPendingInvitations :many
SELECT 
    utm.*,
    t.name as tenant_name,
    t.subdomain
FROM core_user_tenant_memberships utm
JOIN core_tenants t ON utm.tenant_id = t.tenant_id
WHERE utm.user_id = $1 AND utm.status = 'pending'
ORDER BY utm.invited_at DESC;

-- name: IsUserMemberOfTenant :one
-- Check if user is already an active member of a specific tenant
SELECT EXISTS(
    SELECT 1 FROM core_user_tenant_memberships
    WHERE user_id = $1 AND tenant_id = $2 AND status = 'active'
) as is_member;


-- name: GetUserByEmailGlobal :one
-- Get user by email across all tenants (for checking existence)
-- This returns the first user found with this email
SELECT DISTINCT ON (id) 
    id, 
    email, 
    profile, 
    created_at
FROM core_users
WHERE email = sqlc.arg(email)::text
LIMIT 1;

-- name: CountUserTenants :one
-- Count how many tenants a user belongs to
SELECT COUNT(DISTINCT tenant_id)::int
FROM core_user_tenant_memberships
WHERE user_id = $1 AND status = 'active';
