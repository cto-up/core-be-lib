package db

import (
	"path/filepath"
	"runtime"
	"sync"

	"ctoup.com/coreapp/pkg/core/db/repository"
	sqlservice "ctoup.com/coreapp/pkg/shared/sql"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// getMigrationPath returns the absolute path to the migration directory
func getMigrationPath() string {
	_, filename, _, _ := runtime.Caller(0)
	// Get the directory containing this file (store.go)
	dir := filepath.Dir(filename)
	// Construct path to migration directory
	migrationPath := filepath.Join(dir, "migration")
	return "file://" + migrationPath
}

// Provides all function  to execute db queries and transactions
type Store struct {
	*repository.Queries
	ConnPool *pgxpool.Pool
}

func NewStore(connPool *pgxpool.Pool) *Store {
	migrate()
	return &Store{
		Queries:  repository.New(connPool),
		ConnPool: connPool,
	}
}

var once = sync.Once{}

func migrate() {
	once.Do(func() {
		path := getMigrationPath()
		prefix := "core"
		log.Info().Msg("Migrating... " + path + " " + prefix)
		sqlservice.MigrateUp(path, prefix)
	})
}
