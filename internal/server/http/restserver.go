package http

import (
	"context"
	"net/http"

	"ctoup.com/coreapp/pkg/shared/server/core"

	"github.com/gin-gonic/gin"
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
	cors := func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, DELETE, OPTIONS, PATCH")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}

	serverConfig := core.NewServerConfig(connPool, dbConnection, cors)

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
