package core

import (
	"context"
	"database/sql"
	"sync"

	"ctoup.com/coreapp/api/handlers"
	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	// [DO NOT REMOVE COMMENT - Import]
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/service"

	"ctoup.com/coreapp/pkg/shared/seedservice"

	"github.com/go-playground/validator/v10"
	_ "github.com/golang-migrate/migrate/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
	healthcheck "github.com/tavsec/gin-healthcheck"
	"github.com/tavsec/gin-healthcheck/checks"
	"github.com/tavsec/gin-healthcheck/config"

	_ "github.com/golang-migrate/migrate/source/file"
	// pgx/v5 with sqlc you get its implicit support for prepared statements. No additional sqlc configuration is required.
	"github.com/jackc/pgx/v5/pgxpool"
)

type ServerConfig struct {
	Router                 *gin.Engine
	TenantClientPool       *service.FirebaseTenantClientConnectionPool
	FirebaseAuthMiddleware *service.FirebaseAuthMiddleware
	TenantMiddleware       *service.TenantMiddleware
	AuthMiddleware         *service.AuthMiddleware
	APIOptions             core.GinServerOptions
}

var (
	serverConfigInstance *ServerConfig
	serverConfigOnce     sync.Once
)

func NewServerConfig(connPool *pgxpool.Pool, dbConnection string, cors gin.HandlerFunc, additionalChecks ...checks.Check) *ServerConfig {
	serverConfigOnce.Do(func() {
		serverConfigInstance = initializeServerConfig(connPool, dbConnection, cors, additionalChecks...)
	})
	return serverConfigInstance
}

func setupHealthCheck(router *gin.Engine, defaultChecks ...checks.Check) {
	// Always include basic health checks
	allChecks := make([]checks.Check, 0)

	// Add any provided checks
	allChecks = append(allChecks, defaultChecks...)

	// Initialize health check with configuration and all checks
	healthcheck.New(router, config.DefaultConfig(), allChecks)
}

func initializeServerConfig(connPool *pgxpool.Pool, dbConnection string, cors gin.HandlerFunc, additionalChecks ...checks.Check) *ServerConfig {
	coreStore := db.NewStore(connPool)

	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterCustomTypeFunc(helpers.CustomTypeUUID, uuid.UUID{})
		v.RegisterValidation("uuid", helpers.ValidateUUID)
	}
	router := gin.Default()
	router.Use(cors)

	// to be removed when https://github.com/jackc/pgx/pull/1718 can inclide sql
	db, err := sql.Open("postgres", dbConnection)
	if err != nil {
		log.Info().Msg("Open failed")
		log.Info().Msg(err.Error())
	}

	err = db.Ping()
	if err != nil {
		log.Info().Msg("Ping DB failed")
		log.Info().Msg(err.Error())
	}

	defer db.Close()

	// Setup health checks
	sqlCheck := checks.SqlCheck{Sql: db}
	checks := append([]checks.Check{sqlCheck}, additionalChecks...)
	setupHealthCheck(router, checks...)

	multiTenantService := service.NewMultitenantService(coreStore)

	firebaseTenantClientPool, err := service.NewFirebaseTenantClientConnectionPool(context.Background(), multiTenantService)
	if err != nil {
		log.Fatal().Err(err).Msg("Cannot create NewFirebaseTenantClientConnectionPool!")
	}
	firebaseAuthMiddleWare := service.NewFirebaseMiddleware(nil, firebaseTenantClientPool, multiTenantService)
	tenantMiddleWare := service.NewTenantMiddleware(nil, multiTenantService)
	clientAppService := service.NewClientApplicationService(coreStore)

	// Create the combined auth middleware
	authMiddleware := service.NewAuthMiddleware(
		firebaseAuthMiddleWare,
		clientAppService,
	)

	apiOptions := core.GinServerOptions{
		BaseURL: "",
		Middlewares: []core.MiddlewareFunc{
			core.MiddlewareFunc(service.RequestIDMiddleware()),
			// Use the combined middleware, allowing API tokens
			core.MiddlewareFunc(authMiddleware.MiddlewareFunc()),
			core.MiddlewareFunc(tenantMiddleWare.MiddlewareFunc()),
		},
	}

	// Seed
	seedService := seedservice.NewSeedService(coreStore, firebaseTenantClientPool)
	seedService.Seed()

	handlers := handlers.CreateCoreHandlers(connPool, firebaseTenantClientPool, multiTenantService, clientAppService)

	core.RegisterHandlersWithOptions(router, handlers, apiOptions)

	return &ServerConfig{
		Router:                 router,
		TenantClientPool:       firebaseTenantClientPool,
		FirebaseAuthMiddleware: firebaseAuthMiddleWare,
		TenantMiddleware:       tenantMiddleWare,
		AuthMiddleware:         authMiddleware,
		APIOptions:             apiOptions,
	}
}
