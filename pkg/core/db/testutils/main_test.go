package testutils

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ctoup.com/coreapp/pkg/core/db"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

var testStore *db.Store
var connPool *pgxpool.Pool

// Convention is the main entry point for all tests in package
func TestMain(m *testing.M) {
	var err error
	var cleanup func()
	connPool, cleanup, err = SetupPostgresContainer()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to set up test container")
	}
	testStore = db.NewStore(connPool)

	/* Run migrations
	basePath := filepath.Join("..", "migration")

	// Convert to URI format required by golang-migrate
	migrationURI := "file://" + filepath.ToSlash(basePath)
	if err := RunMigrations(migrationURI); err != nil {
		log.Fatal().Err(err).Msg("Failed to run migrations")
	}*/
	// Load and execute test.sql
	if err := loadTestSQL(connPool); err != nil {
		log.Fatal().Err(err).Msg("Failed to load test data")
		cleanup()
		os.Exit(1)
	}
	defer cleanup()

	// Run tests
	code := m.Run()
	os.Exit(code)

	os.Exit(m.Run())
}

func loadTestSQL(pool *pgxpool.Pool) error {
	ctx := context.Background()

	// Read test.sql file
	testSQLPath := filepath.Join("test.sql")
	testSQL, err := os.ReadFile(testSQLPath)
	if err != nil {
		return err
	}

	// Execute the test SQL
	_, err = pool.Exec(ctx, string(testSQL))
	if err != nil {
		return err
	}

	log.Info().Msg("Test SQL executed successfully")
	return nil
}
