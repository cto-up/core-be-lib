package http

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"

	"ctoup.com/coreapp/pkg/shared/server/core"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog/log"

	// pgx/v5 with sqlc you get its implicit support for prepared statements. No additional sqlc configuration is required.
	"github.com/jackc/pgx/v5/pgxpool"
)

// parseAllowedDomain reads DOMAIN from env — a single root domain, e.g.
// "ctoup.com". The apex host and any subdomain of it are allowed for CORS.
// Leading "*." or "." is tolerated.
func parseAllowedDomain() string {
	d := strings.TrimSpace(strings.ToLower(os.Getenv("DOMAIN")))
	d = strings.TrimPrefix(d, "*")
	d = strings.TrimPrefix(d, ".")
	return d
}

func isOriginAllowed(origin, domain string) bool {
	if domain == "" {
		return false
	}
	u, err := url.Parse(strings.ToLower(origin))
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := u.Hostname()
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func RunRESTServer(ctx context.Context, connPool *pgxpool.Pool, address string, dbConnection string) {

	allowedDomain := parseAllowedDomain()
	if allowedDomain == "" {
		log.Warn().Msg("No DOMAIN configured; cross-origin browser requests will be blocked")
	}

	cors := func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" && isOriginAllowed(origin, allowedDomain) {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Add("Vary", "Origin")
		}
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Session-Token, X-CSRF-Token, Cookie, X-Requested-With, X-App-Source")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, DELETE, OPTIONS, PATCH")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}

	serverConfig := core.NewServerConfig(connPool, cors)

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
		log.Err(err).Msg("Server error")
	}
}
