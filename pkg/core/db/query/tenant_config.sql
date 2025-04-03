-- name: GetTenantConfigByID :one
SELECT * FROM core_tenant_configs
WHERE id = $1 AND tenant_id = sqlc.arg('tenant_id')::text LIMIT 1;

-- name: GetTenantConfigByName :one
SELECT * FROM core_tenant_configs
WHERE name = $1 AND tenant_id = sqlc.arg('tenant_id')::text LIMIT 1;

-- name: ListTenantConfigs :many
SELECT * FROM core_tenant_configs
WHERE tenant_id = sqlc.arg('tenant_id')::text
  AND (UPPER(name) LIKE UPPER(sqlc.narg('like')) OR sqlc.narg('like') IS NULL)
ORDER BY
  CASE
            WHEN sqlc.arg('order')::text = 'asc' and sqlc.arg('sortBy')::text = 'name' THEN "name"
            WHEN sqlc.arg('order')::text = 'asc' and sqlc.arg('sortBy')::text = 'value' THEN "value"
        END ASC,
  CASE
            WHEN (NOT sqlc.arg('order')::text = 'asc') and sqlc.arg('sortBy')::text = 'name' THEN "name"
            WHEN (NOT sqlc.arg('order')::text = 'asc') and sqlc.arg('sortBy')::text = 'value' THEN "value"
        END DESC
LIMIT $1
OFFSET $2;

-- name: CreateTenantConfig :one
INSERT INTO core_tenant_configs (
  user_id, tenant_id, "name", "value"
) VALUES (
  $1,sqlc.arg('tenant_id')::text, $2, $3
)
RETURNING *;

-- name: UpdateTenantConfig :one
UPDATE core_tenant_configs 
SET 
    "name" = $2,
    "value" = $3
WHERE id = $1 and tenant_id = sqlc.arg('tenant_id')::text
RETURNING id
;

-- name: DeleteTenantConfig :one
DELETE FROM core_tenant_configs
WHERE id = $1 and tenant_id = sqlc.arg('tenant_id')::text
RETURNING id
;