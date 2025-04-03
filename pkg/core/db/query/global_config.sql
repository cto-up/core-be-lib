-- name: GetGlobalConfigByID :one
SELECT * FROM core_global_configs
WHERE id = $1 LIMIT 1;

-- name: GetGlobalConfigByName :one
SELECT * FROM core_global_configs
WHERE name = $1 LIMIT 1;

-- name: ListGlobalConfigs :many
SELECT * FROM core_global_configs
WHERE (UPPER(name) LIKE UPPER(sqlc.narg('like')) OR sqlc.narg('like') IS NULL)
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

-- name: CreateGlobalConfig :one
INSERT INTO core_global_configs (
  user_id, "name", "value"
) VALUES (
  $1, $2, $3
)
RETURNING *;

-- name: UpdateGlobalConfig :one
UPDATE core_global_configs 
SET 
    "name" = $2,
    "value" = $3
WHERE id = $1
RETURNING id
;

-- name: DeleteGlobalConfig :one
DELETE FROM core_global_configs
WHERE id = $1
RETURNING id
;