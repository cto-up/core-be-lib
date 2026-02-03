package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"os"
)

func GetConnectionString() string {

	username, password, databaseUrl := getConnectionInfo()

	return fmt.Sprintf("postgres://%s:%s@%s", username, password, databaseUrl)
}

func getConnectionInfo() (string, string, string) {
	username := os.Getenv("DATABASE_USERNAME")
	if username == "" {
		log.Fatal().Msg("DATABASE_USERNAME required")
	}
	password := os.Getenv("DATABASE_PASSWORD")
	if password == "" {
		log.Fatal().Msg("DATABASE_PASSWORD required")
	}
	databaseUrl := os.Getenv("DATABASE_URL")
	if databaseUrl == "" {
		log.Fatal().Msg("DATABASE_URL required")
	}
	return username, password, databaseUrl
}

func ConnectDB(connectionString string) (*sql.DB, error) {

	db, err := sql.Open("pgx", connectionString)
	if err != nil {
		return nil, fmt.Errorf("Cannot open database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("Cannot ping database: %w", err)
	}

	return db, nil
}

type IConnector interface {
	Connect() (*pgxpool.Pool, error)
}

type ErrorConnector struct {
}

// Connect always throws error
func (r ErrorConnector) Connect() (*pgxpool.Pool, error) {
	return nil, fmt.Errorf("connect error")
}

type PostgresConnector struct {
	connectionString string
}

func NewPostgresConnector(connectionString string) PostgresConnector {
	return PostgresConnector{connectionString: connectionString}
}

// Connect always throws error
func (r PostgresConnector) Connect() (*pgxpool.Pool, error) {
	connPool, err := pgxpool.New(context.Background(), r.connectionString)
	if err != nil {
		log.Printf("connect error %v \n", err)
	}
	return connPool, err
}

type ConnectorRetryDecorator struct {
	Connector     IConnector
	Attempts      int
	Delay         time.Duration
	IncreaseDelay time.Duration
	MaxDelay      time.Duration
}

func (r ConnectorRetryDecorator) ConnectWithRetry(ctx context.Context) (*pgxpool.Pool, error) {
	for i := 0; i < r.Attempts; i++ {
		connPool, err := r.Connector.Connect()
		if err == nil {
			log.Info().Msg("Connected to DB")
			return connPool, err
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("retry canceled: %w", ctx.Err())
		case <-time.After(r.Delay):
			if r.Delay <= r.MaxDelay {
				r.Delay += r.IncreaseDelay
			}
			log.Printf("Next Attempt in %v minute(s) \n", r.Delay.Minutes())
		}
	}
	return nil, fmt.Errorf("connect error")
}

// MigrationDirection represents the direction of database migration
type MigrationDirection string

const (
	// MigrationDirectionUp represents upward migration
	MigrationDirectionUp MigrationDirection = "up"
	// MigrationDirectionDown represents downward migration
	MigrationDirectionDown MigrationDirection = "down"
)
