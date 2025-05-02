-- Translations
-- name: CreateTranslation :one
INSERT INTO core_translations (
    entity_type,
    entity_id,
    language,
    field,
    value,
    tenant_id
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetTranslation :one
SELECT * FROM core_translations
WHERE entity_type = $1 AND entity_id = $2 AND field = $3 AND language = $4 AND tenant_id = $5;

-- name: GetTranslationById :one
SELECT * FROM core_translations
WHERE id = $1 AND tenant_id = $2;

-- name: ListTranslations :many
SELECT * FROM core_translations
WHERE tenant_id = sqlc.arg('tenant_id')::text
  AND (UPPER(name) LIKE UPPER(sqlc.narg('like')) OR sqlc.narg('like') IS NULL)
ORDER BY
  CASE
            WHEN sqlc.arg('order')::text = 'asc' and sqlc.arg('sortBy')::text = 'name' THEN "name"
        END ASC,
  CASE
            WHEN (NOT sqlc.arg('order')::text = 'asc') and sqlc.arg('sortBy')::text = 'name' THEN "name"
        END DESC
LIMIT $1
OFFSET $2;

-- name: UpdateTranslationById :one
UPDATE core_translations
SET value = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $1 AND tenant_id = $3
RETURNING *;

-- name: DeleteTranslationById :exec
DELETE FROM core_translations
WHERE id = $1 AND tenant_id = $2;

-- name: GetTranslationsByEntityId :many
SELECT * FROM core_translations
WHERE entity_id = $1 AND tenant_id = $2;

-- name: GetTranslationsByEntityTypeAndId :many
SELECT * FROM core_translations
WHERE entity_type = $1 AND entity_id = $2 AND tenant_id = $3;

-- name: DeleteTranslationsByEntityId :exec
DELETE FROM core_translations
WHERE entity_id = $1 AND tenant_id = $2;
