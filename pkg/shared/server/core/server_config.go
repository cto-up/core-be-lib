package core

import (
	"context"
	"sync"

	"ctoup.com/coreapp/api/handlers"
	"ctoup.com/coreapp/api/helpers"
	core "ctoup.com/coreapp/api/openapi/core"
	coreapi "ctoup.com/coreapp/pkg/core/api"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/rs/zerolog/log"

	// [DO NOT REMOVE COMMENT - Import]
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/auth"
	_ "ctoup.com/coreapp/pkg/shared/auth/kratos"
	"ctoup.com/coreapp/pkg/shared/observability"
	"ctoup.com/coreapp/pkg/shared/service"

	"ctoup.com/coreapp/pkg/shared/seedservice"

	"github.com/go-playground/validator/v10"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	_ "github.com/jackc/pgx/v5/stdlib"
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

	// authSlot is the indirection through which the auth middleware actually
	// runs. APIOptions.Middlewares holds the bound method value authSlot.handle,
	// so gin captures a stable reference at route-registration time; later
	// calls to WrapAuthMiddleware mutate authSlot.inner and are seen by every
	// registered route (core + module). See WrapAuthMiddleware.
	authSlot *authMiddlewareSlot
}

// authMiddlewareSlot is a mutable container for the coreapp auth middleware.
// It lets callers layer additional auth behavior AFTER the middleware slice
// has been installed into the router, without relying on slice position or
// function-pointer identity.
type authMiddlewareSlot struct {
	inner gin.HandlerFunc
}

func (s *authMiddlewareSlot) handle(c *gin.Context) {
	s.inner(c)
}

var (
	serverConfigInstance *ServerConfig
	serverConfigOnce     sync.Once
)

// outerMiddlewares are consumer-supplied handlers placed at the very head of
// the API middleware chain, ahead of metrics, request-id, tenant and auth.
var outerMiddlewares []gin.HandlerFunc

