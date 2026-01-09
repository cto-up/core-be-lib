-- name: GetUserByTenantByID :one
SELECT * FROM core_users
WHERE id = $1
AND tenant_id = sqlc.arg(tenant_id)::text
LIMIT 1;

-- name: GetUserByTenantByEmail :one
SELECT * FROM core_users
WHERE email = sqlc.arg(email)::text
AND tenant_id = sqlc.arg(tenant_id)::text
LIMIT 1;

-- name: ListUsersByTenant :many
SELECT * FROM core_users
WHERE (UPPER(email) LIKE UPPER(sqlc.narg('like')) OR sqlc.narg('like') IS NULL)
AND tenant_id = sqlc.arg(tenant_id)::text
ORDER BY created_at
LIMIT $1
OFFSET $2;

-- name: CreateUserByTenant :one
INSERT INTO core_users (
  "id", "email", "profile", roles, "tenant_id"
) VALUES (
  $1, sqlc.arg(email)::text, $2, sqlc.arg(roles)::VARCHAR[], sqlc.arg(tenant_id)::text
)
RETURNING *;

-- name: UpdateProfileByTenant :one
UPDATE core_users 
SET profile = $1
WHERE id = $2
AND tenant_id = sqlc.arg(tenant_id)::text
RETURNING id
;

-- name: UpdateUserByTenant :one
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
AND tenant_id = sqlc.arg(tenant_id)::text
RETURNING id;

-- name: DeleteUserByTenant :one
DELETE FROM core_users
WHERE id = $1
AND tenant_id = sqlc.arg(tenant_id)::text
RETURNING id
;