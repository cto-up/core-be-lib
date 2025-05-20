# Core Application

A Go-based application that integrates with multiple LLM providers (OpenAI, Google AI, Anthropic, Ollama) and includes database support with PostgreSQL/pgvector.

## Prerequisites

### Environment Setup

```bash
# Create log directory with proper permissions
sudo mkdir -p /app/log
sudo chown jcantonio /app/log
sudo chmod 750 /app/log
```

### Required Tools

#### Database Tools

- **PostgreSQL**: Database with vector support

  ```bash
  # Start PostgreSQL container
  make postgresup

  # Stop PostgreSQL container
  make postgresdown
  ```

- **SQLC**: SQL Compiler for Go

  ```bash
  # Installation
  brew install sqlc

  # Generate Go code from SQL
  make sqlc
  ```

  - [Installation Guide](https://docs.sqlc.dev/en/latest/overview/install.html)
  - [PostgreSQL Tutorial](https://docs.sqlc.dev/en/latest/tutorials/getting-started-postgresql.html)

#### API Development Tools

- **OpenAPI Frontend Generator**

  ```bash
  # Installation
  npm install openapi-typescript-codegen -g
  brew install yq@3

  # Generate API code
  make openapi
  ```

- **OpenAPI Generator (oapi-codegen)**
  If you work with core-fe-lib frontend library, you need to clone the repository.
  and place it in a sibling directory as the core application.

Like:

```
- core-be-lib
- core-fe-lib
```

```bash
# Generate server code
make openapi
```

This generates the API code in the `BASE_API_BE_DIR := api/openapi` directory.
This also generates the API axios client in the `BASE_API_FE_DIR := ../core-fe-lib/lib/openapi` directory.

### Database Migration

```bash
# Create new migration
make migrate-create name=migration_name

# Apply migrations
make migrateup    # Apply all pending migrations
make migrateup1   # Apply one migration

# Rollback migrations
make migratedown  # Rollback all migrations
make migratedown1 # Rollback one migration
```

### LLM Providers Support

The application supports multiple LLM providers:

- OpenAI (GPT-3.5, GPT-4, etc ...)
- Google AI (Gemini)
- Anthropic (Claude)
- Ollama (Local models)

Required environment variables for each provider:

```bash
OPENAI_API_KEY=your_key
GOOGLEAI_API_KEY=your_key
ANTHROPIC_API_KEY=your_key
OLLAMA_SERVER_URL=http://localhost:11434
```

## Development

### VSCode Settings

```json
{
  "editor.formatOnSave": true,
  "makefile.configureOnOpen": false,
  "go.testTags": "testutils",
  "go.buildTags": "testutils"
}
```

### Release Process

Create a new release:

```bash
make release VERSION=v1.0.0 NOTES="Release notes here"
```

## Project Structure

```
.
├── api/
│   └── openapi/        # Generated OpenAPI server code
├── pkg/
│   ├── core/           # Core application logic
│   │   ├── api/        # API handlers
│   │   ├── db/         # Database operations
│   │   └── service/    # Business logic
│   └── shared/         # Shared utilities
└── docker/             # Docker configurations
```

## Release Process

```bash
make release VERSION=v1.0.0 NOTES="Release notes here"
```

## License

MIT
