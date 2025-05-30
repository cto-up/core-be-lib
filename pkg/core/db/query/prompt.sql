-- name: GetPromptByID :one
SELECT * FROM core_prompts
WHERE id = $1 AND tenant_id = sqlc.arg('tenant_id')::text LIMIT 1;


-- name: GetPromptByName :one
SELECT * FROM core_prompts
WHERE name = $1 AND tenant_id = $2
LIMIT 1;

-- name: ListPrompts :many
SELECT * FROM core_prompts
WHERE tenant_id = sqlc.arg('tenant_id')::text
  AND (UPPER(name) LIKE UPPER(sqlc.narg('like')) OR sqlc.narg('like') IS NULL)
  AND (
    sqlc.narg('tags')::varchar[] IS NULL 
    OR 
    (sqlc.narg('tags')::varchar[] && tags)
  )
ORDER BY
  CASE WHEN sqlc.arg('sortBy')::TEXT = 'name' AND sqlc.arg('order')::TEXT = 'asc' THEN name END ASC,
  CASE WHEN sqlc.arg('sortBy')::TEXT = 'name' AND sqlc.arg('order')::TEXT != 'asc' THEN name END DESC
  ,
  CASE WHEN sqlc.arg('sortBy')::TEXT = 'content' AND sqlc.arg('order')::TEXT = 'asc' THEN content END ASC,
  CASE WHEN sqlc.arg('sortBy')::TEXT = 'content' AND sqlc.arg('order')::TEXT != 'asc' THEN content END DESC
  ,
  CASE WHEN sqlc.arg('sortBy')::TEXT = 'tags' AND sqlc.arg('order')::TEXT = 'asc' THEN tags END ASC,
  CASE WHEN sqlc.arg('sortBy')::TEXT = 'tags' AND sqlc.arg('order')::TEXT != 'asc' THEN tags END DESC
  ,
  CASE WHEN sqlc.arg('sortBy')::TEXT = 'parameters' AND sqlc.arg('order')::TEXT = 'asc' THEN parameters END ASC,
  CASE WHEN sqlc.arg('sortBy')::TEXT = 'parameters' AND sqlc.arg('order')::TEXT != 'asc' THEN parameters END DESC
  
LIMIT $1
OFFSET $2;

-- name: CreatePrompt :one
INSERT INTO core_prompts (
  user_id, tenant_id, "name", "content", "tags", "parameters", sample_parameters, "format", "format_instructions"
) VALUES (
  $1, sqlc.arg('tenant_id')::text, 
  $2, 
  $3, 
  sqlc.narg('tags')::varchar[], 
  sqlc.narg('parameters')::varchar[],
  sqlc.narg('sample_parameters')::jsonb,
  sqlc.arg('format')::varchar,
  sqlc.narg('format_instructions')::text
)
RETURNING *;

-- name: UpdatePrompt :one
UPDATE core_prompts 
SET "name" = COALESCE(sqlc.narg('name'), name),
    "content" = COALESCE(sqlc.narg('content'), content),
    "tags" = COALESCE(sqlc.narg('tags')::varchar[], tags),
    "parameters" = COALESCE(sqlc.narg('parameters')::varchar[], parameters),
    "sample_parameters" = COALESCE(sqlc.narg('sample_parameters')::jsonb, sample_parameters),
    "format" = sqlc.arg('format')::varchar,
    "format_instructions" = COALESCE(sqlc.narg('format_instructions')::text, format_instructions)
WHERE id = $1 AND tenant_id = sqlc.arg('tenant_id')::text
RETURNING *;

-- name: DeletePrompt :one
DELETE FROM core_prompts
WHERE id = $1 and tenant_id = sqlc.arg('tenant_id')::text
RETURNING id
;
