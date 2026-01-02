-- name: CreateUserTenantMembership :one
INSERT INTO core_user_tenant_memberships (
    user_id,
    tenant_id,
    role,
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

-- name: UpdateUserTenantMembershipRole :one
UPDATE core_user_tenant_memberships
SET role = $3, updated_at = clock_timestamp()
WHERE user_id = $1 AND tenant_id = $2
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
