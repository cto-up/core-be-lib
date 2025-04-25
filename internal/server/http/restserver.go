package http

import (
	"context"
	"net/http"

	"ctoup.com/coreapp/pkg/shared/server/core"

	_ "github.com/golang-migrate/migrate/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"

	_ "github.com/golang-migrate/migrate/source/file"
	// pgx/v5 with sqlc you get its implicit support for prepared statements. No additional sqlc configuration is required.
	"github.com/jackc/pgx/v5/pgxpool"
)

func RunRESTServer(ctx context.Context, connPool *pgxpool.Pool, address string, dbConnection string) {
	// MigrateUp Core
	serverConfig := core.NewServerConfig(connPool, dbConnection)

	// Create the connection pool
	pool, err := pgxpool.New(context.Background(), dbConnection)
	if err != nil {
		log.Fatal().Err(err).Msg("Unable to create connection pool")
	}
	defer pool.Close()

	// Admin routes group
	adminGroup := serverConfig.Router.Group("/admin")
	adminGroup.Use(serverConfig.AuthMiddleware.MiddlewareFunc()) // Don't allow API tokens

	// Run the server in a goroutine so we can listen for ctx cancellation
	serverErrorChan := make(chan error, 1)
	go func() {
		//router.RunTLS(":9000", "server.pem", "server.key")
		if err := serverConfig.Router.Run(address); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("WS Listen")
			serverErrorChan <- err
		}
	}()
	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		log.Info().Msg("Context canceled, shutting down services...")
	case err := <-serverErrorChan:
		log.Error().Err(err).Msg("Server error")
	}
}
