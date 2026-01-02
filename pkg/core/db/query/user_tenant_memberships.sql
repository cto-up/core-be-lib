-- name: CreateUserTenantMembership :one
INSERT INTO core_user_tenant_memberships (
    user_id,
    tenant_id,
    roles,
    status,
    invited_by,
    invited_at,
    joined_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: GetUserTenantMembership :one
SELECT * FROM core_user_tenant_memberships
WHERE user_id = $1 AND tenant_id = $2
LIMIT 1;

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

-- name: UpdateUserTenantMembershipStatus :one
UPDATE core_user_tenant_memberships
SET status = $3, updated_at = clock_timestamp()
WHERE user_id = $1 AND tenant_id = $2
RETURNING *;

-- name: UpdateUserTenantMembershipRoles :one
UPDATE core_user_tenant_memberships
SET roles = $3, updated_at = clock_timestamp()
WHERE user_id = $1 AND tenant_id = $2
RETURNING *;

-- name: AddRoleToUserTenantMembership :one
UPDATE core_user_tenant_memberships
SET roles = array_append(roles, sqlc.arg(role)::TEXT), updated_at = clock_timestamp()
WHERE user_id = sqlc.arg(user_id) 
  AND tenant_id = sqlc.arg(tenant_id) 
  AND NOT (sqlc.arg(role)::TEXT = ANY(roles))
RETURNING *;

-- name: RemoveRoleFromUserTenantMembership :one
UPDATE core_user_tenant_memberships
SET roles = array_remove(roles, sqlc.arg(role)::TEXT), updated_at = clock_timestamp()
WHERE user_id = sqlc.arg(user_id) 
  AND tenant_id = sqlc.arg(tenant_id) 
  AND sqlc.arg(role)::TEXT = ANY(roles)
RETURNING *;

-- name: UpdateUserTenantMembershipJoinedAt :one
UPDATE core_user_tenant_memberships
SET joined_at = $3, status = 'active', updated_at = clock_timestamp()
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

-- name: ListPendingInvitations :many
SELECT 
    utm.*,
    t.name as tenant_name,
    t.subdomain
FROM core_user_tenant_memberships utm
JOIN core_tenants t ON utm.tenant_id = t.tenant_id
WHERE utm.user_id = $1 AND utm.status = 'pending'
ORDER BY utm.invited_at DESC;

-- name: GetUserTenantRoles :one
SELECT roles FROM core_user_tenant_memberships
WHERE user_id = $1 AND tenant_id = $2 AND status = 'active'
LIMIT 1;

-- name: CheckUserHasTenantRole :one
SELECT EXISTS(
    SELECT 1 FROM core_user_tenant_memberships
    WHERE user_id = sqlc.arg(user_id) 
      AND tenant_id = sqlc.arg(tenant_id) 
      AND status = 'active' 
      AND sqlc.arg(role)::TEXT = ANY(roles)
) as has_role;
