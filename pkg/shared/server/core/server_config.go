package core

import (
	"context"
	"sync"

	"ctoup.com/coreapp/api/handlers"
	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	// [DO NOT REMOVE COMMENT - Import]
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/auth"
	_ "ctoup.com/coreapp/pkg/shared/auth/firebase"
	_ "ctoup.com/coreapp/pkg/shared/auth/kratos"
	"ctoup.com/coreapp/pkg/shared/service"

	"ctoup.com/coreapp/pkg/shared/seedservice"

	"github.com/go-playground/validator/v10"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rs/zerolog/log"
	healthcheck "github.com/tavsec/gin-healthcheck"
	"github.com/tavsec/gin-healthcheck/checks"
	"github.com/tavsec/gin-healthcheck/config"

	// pgx/v5 with sqlc you get its implicit support for prepared statements. No additional sqlc configuration is required.
	"github.com/jackc/pgx/v5/pgxpool"
)

type ServerConfig struct {
	Router           *gin.Engine
	AuthProvider     auth.AuthProvider
	TenantMiddleware gin.HandlerFunc
	AuthMiddleware   *service.AuthMiddleware
	APIOptions       core.GinServerOptions
}

var (
	serverConfigInstance *ServerConfig
	serverConfigOnce     sync.Once
)

func NewServerConfig(connPool *pgxpool.Pool, cors gin.HandlerFunc, additionalChecks ...checks.Check) *ServerConfig {
	serverConfigOnce.Do(func() {
		serverConfigInstance = initializeServerConfig(connPool, cors, additionalChecks...)
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

func initializeServerConfig(connPool *pgxpool.Pool, cors gin.HandlerFunc, additionalChecks ...checks.Check) *ServerConfig {
	coreStore := db.NewStore(connPool)

	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterCustomTypeFunc(helpers.CustomTypeUUID, uuid.UUID{})
		v.RegisterValidation("uuid", helpers.ValidateUUID)
	}
	router := gin.Default()
	router.Use(cors)

	err := connPool.Ping(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Ping DB failed")
	}

	// Convert pgxpool.Pool to *sql.DB for SqlCheck
	db := stdlib.OpenDBFromPool(connPool)

	// Setup health checks
	sqlCheck := checks.SqlCheck{Sql: db}
	checks := append([]checks.Check{sqlCheck}, additionalChecks...)
	setupHealthCheck(router, checks...)

	multiTenantService := service.NewMultitenantService(coreStore)

	// Initialize the auth provider based on environment (Firebase or Kratos)
	authProvider, err := auth.InitializeAuthProvider(context.Background(), multiTenantService)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize auth provider")
	}

	clientAppService := service.NewClientApplicationService(coreStore)

	// Create the combined auth middleware with the generic auth provider
	authMiddleware := service.NewAuthMiddleware(
		authProvider,
		clientAppService,
	)

	// Configure middleware order based on provider type
	var middlewares []core.MiddlewareFunc

	tenantMiddleware := service.NewTenantMiddleware(nil, multiTenantService)

	// 1. Request ID middleware
	// 2. Tenant middleware (extract tenant ID)
	// 3. Auth middleware (verify token)

	middlewares = []core.MiddlewareFunc{
		core.MiddlewareFunc(service.RequestIDMiddleware()),
		core.MiddlewareFunc(tenantMiddleware.MiddlewareFunc()),
		core.MiddlewareFunc(authMiddleware.MiddlewareFunc()),
	}

	apiOptions := core.GinServerOptions{
		BaseURL:     "",
		Middlewares: middlewares,
	}

	// Seed
	seedService := seedservice.NewSeedService(coreStore, authProvider)
	seedService.Seed()

	handlers := handlers.CreateCoreHandlers(connPool, authProvider, multiTenantService, clientAppService)

	core.RegisterHandlersWithOptions(router, handlers, apiOptions)

	return &ServerConfig{
		Router:           router,
		AuthProvider:     authProvider,
		TenantMiddleware: tenantMiddleware.MiddlewareFunc(),
		AuthMiddleware:   authMiddleware,
		APIOptions:       apiOptions,
	}
}
