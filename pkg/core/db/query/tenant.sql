-- name: GetTenantByID :one
SELECT * FROM core_tenants
WHERE id = $1 LIMIT 1;

-- name: GetTenantByTenantID :one
SELECT * FROM core_tenants
WHERE tenant_id = $1 LIMIT 1;

-- name: GetTenantBySubdomain :one
SELECT * FROM core_tenants
WHERE subdomain = $1 LIMIT 1;

-- name: ListTenants :many
SELECT * FROM core_tenants
WHERE (UPPER(name) LIKE UPPER(sqlc.narg('like')) OR sqlc.narg('like') IS NULL)
ORDER BY
  CASE
            WHEN sqlc.arg('order')::text = 'asc' and sqlc.arg('sortBy')::text = 'tenant_id' THEN "tenant_id"
            WHEN sqlc.arg('order')::text = 'asc' and sqlc.arg('sortBy')::text = 'name' THEN "name"
            WHEN sqlc.arg('order')::text = 'asc' and sqlc.arg('sortBy')::text = 'subdomain' THEN "subdomain"
        END ASC,
  CASE
            WHEN (NOT sqlc.arg('order')::text = 'asc') and sqlc.arg('sortBy')::text = 'tenant_id' THEN "tenant_id"
            WHEN (NOT sqlc.arg('order')::text = 'asc') and sqlc.arg('sortBy')::text = 'name' THEN "name"
            WHEN (NOT sqlc.arg('order')::text = 'asc') and sqlc.arg('sortBy')::text = 'subdomain' THEN "subdomain"
        END DESC
LIMIT $1
OFFSET $2;

-- name: CreateTenant :one
INSERT INTO core_tenants (
  user_id, "tenant_id", "name", "subdomain", "enable_email_link_sign_in", "allow_password_sign_up", "allow_signup"
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: UpdateTenant :one
UPDATE core_tenants 
SET 
    "name" = $2,
    "subdomain" = $3,
    "enable_email_link_sign_in" = $4,
    "allow_password_sign_up" = $5,
    "allow_signup" = $6
WHERE id = $1
RETURNING id;

-- name: DeleteTenant :one
DELETE FROM core_tenants
WHERE id = $1
RETURNING id
;


-- name: UpdateTenantProfile :one
UPDATE core_tenants 
SET profile = $1
WHERE tenant_id = sqlc.arg(tenant_id)::text
RETURNING id
;

-- name: UpdateTenantFeatures :one
UPDATE core_tenants 
SET features = $1
WHERE id = $2
RETURNING id
;
