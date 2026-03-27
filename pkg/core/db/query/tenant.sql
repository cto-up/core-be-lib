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
AND (reseller_id = sqlc.narg('reseller_id') OR sqlc.narg('reseller_id') IS NULL)
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
  user_id, "tenant_id", "name", "subdomain", "enable_email_link_sign_in", "allow_password_sign_up", "allow_sign_up", "reseller_id", "is_reseller", "contract_end_date", "is_disabled"
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
RETURNING *;

-- name: UpdateTenant :one
UPDATE core_tenants
SET
    "name" = $2,
    "subdomain" = $3,
    "enable_email_link_sign_in" = $4,
    "allow_password_sign_up" = $5,
    "allow_sign_up" = $6,
    "is_reseller" = $7,
    "contract_end_date" = $8,
    "is_disabled" = $9
WHERE id = $1
RETURNING id;

-- name: DisableTenant :exec
UPDATE core_tenants SET is_disabled = true, updated_at = NOW()
WHERE tenant_id = $1;

-- name: EnableTenant :exec
UPDATE core_tenants SET is_disabled = false, updated_at = NOW()
WHERE tenant_id = $1;

-- name: GetExpiredEnabledTenants :many
SELECT * FROM core_tenants
WHERE contract_end_date IS NOT NULL
  AND contract_end_date < NOW()
  AND is_disabled = false;

-- name: DeleteTenant :one
DELETE FROM core_tenants
WHERE id = $1
RETURNING id
;


-- name: ListResellerTenants :many
WITH reseller AS (
    SELECT t.tenant_id FROM core_tenants t
    INNER JOIN core_user_tenant_memberships utm ON utm.tenant_id = t.tenant_id
    WHERE utm.user_id = sqlc.arg('user_id')
    AND t.is_reseller = true
    AND 'CUSTOMER_ADMIN' = ANY(utm.roles)
)
SELECT ct.* FROM core_tenants ct
WHERE ct.tenant_id IN (SELECT tenant_id FROM reseller)
   OR ct.reseller_id IN (SELECT tenant_id FROM reseller)
ORDER BY ct.name ASC;

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