// RegisterOuterMiddleware injects cross-cutting middleware at the OUTERMOST
// position of the API chain (roadmap 020, Tier 2).
//
// This exists so a consumer can install distributed tracing without core taking
// a dependency on any tracing vendor's SDK — core stays vendor-neutral, and an
// app that wants none pays nothing.
//
// Outermost is the correct position for a tracer specifically: the span must
// enclose auth (a network round-trip to Kratos), tenant resolution and the
// handler, or the trace shows a gap it cannot explain. It also means the tracer
// sees panics before Recovery converts them into a 500.
//
// MUST be called before NewServerConfig. NewServerConfig is a sync.Once
// singleton, so a later call is silently ignored — hence the panic, which turns
// a wiring mistake into a startup failure rather than permanently missing
// telemetry that nobody notices for a month.
func RegisterOuterMiddleware(mw ...gin.HandlerFunc) {
	if serverConfigInstance != nil {
		panic("RegisterOuterMiddleware: must be called before NewServerConfig — the middleware chain is already built")
	}
	outerMiddlewares = append(outerMiddlewares, mw...)
}

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
	// gin.New() rather than gin.Default() (roadmap 020, Tier 1.2).
	//
	// gin.Default() is Logger() + Recovery(). Its Logger computes a latency for
	// every request and writes it to gin.DefaultWriter — os.Stdout — which in a
	// container goes to the runtime's log driver, which nothing scrapes. The
	// measurement was being taken correctly on every request and discarded.
	//
	// RequestIDMiddleware already emits a structured "Request handled" line
	// (method, route, status, duration_ms, request_id, tenant_id, user_id) to
	// the zerolog logger the app ships to Loki. Keeping gin's Logger as well
	// would mean two access logs, one of which is unreachable. Recovery is kept
	// verbatim — dropping it would turn a handler panic into a dead connection.
	router := gin.New()

	// RED metrics go on GIN'S OWN CHAIN, deliberately — not into the
	// APIOptions.Middlewares list below with the other cross-cutting concerns.
	//
	// oapi-codegen invokes that list as plain sequential function calls, so a
	// middleware timing via c.Next() there returns BEFORE the handler runs and
	// records ~76µs for every generated route. See the long note on
	// HTTPMetricsMiddleware. Moving this line back into `middlewares` silently
	// reverts the app-latency layer to measuring nothing, with every panel still
	// rendering plausible numbers.
	//
	// Placed OUTSIDE gin.Recovery(), which is subtle and load-bearing. Recovery
	// converts a handler panic into a 500; if metrics sat outside-in the other
	// order, its deferred observation would run while the panic was still
	// unwinding — before Recovery had set the status — and every panicking
	// request would be filed as 2xx. Outermost, it reads the 500 Recovery
	// produced. Also before cors, so preflight cost counts as the server-side
	// time it really is.
	router.Use(observability.HTTPMetricsMiddleware())
	router.Use(gin.Recovery())
	router.Use(cors)

	// Prometheus scrape endpoint (roadmap 020, Tier 1.1). Mounted on the app's
	// own listener, which is published to 127.0.0.1 only and is not proxied by
	// nginx, so it is unreachable from the internet; Prometheus reaches it over
	// the internal network. Registered before the API handlers so it is never
	// shadowed by a catch-all route.
	observability.MountMetricsEndpoint(router)

	err := connPool.Ping(context.Background())
	if err != nil {
		log.Err(err).Msg("Ping DB failed")
	}

	// Connection-pool saturation (roadmap 020, Tier 1.3). Watch
	// pgxpool_empty_acquire_count_total: when it rises, every individual query
	// is still fast and requests are nonetheless slow, because they are waiting
	// for a CONNECTION. Nothing else in the stack surfaces that wait — not the
	// nginx log, not the query timings, not the request histogram.
	observability.RegisterPgxPoolCollector(connPool)

	// Convert pgxpool.Pool to *sql.DB for SqlCheck
	db := stdlib.OpenDBFromPool(connPool)

	// Setup health checks
	sqlCheck := checks.SqlCheck{Sql: db}
	checks := append([]checks.Check{sqlCheck}, additionalChecks...)
	setupHealthCheck(router, checks...)

	multiTenantService := service.NewMultitenantService(coreStore)

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

	// Auth dispatches through authSlot so WrapAuthMiddleware can layer
	// behavior on top without depending on slice position.
	//
	// 0. RED metrics (roadmap 020 Tier 1.1) — FIRST, so the histogram covers
	//    the entire server-side cost of the request. Anything placed above it
	//    is time the metric cannot see and nobody can later explain.
	// 1. Request ID middleware (also emits the structured access log)
	// 2. Tenant middleware (extract tenant ID)
	// 3. Auth middleware (verify token, via authSlot), separately timed
	// 4. Logger enrichment (stamp tenant_id/user_id onto the request logger)
	//
	// The auth step is BRACKETED by a timer pair because it makes a NETWORK
	// round-trip to Kratos on every authenticated request. Roadmap 018 dropped
	// the backend session cache (T5) pending "a measurement — Kratos's share of
	// p50 on ordinary endpoints"; http_server_auth_duration_seconds over
	// http_server_request_duration_seconds is that measurement.
	authSlot := &authMiddlewareSlot{inner: authMiddleware.MiddlewareFunc()}

	// Consumer-supplied outer middleware (tracing) goes ahead of everything, so
	// its span encloses auth and the handler. Empty unless a consumer called
	// RegisterOuterMiddleware.
	for _, mw := range outerMiddlewares {
		middlewares = append(middlewares, core.MiddlewareFunc(mw))
	}

	middlewares = append(middlewares,
		// NOTE: HTTPMetricsMiddleware is NOT here — it is on router.Use() above.
		core.MiddlewareFunc(service.RequestIDMiddleware()),
		core.MiddlewareFunc(tenantMiddleware.MiddlewareFunc()),
		// AuthTimerStart / AuthTimerEnd MUST bracket auth adjacently — anything
		// between them is counted as auth time. A single wrapping middleware
		// cannot work here: the auth middleware calls c.Next() itself, so a
		// wrapper measures the whole downstream chain rather than auth.
		core.MiddlewareFunc(observability.AuthTimerStart()),
		core.MiddlewareFunc(authSlot.handle),
		core.MiddlewareFunc(observability.AuthTimerEnd()),
		core.MiddlewareFunc(service.LoggerEnrichmentMiddleware()),
	)

	apiOptions := core.GinServerOptions{
		BaseURL:     "",
		Middlewares: middlewares,
	}

	// Seed
	seedService := seedservice.NewSeedService(coreStore, authProvider)
	seedService.Seed()

	handlers := handlers.CreateCoreHandlers(connPool, authProvider, multiTenantService, clientAppService)

	core.RegisterHandlersWithOptions(router, handlers, apiOptions)

	// Self-service membership (W1.1): invite / accept / decline, plus workspace
	// member administration. Registered here rather than generated from the spec
	// because these paths were never in it — the handler existed fully stubbed
	// and mounted nowhere. Same middleware chain as every other core route, so
	// they inherit auth and tenant resolution unchanged.
	membershipMW := make([]gin.HandlerFunc, 0, len(apiOptions.Middlewares))
	for _, mw := range apiOptions.Middlewares {
		membershipMW = append(membershipMW, gin.HandlerFunc(mw))
	}
	coreapi.RegisterTenantMembershipRoutes(
		router,
		coreapi.NewTenantMembershipHandler(coreStore, authProvider),
		membershipMW...,
	)

	return &ServerConfig{
		Router:           router,
		AuthProvider:     authProvider,
		TenantMiddleware: tenantMiddleware.MiddlewareFunc(),
		AuthMiddleware:   authMiddleware,
		APIOptions:       apiOptions,
		authSlot:         authSlot,
	}
}
