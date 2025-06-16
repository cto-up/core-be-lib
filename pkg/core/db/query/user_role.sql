UPDATE core_users 
SET roles = CASE 
    WHEN roles IS NULL THEN ARRAY[sqlc.arg(role_name)::VARCHAR(20)]
    WHEN sqlc.arg(role_name)::VARCHAR(20) = ANY(roles) THEN roles -- Role already exists, no change
    ELSE array_append(roles, sqlc.arg(role_name)::VARCHAR(20))
END
WHERE id = sqlc.arg(user_id)::VARCHAR
AND tenant_id = sqlc.arg(tenant_id)::VARCHAR
RETURNING id, roles, 
    CASE 
        WHEN sqlc.arg(role_name)::VARCHAR(20) = ANY(roles) THEN true 
        ELSE false 
    END as role_assigned;

-- name: UnassignRole :one
-- Removes a role from a user if it exists
-- Returns the updated roles array
UPDATE core_users 
SET roles = CASE 
    WHEN roles IS NULL THEN NULL
    WHEN NOT (sqlc.arg(role_name)::VARCHAR(20) = ANY(roles)) THEN roles -- Role doesn't exist, no change
    ELSE array_remove(roles, sqlc.arg(role_name)::VARCHAR(20))
END
WHERE id = sqlc.arg(user_id)::VARCHAR
AND tenant_id = sqlc.arg(tenant_id)::VARCHAR
RETURNING id, roles,
    CASE 
        WHEN roles IS NULL OR NOT (sqlc.arg(role_name)::VARCHAR(20) = ANY(roles)) THEN true
        ELSE false 
    END as role_unassigned;

-- name: AssignRoleWithRowsAffected :execrows
-- Alternative version that returns the number of rows affected
-- Useful if you just need to know if the operation succeeded
UPDATE core_users 
SET roles = CASE 
    WHEN roles IS NULL THEN ARRAY[sqlc.arg(role_name)::VARCHAR(20)]
    WHEN sqlc.arg(role_name)::VARCHAR(20) = ANY(roles) THEN roles
    ELSE array_append(roles, sqlc.arg(role_name)::VARCHAR(20))
END
WHERE id = sqlc.arg(user_id)::VARCHAR
AND tenant_id = sqlc.arg(tenant_id)::VARCHAR;

-- name: UnassignRoleWithRowsAffected :execrows
-- Alternative version that returns the number of rows affected
UPDATE core_users 
SET roles = CASE 
    WHEN roles IS NULL THEN NULL
    WHEN NOT (sqlc.arg(role_name)::VARCHAR(20) = ANY(roles)) THEN roles
    ELSE array_remove(roles, sqlc.arg(role_name)::VARCHAR(20))
END
WHERE id = sqlc.arg(user_id)::VARCHAR
AND tenant_id = sqlc.arg(tenant_id)::VARCHAR;

-- name: CheckUserHasRole :one
-- Check if a user has a specific role
SELECT 
    id,
    CASE 
        WHEN roles IS NULL THEN false
        WHEN sqlc.arg(role_name)::VARCHAR(20) = ANY(roles) THEN true
        ELSE false
    END as has_role,
    roles
FROM core_users
WHERE id = sqlc.arg(user_id)::VARCHAR
AND tenant_id = sqlc.arg(tenant_id)::VARCHAR
LIMIT 1;

-- name: GetUserRoles :one
-- Get all roles for a specific user
SELECT 
    id,
    COALESCE(roles, ARRAY[]::VARCHAR[]) as roles
FROM core_users
WHERE id = sqlc.arg(user_id)::VARCHAR
AND tenant_id = sqlc.arg(tenant_id)::VARCHAR
LIMIT 1;

-- name: AssignMultipleRoles :one
-- Assign multiple roles to a user at once
-- Handles duplicates automatically
UPDATE core_users 
SET roles = (
    SELECT ARRAY_AGG(DISTINCT role_name ORDER BY role_name)
    FROM (
        SELECT unnest(COALESCE(roles, ARRAY[]::VARCHAR[])) as role_name
        UNION
        SELECT unnest(sqlc.arg(role_names)::VARCHAR[]) as role_name
    ) combined_roles
)
WHERE id = sqlc.arg(user_id)::VARCHAR
AND tenant_id = sqlc.arg(tenant_id)::VARCHAR
RETURNING id, roles;

-- name: UnassignMultipleRoles :one
-- Remove multiple roles from a user at once
UPDATE core_users 
SET roles = (
    SELECT CASE 
        WHEN array_length(remaining_roles, 1) > 0 THEN remaining_roles
        ELSE NULL
    END
    FROM (
        SELECT ARRAY_AGG(role_name) as remaining_roles
        FROM unnest(COALESCE(roles, ARRAY[]::VARCHAR[])) as role_name
        WHERE NOT (role_name = ANY(sqlc.arg(role_names)::VARCHAR[]))
    ) filtered
)
WHERE id = sqlc.arg(user_id)::VARCHAR
AND tenant_id = sqlc.arg(tenant_id)::VARCHAR
RETURNING id, roles;

-- name: SetUserRoles :one
-- Replace all user roles with a new set of roles
-- Use this when you want to completely override existing roles
UPDATE core_users 
SET roles = CASE 
    WHEN sqlc.arg(role_names)::VARCHAR[] IS NULL OR array_length(sqlc.arg(role_names)::VARCHAR[], 1) = 0 THEN NULL
    ELSE sqlc.arg(role_names)::VARCHAR[]
END
WHERE id = sqlc.arg(user_id)::VARCHAR
AND tenant_id = sqlc.arg(tenant_id)::VARCHAR
RETURNING id, roles;

-- name: GetUsersWithRole :many
-- Get all users that have a specific role
SELECT 
    id,
    email,
    profile,
    roles,
    created_at,
    tenant_id
FROM core_users
WHERE sqlc.arg(role_name)::VARCHAR(20) = ANY(roles)
AND tenant_id = sqlc.arg(tenant_id)::VARCHAR
ORDER BY id;

-- name: GetUsersWithAnyRole :many
-- Get all users that have any of the specified roles
SELECT 
    id,
    email,
    profile,
    roles,
    created_at,
    tenant_id
FROM core_users
WHERE roles && sqlc.arg(role_names)::VARCHAR[] -- Array overlap operator
AND tenant_id = sqlc.arg(tenant_id)::VARCHAR
ORDER BY id;