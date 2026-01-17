# Kratos Multi-Tenancy Server Setup Example

This document provides a complete example of setting up a Go server with Kratos multi-tenancy support.

## Complete Server Setup

### 1. Main Server File

```go
// cmd/full/main.go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ctoup.com/coreapp/internal/server/http"
	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/shared/auth"
	"ctoup.com/coreapp/pkg/shared/repository"
	"ctoup.com/coreapp/pkg/shared/service"
	sqlservice "ctoup.com/coreapp/pkg/shared/sql"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

func main() {
	// Load environment
	godotenv.Load("./.env")
	godotenv.Overload("./.env", "./.env.local")

	// Setup logging
	setupLogging()

	log.Print("Application started...")

	// Setup database
	ctx := context.Background()
	connPool := setupDatabase(ctx)
	defer connPool.Close()

	// Initialize services
	store := db.NewStore(connPool)
	multitenantService := service.NewMultitenantService(store)

	// Initialize auth provider (Kratos or Firebase based on env)
	authProvider, err := auth.InitializeAuthProvider(ctx, multitenantService)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize auth provider")
	}
	log.Info().Str("provider", authProvider.GetProviderName()).Msg("Auth provider initialized")

	// Initialize services
	clientAppService := service.NewClientApplicationService(store)
	kratosTenantService := service.NewKratosTenantService(store, authProvider)

	// Setup server
	webPort := os.Getenv("BACKEND_PORT")
	if webPort == "" {
		log.Fatal().Msg("BACKEND_PORT environment variable required")
	}

	// Start HTTP server
	go runServer(ctx, connPool, webPort, authProvider, clientAppService, multitenantService, kratosTenantService)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")
}

func setupLogging() {
	logFolder := os.Getenv("LOG_FOLDER")
	if logFolder == "" {
		log.Fatal().Msg("LOG_FOLDER required")
	}

	instanceName := os.Getenv("INSTANCE_NAME")
	logFilePath := fmt.Sprintf("%s/%s.log", logFolder, instanceName)
	if instanceName == "" {
		logFilePath = fmt.Sprintf("%s/main.log", logFolder)
	}

	logFile := &lumberjack.Logger{
		Filename:   logFilePath,
		MaxSize:    10,
		MaxBackups: 5,
		MaxAge:     30,
		Compress:   true,
	}

	multiWriter := zerolog.MultiLevelWriter(logFile, os.Stdout)
	log.Logger = zerolog.New(multiWriter).With().Timestamp().Logger()
}

func setupDatabase(ctx context.Context) *pgxpool.Pool {
	connectionString := repository.GetConnectionString()

	connector := sqlservice.ConnectorRetryDecorator{
		Connector:     sqlservice.NewPostgresConnector(connectionString),
		Attempts:      1000,
		Delay:         5 * time.Second,
		IncreaseDelay: 20 * time.Millisecond,
		MaxDelay:      1 * time.Minute,
	}

	log.Info().Msg("Creating Connection Pool")
	connPool, err := connector.ConnectWithRetry(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("Cannot create Pool")
	}

	log.Info().Msg("Connection Pool created")

	err = connPool.Ping(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("Cannot ping database")
	}

	log.Info().Msg("Database connection verified")
	return connPool
}

func runServer(
	ctx context.Context,
	connPool *pgxpool.Pool,
	port string,
	authProvider auth.AuthProvider,
	clientAppService *service.ClientApplicationService,
	multitenantService *service.MultitenantService,
	kratosTenantService *service.KratosTenantService,
) {
	router := gin.Default()

	// Setup CORS
	router.Use(corsMiddleware())

	// Setup request ID middleware
	requestIDMiddleware := service.NewRequestIDMiddleware()
	router.Use(requestIDMiddleware.MiddlewareFunc())

	// Public routes (no authentication)
	setupPublicRoutes(router, kratosTenantService, authProvider)

	// Protected routes (authentication required)
	setupProtectedRoutes(router, authProvider, clientAppService, multitenantService)

	// Start server
	address := ":" + port
	log.Info().Str("address", address).Msg("Starting HTTP server")

	if err := router.Run(address); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
	}
}

func setupPublicRoutes(
	router *gin.Engine,
	kratosTenantService *service.KratosTenantService,
	authProvider auth.AuthProvider,
) {
	public := router.Group("/public")
	{
		// Health check
		public.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "ok"})
		})

		// Kratos webhooks
		webhookHandler := service.NewKratosWebhookHandler(kratosTenantService, authProvider)
		webhookHandler.RegisterWebhookRoutes(public)
	}
}

func setupProtectedRoutes(
	router *gin.Engine,
	authProvider auth.AuthProvider,
	clientAppService *service.ClientApplicationService,
	multitenantService *service.MultitenantService,
) {
	// Create middlewares
	authMiddleware := service.NewAuthMiddleware(authProvider, clientAppService)
	tenantMiddleware := service.NewKratosTenantMiddleware(multitenantService, authProvider)

	// Apply middlewares to protected routes
	api := router.Group("/api/v1")
	api.Use(authMiddleware.MiddlewareFunc())
	api.Use(tenantMiddleware.MiddlewareFunc())
	{
		// Example tenant-scoped routes
		api.GET("/resources", getResources)
		api.POST("/resources", createResource)
		api.GET("/resources/:id", getResource)
		api.PUT("/resources/:id", updateResource)
		api.DELETE("/resources/:id", deleteResource)

		// User management
		users := api.Group("/users")
		{
			users.GET("/me", getCurrentUser)
			users.PUT("/me", updateCurrentUser)
		}

		// Tenant management (admin only)
		tenants := api.Group("/tenants")
		{
			tenants.GET("/current", getCurrentTenant)
			tenants.GET("/users", getTenantUsers)
			tenants.POST("/invite", inviteUserToTenant)
		}
	}

	// Admin routes (admin role required)
	admin := router.Group("/admin-api")
	admin.Use(authMiddleware.MiddlewareFunc())
	{
		admin.GET("/tenants", listAllTenants)
		admin.POST("/tenants", createTenant)
		admin.PUT("/tenants/:id", updateTenant)
	}

	// Super admin routes (super admin role required)
	superAdmin := router.Group("/superadmin-api")
	superAdmin.Use(authMiddleware.MiddlewareFunc())
	{
		superAdmin.GET("/stats", getSystemStats)
		superAdmin.POST("/tenants/:id/disable", disableTenant)
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Session-Token, X-Tenant-ID")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// Example handler implementations

func getResources(c *gin.Context) {
	// Get tenant context
	tenantID, subdomain, ok := service.GetTenantContext(c)
	if !ok {
		c.JSON(400, gin.H{"error": "No tenant context"})
		return
	}

	// Get authenticated user
	user := service.GetAuthenticatedUser(c)

	log.Info().
		Str("tenant_id", tenantID).
		Str("subdomain", subdomain).
		Str("user_id", user.UserID).
		Msg("Fetching resources")

	// Query resources scoped by tenant
	// resources, err := store.GetResourcesByTenant(c, tenantID)

	c.JSON(200, gin.H{
		"tenant_id": tenantID,
		"subdomain": subdomain,
		"resources": []interface{}{},
	})
}

func createResource(c *gin.Context) {
	tenantID, _, ok := service.GetTenantContext(c)
	if !ok {
		c.JSON(400, gin.H{"error": "No tenant context"})
		return
	}

	var input struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.BindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Create resource with tenant_id
	// resource, err := store.CreateResource(c, db.CreateResourceParams{
	//     TenantID:    tenantID,
	//     Name:        input.Name,
	//     Description: input.Description,
	// })

	c.JSON(201, gin.H{
		"message":   "Resource created",
		"tenant_id": tenantID,
	})
}

func getResource(c *gin.Context) {
	tenantID, _, ok := service.GetTenantContext(c)
	if !ok {
		c.JSON(400, gin.H{"error": "No tenant context"})
		return
	}

	resourceID := c.Param("id")

	// Get resource and verify it belongs to tenant
	// resource, err := store.GetResource(c, resourceID)
	// if resource.TenantID != tenantID {
	//     c.JSON(403, gin.H{"error": "Access denied"})
	//     return
	// }

	c.JSON(200, gin.H{
		"id":        resourceID,
		"tenant_id": tenantID,
	})
}

func updateResource(c *gin.Context) {
	tenantID, _, ok := service.GetTenantContext(c)
	if !ok {
		c.JSON(400, gin.H{"error": "No tenant context"})
		return
	}

	resourceID := c.Param("id")

	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := c.BindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Verify resource belongs to tenant before updating
	// resource, err := store.GetResource(c, resourceID)
	// if resource.TenantID != tenantID {
	//     c.JSON(403, gin.H{"error": "Access denied"})
	//     return
	// }

	c.JSON(200, gin.H{
		"message": "Resource updated",
		"id":      resourceID,
	})
}

func deleteResource(c *gin.Context) {
	tenantID, _, ok := service.GetTenantContext(c)
	if !ok {
		c.JSON(400, gin.H{"error": "No tenant context"})
		return
	}

	resourceID := c.Param("id")

	// Verify resource belongs to tenant before deleting
	// resource, err := store.GetResource(c, resourceID)
	// if resource.TenantID != tenantID {
	//     c.JSON(403, gin.H{"error": "Access denied"})
	//     return
	// }

	c.JSON(200, gin.H{
		"message": "Resource deleted",
		"id":      resourceID,
	})
}

func getCurrentUser(c *gin.Context) {
	user := service.GetAuthenticatedUser(c)

	c.JSON(200, gin.H{
		"user_id":   user.UserID,
		"email":     user.Email,
		"tenant_id": user.TenantID,
		"subdomain": user.Subdomain,
		"roles":     user.CustomClaims,
	})
}

func updateCurrentUser(c *gin.Context) {
	user := service.GetAuthenticatedUser(c)

	var input struct {
		Name string `json:"name"`
	}

	if err := c.BindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Update user profile
	c.JSON(200, gin.H{
		"message": "User updated",
		"user_id": user.UserID,
	})
}

func getCurrentTenant(c *gin.Context) {
	tenantID, subdomain, ok := service.GetTenantContext(c)
	if !ok {
		c.JSON(400, gin.H{"error": "No tenant context"})
		return
	}

	// Get tenant details from database
	// tenant, err := store.GetTenant(c, tenantID)

	c.JSON(200, gin.H{
		"tenant_id": tenantID,
		"subdomain": subdomain,
	})
}

func getTenantUsers(c *gin.Context) {
	tenantID, subdomain, ok := service.GetTenantContext(c)
	if !ok {
		c.JSON(400, gin.H{"error": "No tenant context"})
		return
	}

	// Check if user has admin role
	user := service.GetAuthenticatedUser(c)
	if !contains(user.CustomClaims, "ADMIN") && !contains(user.CustomClaims, "SUPER_ADMIN") {
		c.JSON(403, gin.H{"error": "Admin access required"})
		return
	}

	// Get tenant users from Kratos
	// users, err := kratosTenantService.ListTenantUsers(c, subdomain)

	c.JSON(200, gin.H{
		"tenant_id": tenantID,
		"users":     []interface{}{},
	})
}

func inviteUserToTenant(c *gin.Context) {
	// Handled by webhook handler
	webhookHandler := service.NewKratosWebhookHandler(nil, nil)
	webhookHandler.HandleTenantInvitation(c)
}

func listAllTenants(c *gin.Context) {
	// Admin only - list all tenants
	user := service.GetAuthenticatedUser(c)
	if !contains(user.CustomClaims, "ADMIN") && !contains(user.CustomClaims, "SUPER_ADMIN") {
		c.JSON(403, gin.H{"error": "Admin access required"})
		return
	}

	// Get all tenants from database
	// tenants, err := store.ListTenants(c)

	c.JSON(200, gin.H{
		"tenants": []interface{}{},
	})
}

func createTenant(c *gin.Context) {
	user := service.GetAuthenticatedUser(c)
	if !contains(user.CustomClaims, "ADMIN") && !contains(user.CustomClaims, "SUPER_ADMIN") {
		c.JSON(403, gin.H{"error": "Admin access required"})
		return
	}

	var input struct {
		Name                    string `json:"name" binding:"required"`
		Subdomain               string `json:"subdomain" binding:"required"`
		EnableEmailLinkSignIn   bool   `json:"enable_email_link_sign_in"`
		AllowPasswordSignUp     bool   `json:"allow_password_sign_up"`
	}

	if err := c.BindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Create tenant in database
	// tenant, err := store.CreateTenant(c, db.CreateTenantParams{...})

	c.JSON(201, gin.H{
		"message": "Tenant created",
	})
}

func updateTenant(c *gin.Context) {
	user := service.GetAuthenticatedUser(c)
	if !contains(user.CustomClaims, "ADMIN") && !contains(user.CustomClaims, "SUPER_ADMIN") {
		c.JSON(403, gin.H{"error": "Admin access required"})
		return
	}

	tenantID := c.Param("id")

	var input struct {
		Name                  string `json:"name"`
		EnableEmailLinkSignIn *bool  `json:"enable_email_link_sign_in"`
		AllowPasswordSignUp   *bool  `json:"allow_password_sign_up"`
	}

	if err := c.BindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Update tenant
	c.JSON(200, gin.H{
		"message":   "Tenant updated",
		"tenant_id": tenantID,
	})
}

func getSystemStats(c *gin.Context) {
	user := service.GetAuthenticatedUser(c)
	if !contains(user.CustomClaims, "SUPER_ADMIN") {
		c.JSON(403, gin.H{"error": "Super admin access required"})
		return
	}

	// Get system statistics
	c.JSON(200, gin.H{
		"total_tenants": 0,
		"total_users":   0,
	})
}

func disableTenant(c *gin.Context) {
	user := service.GetAuthenticatedUser(c)
	if !contains(user.CustomClaims, "SUPER_ADMIN") {
		c.JSON(403, gin.H{"error": "Super admin access required"})
		return
	}

	tenantID := c.Param("id")

	// Disable tenant
	c.JSON(200, gin.H{
		"message":   "Tenant disabled",
		"tenant_id": tenantID,
	})
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
```

