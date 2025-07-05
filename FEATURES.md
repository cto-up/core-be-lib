# Technical Features of the Core Application

Based on the codebase, here are the key technical features:

## Backend Infrastructure

- Go-based application
- PostgreSQL with pgvector support for vector storage
- Database migrations using golang-migrate
- SQL code generation with SQLC

## API Development

- OpenAPI-driven API design
- Auto-generated API code (server and client)
- Gin web framework for HTTP routing
- Multiple API namespaces (public, admin, superadmin)

## Authentication & Authorization

- Firebase Authentication integration
- Multi-tenant architecture
- API token authentication for service-to-service communication
- Role-based access control (CUSTOMER_ADMIN, ADMIN, SUPER_ADMIN)

## LLM Integration

- Support for multiple LLM providers:
  OpenAI (GPT models)
  Google AI (Gemini)
  Anthropic (Claude)
  Ollama (local models)
  LangChain integration

## Tenant Management

- Multi-tenant architecture
- Tenant features management
- Tenant profile customization
- Subdomain-based tenant identification

## Development Tools

- Makefile-based workflow
- OpenAPI code generation
- SQLC for database code generation
- Docker containerization
- Structured logging with zerolog
- OpenTelemetry integration

## Testing

- Testcontainers for integration testing
- Unit testing support
