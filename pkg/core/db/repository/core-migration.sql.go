// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.29.0
// source: core-migration.sql

package repository

import (
	"context"
)

const getCoreMigration = `-- name: GetCoreMigration :one
SELECT version, dirty FROM core_migrations LIMIT 1
`

func (q *Queries) GetCoreMigration(ctx context.Context) (CoreMigration, error) {
	row := q.db.QueryRow(ctx, getCoreMigration)
	var i CoreMigration
	err := row.Scan(&i.Version, &i.Dirty)
	return i, err
}

const updateCoreMigration = `-- name: UpdateCoreMigration :exec
UPDATE core_migrations
SET 
  version = $1,
  dirty = $2
`

type UpdateCoreMigrationParams struct {
	Version int64 `json:"version"`
	Dirty   bool  `json:"dirty"`
}

func (q *Queries) UpdateCoreMigration(ctx context.Context, arg UpdateCoreMigrationParams) error {
	_, err := q.db.Exec(ctx, updateCoreMigration, arg.Version, arg.Dirty)
	return err
}
