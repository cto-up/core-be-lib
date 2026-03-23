# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Before you start any task

1. Use the domain map below to identify which module docs to read
2. Glob-triggered rules in `.claude/rules/` load automatically when you touch matching files
3. Architecture Decision Records are in `docs/adr`folder
4. If touching database, OpenAPI, or a frontend — those rules files enforce mandatory workflows
5. The application is using ctoup.com/coreapp for user, tenant management. If improvement is required for ctoup.com/coreapp , source code is found in folder: ../core-be-lib
6. Never modify vendor folder directly

## Build & Development Commands

```bash
# Infrastructure
make postgresup          # Start PostgreSQL + pgvector (Docker)
make postgresdown        # Stop PostgreSQL

# Code generation (run after changing specs/queries)
make openapi             # Generate Go server + TypeScript client from OpenAPI specs
make sqlc                # Generate type-safe Go code from SQL queries

# Build
go build -o app ./cmd/full         # Build REST server
go build -o prompt ./cmd/prompt    # Build LLM prompt CLI

# Tests
go test ./...                              # Run all tests
go test -v ./pkg/core/service/...          # Run tests for a specific package
go test -v -run TestFunctionName ./...     # Run a single test

# Release
make release VERSION=v1.0.0 NOTES="Description"
```

**Toolchain prerequisites:** `brew install sqlc`, `npm install openapi-typescript-codegen -g`, `brew install yq@3`

**Log directory required:** `sudo mkdir -p /app/log && sudo chown $(whoami) /app/log`

**Tests use Testcontainers** — Docker must be running to execute integration tests (they spin up a pgvector container automatically).

## Architecture

### Layered Structure (contract-first)

1. **OpenAPI spec** (`pkg/core/api/openapi/`) — define endpoints here first
2. **Generated code** (`api/openapi/core/`) — `oapi-codegen` output, DO NOT EDIT
3. **Handlers** (`pkg/core/api/*_handler.go`) — parse request, call service, return response. Role checks happen here.
4. **Services** (`pkg/core/service/`) — business logic, no HTTP context
5. **Repository** (`pkg/core/db/repository/`) — SQLC-generated, DO NOT EDIT
6. **SQL queries** (`pkg/core/db/query/`) — SQLC source files
7. **Migrations** (`pkg/core/db/migration/`) — Goose format with `-- +goose Up` / `-- +goose Down`

### Key Packages

- `pkg/shared/auth/` — Ory Kratos auth provider, role helpers (`auth.IsAdmin(c)`, `auth.IsSuperAdmin(c)`, `auth.IsCustomerAdmin(c)`)
- `pkg/shared/service/` — middleware (auth, tenant, request ID), multi-tenant service, user service
- `pkg/shared/server/core/` — `ServerConfig` singleton for Gin router setup
- `pkg/shared/llmmodels/` — LLM model configs (OpenAI, Anthropic, Gemini, Ollama)
- `pkg/shared/fileservice/` — pluggable file storage (local, GCS, S3, Azure)
- `pkg/core/db/testutils/` — Testcontainers helpers, mock authenticator, random data generators
- `api/handlers/` — handler factory wiring all dependencies

### Middleware Pipeline (order matters)

RequestIDMiddleware → TenantMiddleware (subdomain-based) → AuthMiddleware (JWT + roles)

### Context Keys (from `pkg/shared/auth`)

`auth.AUTH_TENANT_ID_KEY`, `auth.AUTH_USER_ID`, `auth.AUTH_CLAIMS`, `auth.AUTH_TENANT_MEMBERSHIPS`, `auth.AUTH_IS_RESELLER`, `auth.AUTH_IS_ACTING_RESELLER`

### Role Hierarchy

`SUPER_ADMIN` (global) > `CUSTOMER_ADMIN` (tenant) > `ADMIN` (tenant) > `USER` (tenant)

## Development Conventions

### Workflow for New Endpoints

1. Edit OpenAPI spec in `pkg/core/api/openapi/`
2. Run `make openapi`
3. Implement the generated interface in a handler file
4. Add service logic in `pkg/core/service/`

### Workflow for Database Changes

1. Create migration in `pkg/core/db/migration/` (format: `YYYYMMDDHHMMSS_description.sql`)
2. Add SQL queries for SQLC
3. Run `make sqlc`
4. Never write raw DB code — always use SQLC-generated functions

### Code Patterns

- Import order: stdlib → external → internal (alphabetical)
- Error wrapping: `fmt.Errorf("service.DoThing: %w", err)`
- `context.Context` as first param in public functions
- Role checks in handlers only (not services), return 403 for failures
- Tenant isolation: always filter by tenant ID from context
- DB driver: `pgx/v5` only, never `database/sql`

## Repository Layout Note

For full-stack code generation, `core-be-lib` and `core-fe-lib` should be sibling directories — `make openapi` writes TypeScript clients to `../core-fe-lib/lib/openapi/`.
