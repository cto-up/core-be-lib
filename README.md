# Core Application: Modular Monolith Framework

A production-ready **Go** framework designed for building scalable, multi-tenant modular monoliths. This repository provides a contract-first development workflow, integrating advanced identity management, RAG-ready vector storage, and a pluggable LLM architecture.

## Key Capabilities

- **Contract-First API**: Automated code generation for Go (server) and TypeScript (Axios client) via OpenAPI.
- **Dual-Provider Auth**: Seamlessly toggle between **Firebase** (to be **DEPRECATED**) and **Ory Kratos** (supporting MFA and native multi-tenancy).
- **Enterprise Multi-tenancy**: Subdomain-based isolation with built-in **Reseller support** (`IsActingReseller`) for hierarchical account management.
- **AI/LLM Native**: Unified interface for OpenAI, Anthropic, Gemini, and Ollama, paired with **pgvector** for RAG patterns.
- **Type-Safe Persistence**: SQLC-driven database layer for high-performance, compile-time safe PostgreSQL operations.

---

## Getting Started

### 1. Environment Setup

The application requires a specific logging structure and a running PostgreSQL instance with vector support.

```bash
# Setup log directory
sudo mkdir -p /app/log && sudo chown $(whoami) /app/log && sudo chmod 750 /app/log

# Launch Infrastructure
make postgresup

```

### 2. Toolchain Installation

Ensure your local environment has the following generators:

- **Database**: `brew install sqlc`
- **API**: `npm install openapi-typescript-codegen -g` and `brew install yq@3`

### 3. Repository Structure

For full-stack synchronization, the backend (`core-be-lib`) and frontend (`core-fe-lib`) should be sibling directories to allow the OpenAPI generator to populate both libraries simultaneously.

```text
.
├── api/                # OpenAPI specs & generated handlers
├── cmd/                # Application entry points
├── internal/           # Private server & test logic
├── pkg/
│   ├── core/           # Domain logic (DB, Services, API)
│   └── shared/         # Cross-cutting concerns (Auth, Email, File)
└── docker/             # Kratos & Postgres configurations

```

---

## Development Workflow

### API Evolution

Modify your OpenAPI definitions, then synchronize the entire stack:

```bash
make openapi

```

This updates the Go server code in `api/openapi` and the Axios client in `../core-fe-lib/lib/openapi`.

### Database Migrations

We use **Goose** for versioned migrations. After updating SQL files, generate type-safe Go code:

```bash
make sqlc

```

### Authentication Configuration

Switch providers via environment variables:

```bash
AUTH_PROVIDER=kratos

```

---

## Technical Specifications

| Feature           | Implementation                                        |
| ----------------- | ----------------------------------------------------- |
| **Web Framework** | Gin (Go)                                              |
| **Database**      | PostgreSQL + pgvector , Goose for migration           |
| **Observability** | OpenTelemetry + zerolog                               |
| **Auth Patterns** | RBAC (Admin, SuperAdmin, Customer), MFA, Social Login |
| **Testing**       | Testcontainers for isolated integration tests         |

## Release Management

Automated versioning is handled via the Makefile:

```bash
make release VERSION=v1.0.0 NOTES="Description of changes"

```

## License

Distributed under the **MIT License**.
