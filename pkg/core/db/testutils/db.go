//go:build testutils
// +build testutils

/*************************************************************
This needs to be added in VS code settings.json
{
    "go.testTags": "testutils",
    "go.buildTags": "testutils"
}
*/

package testutils

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	coreDB "ctoup.com/coreapp/pkg/core/db"
	connectionRepository "ctoup.com/coreapp/pkg/shared/repository"
	"github.com/pressly/goose/v3"
	"github.com/rs/zerolog/log"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// ModuleIdentifier is the file used to identify the module root
	ModuleIdentifier = "go.mod"
	// MigrationRelativePath is the path from module root to migrations
	MigrationRelativePath = "pkg/core/db/migration"
	// PostgresImage is the Docker image used for testing
	PostgresImage = "pgvector/pgvector:pg17"
	// PostgresPort is the default Postgres port
	PostgresPort = "5432"
	// TestDBUser is the username for test database
	TestDBUser = "testuser"
	// TestDBPassword is the password for test database
	TestDBPassword = "testpassword"
	// TestDBName is the name of the test database
	TestDBName = "testdb"
	// PostgresScheme is the database connection scheme
	PostgresScheme = "postgres"
)

var DB_CONNECTION string

func SetupPostgresContainer() (*pgxpool.Pool, func(), error) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        PostgresImage,
		ExposedPorts: []string{PostgresPort + "/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     TestDBUser,
			"POSTGRES_PASSWORD": TestDBPassword,
			"POSTGRES_DB":       TestDBName,
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithStartupTimeout(60 * time.Second),
	}

	postgresC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start container: %w", err)
	}

	host, err := postgresC.Host(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get host: %w", err)
	}

	mappedPort, err := postgresC.MappedPort(ctx, PostgresPort+"/tcp")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	// Ensure proper scheme in connection string
	DB_CONNECTION = fmt.Sprintf("%s://%s:%s@%s:%s/%s?sslmode=disable",
		PostgresScheme,
		TestDBUser,
		TestDBPassword,
		host,
		mappedPort.Port(),
		TestDBName,
	)

	// Verify connection string format
	log.Info().Msgf("Database connection string: %s", DB_CONNECTION)

	connPool, err := pgxpool.New(context.Background(), DB_CONNECTION)
	if err != nil {
		log.Fatal().Err(err).Msg("Cannot connect to database")
	}

	if err := waitForDatabase(connPool); err != nil {
		log.Fatal().Err(err).Msg("failed to wait for database")
	}

	cleanup := func() {
		connPool.Close()
		if err := postgresC.Terminate(ctx); err != nil {
			log.Error().Err(err).Msg("Failed to terminate container")
		}
	}

	return connPool, cleanup, nil
}

func waitForDatabase(connPool *pgxpool.Pool) error {
	for i := 0; i < 10; i++ { // retry 10 times
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var result int
		err := connPool.QueryRow(ctx, "SELECT 1").Scan(&result)
		if err != nil {
			log.Printf("Database check failed: %v\n", err)
		}

		if result == 1 {
			fmt.Println("Database is ready")
			return nil
		} else {
			fmt.Println("Unexpected result from database check")
		}
		time.Sleep(2 * time.Second) // wait before retrying
	}
	return fmt.Errorf("unable to connect to the database after retries")
}

func RunMigrations(db *sql.DB) error {
	// Get the current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %w", err)
	}

	// Find the module root directory by looking for ModuleIdentifier
	moduleDir := currentDir
	for {
		if _, err := os.Stat(filepath.Join(moduleDir, ModuleIdentifier)); err == nil {
			break
		}
		parentDir := filepath.Dir(moduleDir)
		if parentDir == moduleDir {
			return fmt.Errorf("could not find %s file", ModuleIdentifier)
		}
		moduleDir = parentDir
	}

	// Construct the absolute path to the migrations directory relative to the module root
	migrationsDir := filepath.Join(moduleDir, MigrationRelativePath)
	if _, err := os.Stat(migrationsDir); err != nil {
		return fmt.Errorf("migrations directory not found at %s: %w", migrationsDir, err)
	}
	// Create migrate instance
	// Run goose migrations
	if err := goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("failed to run goose migration up: %w", err)
	}

	return nil
}

// NewTestStore creates a new test store using a Postgres test container
func NewTestStore(t *testing.T) *coreDB.Store {
	connPool, cleanup, err := SetupPostgresContainer()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to set up test container")
	}

	// Register cleanup to be called when the test completes
	t.Cleanup(cleanup)

	db, err := connectionRepository.ConnectDB(DB_CONNECTION)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database for migrations")
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		log.Fatal().Err(err).Msg("Failed to run migrations")
	}

	// Create and return the store
	return coreDB.NewStore(connPool)
}
