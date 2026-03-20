---
paths:
  - "pkg/**"
  - "cmd/**"
  - "api/**"
  - "internal/**"
---

# Backend Module Rules

## Package layout

```
pkg/core/
├── api/
│   ├── *_handler.go     # Gin HTTP handlers — call services only, no business logic
│   └── openapi/         # OpenAPI YAML specs + oapi-codegen configs
├── db/
│   ├── migration/       # Goose SQL migrations
│   ├── query/           # sqlc input query files
│   ├── repository/      # sqlc-generated repository (DO NOT EDIT)
│   ├── sqlc.yaml
│   ├── store.go
│   └── testutils/       # Testcontainers helpers, mock auth, random data
└── service/
    └── *.go             # Business logic

pkg/shared/
├── auth/                # Auth provider interface + role helpers
├── service/             # Middleware (auth, tenant, requestID), user/tenant services
├── server/core/         # ServerConfig singleton (Gin + middleware wiring)
├── llmmodels/           # LLM model configs (OpenAI, Anthropic, Gemini, Ollama)
├── fileservice/         # Pluggable file storage (local, GCS, S3, Azure)
├── emailservice/
├── pgvector/
└── seedservice/

api/
├── handlers/            # Handler factory (wires all dependencies)
├── helpers/             # Response helpers (ErrorResponse, etc.)
├── health/              # Health check handler
└── openapi/
    └── core/            # oapi-codegen output — DO NOT EDIT

internal/
└── server/
    └── http/restserver.go   # Gin router setup
```

## Layer responsibilities

- **Handler**: Parse request → call service → return response. Role checks here. No DB calls, no business logic.
- **Service**: Business logic. Calls repository. No HTTP context.
- **Repository**: DB access only. Uses sqlc-generated functions (DO NOT EDIT generated files).

## Go conventions

### Import order

```go
import (
    // 1. std lib
    "context"
    "fmt"

    // 2. external
    "github.com/gin-gonic/gin"

    // 3. internal (alphabetical)
    "ctoup.com/coreapp/api/helpers"
    auth "ctoup.com/coreapp/pkg/shared/auth"
)
```

### Error wrapping

```go
return fmt.Errorf("service.DoThing: %w", err)
```

### Context

- Always `context.Context` as first param in public functions
- Extract from Gin context in handlers:

```go
tenantID, exists := c.Get(auth.AUTH_TENANT_ID_KEY)
if !exists {
    c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(errors.New("tenant_id not found in context")))
    return
}

userID, exists := c.Get(auth.AUTH_USER_ID)
if !exists {
    c.JSON(http.StatusInternalServerError, helpers.ErrorResponse(errors.New("user_id not found in context")))
    return
}
```

## Auth & multi-tenancy

- Middleware pipeline: RequestIDMiddleware → TenantMiddleware → AuthMiddleware
- All wired in `internal/server/http/restserver.go` and `pkg/shared/server/core/`
- Tenant ID via `auth.AUTH_TENANT_ID_KEY` context key
- Never implement auth logic directly — use helpers from `ctoup.com/coreapp/pkg/shared/auth`

## Role checking

### Role hierarchy

| Role             | Scope  | Privilege |
| ---------------- | ------ | --------- |
| `SUPER_ADMIN`    | Global | Highest   |
| `CUSTOMER_ADMIN` | Tenant | High      |
| `ADMIN`          | Tenant | Medium    |
| `USER`           | Tenant | Standard  |

### In handlers — use `auth.Is*()` from `ctoup.com/coreapp/pkg/shared/auth`

```go
import auth "ctoup.com/coreapp/pkg/shared/auth"

// Single role check
if !auth.IsAdmin(c) {
    c.JSON(http.StatusForbidden, gin.H{"error": "admin required"})
    return
}

// Multiple roles (any of)
if !auth.IsAdmin(c) && !auth.IsSuperAdmin(c) && !auth.IsCustomerAdmin(c) {
    c.JSON(http.StatusForbidden, gin.H{"error": "insufficient role"})
    return
}

// Assign to a variable when reused
isAdminOrAbove := auth.IsCustomerAdmin(c) || auth.IsAdmin(c) || auth.IsSuperAdmin(c)
```

Available functions: `auth.IsAdmin(c)`, `auth.IsSuperAdmin(c)`, `auth.IsCustomerAdmin(c)`

### Rules

- Role checks belong in the **handler**, not the service layer
- Always return `http.StatusForbidden` (403) for role failures, not 401
- Never re-implement role logic — only use `auth.Is*()` functions

## Testing

- Files: `*_test.go` alongside source files
- Framework: `stretchr/testify`
- DB tests use Testcontainers (Docker must be running): helpers in `pkg/core/db/testutils/`
- Run all: `go test ./...`
- Run package: `go test -v ./pkg/core/service/...`
- Run single test: `go test -v -run TestFunctionName ./...`
