-- name: CreateAPIToken :one
INSERT INTO core_api_tokens (
  client_application_id, name, description, token_hash, token_prefix, 
  expires_at, created_by, scopes
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: GetAPITokenByID :one
SELECT t.*, c.tenant_id 
FROM core_api_tokens t
JOIN core_client_applications c ON t.client_application_id = c.id
WHERE t.id = $1 
  AND (c.tenant_id = sqlc.narg('tenant_id')::varchar OR c.tenant_id IS NULL)
  AND t.revoked = false
LIMIT 1;

-- name: GetAPITokenByHash :one
SELECT t.*, c.tenant_id 
FROM core_api_tokens t
JOIN core_client_applications c ON t.client_application_id = c.id
WHERE t.token_hash = $1 
  AND t.revoked = false
  AND t.expires_at > NOW()
LIMIT 1;

-- name: ListAPITokens :many
SELECT t.*, c.name as application_name 
FROM core_api_tokens t
JOIN core_client_applications c ON t.client_application_id = c.id
WHERE (c.tenant_id = sqlc.narg('tenant_id')::varchar OR c.tenant_id IS NULL)
  AND (t.client_application_id = sqlc.narg('client_application_id')::uuid OR sqlc.narg('client_application_id') IS NULL)
  AND (
    sqlc.narg('include_revoked')::boolean OR t.revoked = false
  )
  AND (
    sqlc.narg('include_expired')::boolean OR t.expires_at > NOW()
  )
  AND (
    UPPER(t.name) LIKE UPPER(sqlc.narg('like')) 
    OR UPPER(c.name) LIKE UPPER(sqlc.narg('like'))
    OR sqlc.narg('like') IS NULL
  )
ORDER BY
  CASE WHEN sqlc.arg('sort_by')::TEXT = 'name' AND sqlc.arg('order')::TEXT = 'asc' THEN t.name END ASC,
  CASE WHEN sqlc.arg('sort_by')::TEXT = 'name' AND sqlc.arg('order')::TEXT != 'asc' THEN t.name END DESC,
  CASE WHEN sqlc.arg('sort_by')::TEXT = 'created_at' AND sqlc.arg('order')::TEXT = 'asc' THEN t.created_at END ASC,
  CASE WHEN sqlc.arg('sort_by')::TEXT = 'created_at' AND sqlc.arg('order')::TEXT != 'asc' THEN t.created_at END DESC,
  CASE WHEN sqlc.arg('sort_by')::TEXT = 'expires_at' AND sqlc.arg('order')::TEXT = 'asc' THEN t.expires_at END ASC,
  CASE WHEN sqlc.arg('sort_by')::TEXT = 'expires_at' AND sqlc.arg('order')::TEXT != 'asc' THEN t.expires_at END DESC
LIMIT $1
OFFSET $2;

-- name: UpdateAPIToken :one
UPDATE core_api_tokens
SET 
  name = $2,
  description = $3,
  expires_at = $4,
  scopes = $5
WHERE id = $1
RETURNING *;

-- name: RevokeAPIToken :one
UPDATE core_api_tokens
SET 
  revoked = true,
  revoked_at = NOW(),
  revoked_reason = $2,
  revoked_by = $3
WHERE id = $1
RETURNING *;

-- name: DeleteAPIToken :one
DELETE FROM core_api_tokens
WHERE id = $1
RETURNING id;

-- name: UpdateAPITokenLastUsed :exec
UPDATE core_api_tokens
SET 
  last_used_at = NOW(),
  last_used_ip = sqlc.narg('ip_address')::varchar
WHERE id = $1;

-- name: CreateAPITokenAuditLog :one
INSERT INTO core_api_token_audit_logs (
  token_id, action, ip_address, user_agent, additional_data
) VALUES (
  $1, $2, $3, $4, $5
)
RETURNING *;

-- name: GetAPITokenAuditLogs :many
SELECT * FROM core_api_token_audit_logs
WHERE token_id = $1
ORDER BY timestamp DESC
LIMIT $2
OFFSET $3;