## Environment Variables

```env
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=coreapp
DB_SSLMODE=disable

# Server
BACKEND_PORT=8080
INSTANCE_NAME=main
LOG_FOLDER=/app/log

# Auth Provider
AUTH_PROVIDER=kratos  # or 'firebase'

# Kratos Configuration
KRATOS_ADMIN_URL=http://localhost:4434
KRATOS_PUBLIC_URL=http://localhost:4433

# Firebase Configuration (if using Firebase)
FIREBASE_CREDENTIALS_FILE=/path/to/credentials.json
```

## Kratos Configuration

```yaml
# kratos.yml
version: v0.13.0

dsn: postgres://postgres:postgres@localhost:5432/kratos?sslmode=disable

serve:
  public:
    base_url: http://localhost:4433/
    cors:
      enabled: true
      allowed_origins:
        - http://localhost:3000
        - http://localhost:5173
        - https://app.example.com
        - https://*.example.com
      allowed_methods:
        - GET
        - POST
        - PUT
        - DELETE
        - PATCH
      allowed_headers:
        - Authorization
        - Content-Type
        - X-Session-Token
        - X-Tenant-ID
      allow_credentials: true
  admin:
    base_url: http://localhost:4434/

selfservice:
  default_browser_return_url: http://localhost:3000/
  allowed_return_urls:
    - http://localhost:3000
    - http://localhost:5173
    - https://app.example.com
    - https://*.example.com

  methods:
    password:
      enabled: true
    link:
      enabled: true

  flows:
    error:
      ui_url: http://localhost:3000/error

    settings:
      ui_url: http://localhost:3000/settings
      privileged_session_max_age: 15m
      after:
        hooks:
          - hook: web_hook
            config:
              url: http://localhost:8080/public/webhooks/kratos/settings
              method: POST

    recovery:
      enabled: true
      ui_url: http://localhost:3000/recovery

    verification:
      enabled: true
      ui_url: http://localhost:3000/verification

    logout:
      after:
        default_browser_return_url: http://localhost:3000/auth/signin

    login:
      ui_url: http://localhost:3000/auth/signin
      lifespan: 10m
      after:
        hooks:
          - hook: web_hook
            config:
              url: http://localhost:8080/public/webhooks/kratos/login
              method: POST

    registration:
      lifespan: 10m
      ui_url: http://localhost:3000/auth/signup
      after:
        password:
          hooks:
            - hook: web_hook
              config:
                url: http://localhost:8080/public/webhooks/kratos/registration
                method: POST
                body: base64://ewogICJpZGVudGl0eSI6IHt9Cn0=

log:
  level: debug
  format: text
  leak_sensitive_values: true

secrets:
  cookie:
    - PLEASE-CHANGE-ME-I-AM-VERY-INSECURE
  cipher:
    - 32-LONG-SECRET-NOT-SECURE-AT-ALL

ciphers:
  algorithm: xchacha20-poly1305

hashers:
  algorithm: bcrypt
  bcrypt:
    cost: 8

identity:
  default_schema_id: default
  schemas:
    - id: default
      url: file:///etc/config/kratos/identity.schema.json

courier:
  smtp:
    connection_uri: smtps://test:test@mailslurper:1025/?skip_ssl_verify=true
```

## Identity Schema

```json
// identity.schema.json
{
  "$id": "https://schemas.ory.sh/presets/kratos/quickstart/email-password/identity.schema.json",
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Person",
  "type": "object",
  "properties": {
    "traits": {
      "type": "object",
      "properties": {
        "email": {
          "type": "string",
          "format": "email",
          "title": "E-Mail",
          "minLength": 3,
          "ory.sh/kratos": {
            "credentials": {
              "password": {
                "identifier": true
              }
            },
            "verification": {
              "via": "email"
            },
            "recovery": {
              "via": "email"
            }
          }
        },
        "name": {
          "type": "string",
          "title": "Name"
        },
        "subdomain": {
          "type": "string",
          "title": "Tenant Subdomain"
        }
      },
      "required": ["email"],
      "additionalProperties": false
    }
  }
}
```

This complete example shows how to set up a production-ready server with Kratos multi-tenancy support.
