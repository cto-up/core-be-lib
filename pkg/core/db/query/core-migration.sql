-- name: GetCoreMigration :one
SELECT version, dirty FROM core_migrations LIMIT 1;

-- name: UpdateCoreMigration :exec
UPDATE core_migrations
SET 
  version = $1,
  dirty = $2;