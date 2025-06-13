-- name: GetRoleByID :one
SELECT * FROM core_roles
WHERE id = $1 LIMIT 1;

-- name: GetRoleByName :one
SELECT * FROM core_roles
WHERE name = $1 LIMIT 1;

-- name: ListRoles :many
SELECT * FROM core_roles
WHERE (UPPER(name) LIKE UPPER(sqlc.narg('like')) OR sqlc.narg('like') IS NULL)
ORDER BY created_at
LIMIT $1
OFFSET $2;

-- name: CreateRole :one
INSERT INTO core_roles (
  "name", user_id
) VALUES (
  $1, $2
)
RETURNING *;

-- name: UpdateRole :one
UPDATE core_roles 
SET "name" = $2
WHERE id = $1
RETURNING id
;

-- name: DeleteRole :one
DELETE FROM core_roles
WHERE id = $1 AND user_id = $2
RETURNING id
;