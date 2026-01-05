To set up multi-tenancy in Ory Kratos, we leverage **Organizations**. In this architecture, Kratos acts as the identity provider, and your Go Gin application acts as the "relying party" that manages tenants and users.

---

## Phase 1: Docker Compose Setup

Ory Kratos requires a SQL database (PostgreSQL) to store identities and organizational data.

### 1. Project Structure

```text
.
├── docker-compose.yml
├── kratos.yaml
└── main.go (Gin App)

```

### 2. `docker-compose.yml`

```yaml
services:
  kratos-db:
    image: postgres:16
    environment:
      - POSTGRES_USER=kratos
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=kratos
    ports: ["5432:5432"]

  kratos:
    image: oryd/kratos:latest
    ports:
      - "4433:4433" # Public API
      - "4434:4434" # Admin API
    command: serve -c /etc/config/kratos/kratos.yaml --watch-courier
    volumes:
      - ./kratos.yaml:/etc/config/kratos/kratos.yaml
    environment:
      - DSN=postgres://kratos:secret@kratos-db:5432/kratos?sslmode=disable
    depends_on:
      - kratos-db
```

To improve this README for a developer, we need to bridge the gap between "configuration" and "execution." A developer needs to know **how to verify** the setup, the **exact order of operations**, and how the **seeding** logic actually interacts with the code.

Here is the enhanced version of your README, organized for clarity and actionability.

---

# Multi-Tenant Identity Management with Ory Kratos & Go Gin

This guide sets up a native multi-tenant authentication system where users are logically isolated by `organization_id` and assigned roles via traits.

## Phase 1: Configuration

### 1. Kratos Schema (`identity.schema.json`)

### 2. `kratos.yaml`

Ensure `organizations` is a top-level key (not nested under `selfservice`).

```yaml
selfservice:
  default_browser_return_url: http://localhost:8080/
  allowed_return_urls:
    - http://localhost:8080/
  methods:
    password:
      enabled: true

# Top-level toggle for Native Multi-Tenancy
organizations:
  enabled: true

courier:
  enabled: true
```

---

## Phase 2: Go Gin Implementation

### 1. Dependencies

```bash
go get github.com/gin-gonic/gin
go get github.com/ory/kratos-client-go

```

### 2. Tenant & User CRUD (`main.go`)

This implementation uses the **Admin API** (port `4434`) to perform privileged operations.

```go
package main

import (
    "context"
    "github.com/gin-gonic/gin"
    ory "github.com/ory/kratos-client-go"
)

var (
    kratosAdmin = ory.NewAPIClient(ory.NewConfiguration())
    ctx = context.Background()
)

func main() {
    r := gin.Default()
    kratosAdmin.GetConfig().Servers = ory.ServerConfigurations{{URL: "http://localhost:4434"}}

    // Bootstrap: Create Super Admin on Startup
    r.POST("/system/seed", seedSuperAdmin)

    // Tenant CRUD
    r.POST("/tenants", createTenant)

    // User CRUD (Scoped to Tenant)
    r.POST("/tenants/:tid/users", createUser)

    r.Run(":8080")
}

func createTenant(c *gin.Context) {
    var req struct{ Name string `json:"name"` }
    if err := c.BindJSON(&req); err != nil return

    org, _, err := kratosAdmin.IdentityAPI.CreateOrganization(ctx).
        CreateOrganizationRequest(*ory.NewCreateOrganizationRequest(req.Name)).Execute()

    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(201, org)
}

func createUser(c *gin.Context) {
    tenantID := c.Param("tid")
    var req struct {
        Email string `json:"email"`
        Role  string `json:"role"`
    }
    c.BindJSON(&req)

    // Map traits (Claims) and Organization ID
    ident := *ory.NewCreateIdentityBody("default", map[string]interface{}{
        "email":        req.Email,
    })

    user, _, err := kratosAdmin.IdentityAPI.CreateIdentity(ctx).CreateIdentityBody(ident).Execute()
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(201, user)
}

```

---

## Phase 3: Seeding & Verification

### 1. Seeding the SUPER_ADMIN

To bootstrap your system, you can create a "Global" admin not tied to a specific tenant, or tied to a "System" tenant.

```go
func seedSuperAdmin(c *gin.Context) {
    ident := *ory.NewCreateIdentityBody("default", map[string]interface{}{
        "email":        "admin@system.com",
    })

    user, _, err := kratosAdmin.IdentityAPI.CreateIdentity(ctx).CreateIdentityBody(ident).Execute()
    if err != nil {
        c.JSON(500, gin.H{"message": "Seed failed", "error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"message": "Super Admin Seeded", "id": user.Id})
}

```

### 2. Verifying Claims in Middleware

When a user logs in, Kratos returns the `organization_id` and `traits`. Use this to enforce boundaries.

```go
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        cookie, _ := c.Cookie("ory_kratos_session")
        // Use Frontend API for session validation
        session, _, err := kratosPublic.FrontendAPI.ToSession(ctx).Cookie(cookie).Execute()

        if err != nil || !*session.Active {
            c.AbortWithStatus(401)
            return
        }

        // 2. Extract Role Claim
        traits := session.Identity.Traits.(map[string]interface{})

        c.Next()
    }
}

```

---

## Key Developer Commands

- **Start Infrastructure**: `docker-compose up -d`
- **Check Organizations**: `curl http://localhost:4434/admin/organizations`
- **Verify Identity**: `curl http://localhost:4434/admin/identities/<uuid>`
