//go:build testutils
// +build testutils

/*
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
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/rs/zerolog/log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var DB_CONNECTION string

func SetupPostgresContainer() (*pgxpool.Pool, func(), error) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "pgvector/pgvector:pg17",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpassword",
			"POSTGRES_DB":       "testdb",
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

	port, err := postgresC.MappedPort(ctx, "5432")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	DB_CONNECTION = fmt.Sprintf("postgres://testuser:testpassword@%s:%s/testdb?sslmode=disable", host, port.Port())

	connPool, err := pgxpool.New(context.Background(), DB_CONNECTION)
	if err != nil {
		log.Fatal().Err(err).Msg("Cannot connect to database")
	}

	if err := waitForDatabase(connPool); err != nil {
		log.Fatal().Err(err).Msg("failed to wait for database")
	}

	cleanup := func() {
		connPool.Close()
		postgresC.Terminate(ctx)
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

	// Find the backend/src directory by walking up the directory tree
	srcDir := currentDir
	for {
		if strings.HasSuffix(srcDir, "backend/src") || srcDir == "/" {
			break
		}
		srcDir = filepath.Dir(srcDir)
	}

	if srcDir == "/" {
		return fmt.Errorf("could not find backend/src directory")
	}

	// Construct the absolute path to the migrations directory
	migrationsDir := filepath.Join(srcDir, "db", "migration")

	// Run goose migrations
	if err := goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("failed to run goose migration up: %w", err)
	}

	return nil
}
