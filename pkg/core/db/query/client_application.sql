-- name: CreateClientApplication :one
INSERT INTO core_client_applications (
  name, description, tenant_id, created_by
) VALUES (
  $1, $2, sqlc.narg('tenant_id')::varchar, $3
)
RETURNING *;

-- name: GetClientApplicationByID :one
SELECT * FROM core_client_applications
WHERE id = $1 AND (tenant_id = sqlc.narg('tenant_id')::varchar OR tenant_id IS NULL)
LIMIT 1;

-- name: ListClientApplications :many
SELECT * 
FROM core_client_applications
WHERE (tenant_id = sqlc.narg('tenant_id')::varchar OR tenant_id IS NULL)
  AND (sqlc.narg('include_inactive')::boolean OR active = true)
  AND (UPPER(name) LIKE UPPER(sqlc.narg('like')) OR sqlc.narg('like') IS NULL)
ORDER BY
  CASE WHEN sqlc.arg('sort_by')::TEXT = 'name' AND sqlc.arg('order')::TEXT = 'asc' THEN name END ASC,
  CASE WHEN sqlc.arg('sort_by')::TEXT = 'name' AND sqlc.arg('order')::TEXT != 'asc' THEN name END DESC,
  CASE WHEN sqlc.arg('sort_by')::TEXT = 'created_at' AND sqlc.arg('order')::TEXT = 'asc' THEN created_at END ASC,
  CASE WHEN sqlc.arg('sort_by')::TEXT = 'created_at' AND sqlc.arg('order')::TEXT != 'asc' THEN created_at END DESC
LIMIT $1
OFFSET $2;

-- name: UpdateClientApplication :one
UPDATE core_client_applications
SET 
  name = $2,
  description = $3,
  active = $4
WHERE id = $1 AND (tenant_id = sqlc.narg('tenant_id')::varchar OR tenant_id IS NULL)
RETURNING *;

-- name: DeactivateClientApplication :one
UPDATE core_client_applications 
SET active = false
WHERE id = $1 AND (tenant_id = sqlc.narg('tenant_id')::varchar OR tenant_id IS NULL)
RETURNING id;

-- name: DeleteClientApplication :one
DELETE FROM core_client_applications
WHERE id = $1 AND (tenant_id = sqlc.narg('tenant_id')::varchar OR tenant_id IS NULL)
RETURNING id;

-- name: UpdateClientApplicationLastUsed :exec
UPDATE core_client_applications
SET last_used_at = NOW()
WHERE id = $1;