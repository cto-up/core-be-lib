package sqlservice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang-migrate/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

type PagingSQL struct {
	Offset   int32  `form:"page" binding:"required,min=1"`
	PageSize int32  `form:"page_size" binding:"required,min=1,max=50"`
	SortBy   string `form:"sort_by"`
	Order    string `form:"order"`
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
	dbConnection string
}

func NewPostgresConnector(dbConnection string) PostgresConnector {
	return PostgresConnector{dbConnection: dbConnection}
}

// Connect always throws error
func (r PostgresConnector) Connect() (*pgxpool.Pool, error) {
	connPool, err := pgxpool.New(context.Background(), r.dbConnection)
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

func MigrateUp(dbConnection string, path string, prefix string) error {
	log.Info().Msg("Start Migrating up... " + path + " with prefix " + prefix)
	err := migrateMe(dbConnection, path, prefix, MigrationDirectionUp)
	if err != nil {
		log.Err(err).Msg("Cannot migrate up!")
		return err
	}
	log.Info().Msg("End Migrated up!")
	return nil
}
func MigrateDown(dbConnection string, path string, prefix string) error {
	log.Info().Msg("Start Migrating down... " + path + " with prefix " + prefix)
	err := migrateMe(dbConnection, path, prefix, MigrationDirectionDown)
	if err != nil {
		log.Err(err).Msg("Cannot migrate down!")
		return err
	}
	log.Info().Msg("End Migrating down!")
	return nil
}

func migrateMe(dbConnection string, path string, prefix string, direction MigrationDirection) error {
	m, err := migrate.New(
		path,
		dbConnection+"&x-migrations-table="+prefix+"_migrations",
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Cannot create migrate instance!")
	}

	if direction == MigrationDirectionUp {
		err = m.Up()
	} else if direction == MigrationDirectionDown {
		err = m.Down()
	}
	if err != nil {
		if strings.Contains(err.Error(), "no change") {
			log.Info().Msg("No migration change.")
		} else {
			log.Err(err).Msg("Error migrate!")
			return err
		}
	}
	return nil
}
