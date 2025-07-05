## Project Overview

Core Application

- **backend**: A Go-based API server.

## Key Technologies

- **Backend**: Go
- **Gin Framework**: Gin
- **Database**: PostgreSQL

## Development Workflow & Common Commands

This project uses `make` as the primary task runner in the root directory

### Global Commands (from root directory)

### Backend (`/backend`)

- **Run tests**: `cd backend/src && go test ./...`
- **Run linter**: `cd backend/src && golangci-lint run` (Example, if you use it)
- **Build binary**: `cd backend/src && go build -o app ./cmd/full`

## Architectural Conventions

Use Modular Monolith architecture.
Each feature is in its own module with its own API, database, and UI prefix.

- All API changes must be first defined in the OpenAPI specification located at
  `/backend/src/pkg/core/api/openapi`.
- Once defined in the OpenAPI specification, the API code is generated using `make openapi`.
- All new database migrations must be added to the `/backend/src/pkg/core/db/migration` directory.
- All new database tables must be created using SQLC and the corresponding queries must be added to the `/backend/src/pkg/core/db/sqlc` directory.
- All new database types must be added to the `/backend/src/pkg/core/db/types.go` file./modules/{module}/backend/migrations` directory
- Once the migration and SQLC files are added, run `make sqlc` to generate the database code.
- Handlers are located in the `/backend/src/pkg/core/api` directory.
- Services are located in the `/backend/src/pkg/core/service` directory.
- Database operations are located in the `/backend/src/pkg/core/db` directory.
