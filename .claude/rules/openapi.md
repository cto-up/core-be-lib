---
paths:
  - "pkg/core/api/openapi/**"
  - "api/openapi/**"
  - "**/*-api.yaml"
  - "**/*-schema.yaml"
---

# OpenAPI Rules

## MANDATORY: Spec first, code second

Never add a new API endpoint by writing Go handler code directly.

**Always follow this order:**

1. Edit `pkg/core/api/openapi/core-api.yaml` (operations/paths)
2. Edit `pkg/core/api/openapi/core-schema.yaml` if adding new types
3. Run `make openapi` to generate backend Go code + TypeScript Axios client
4. Implement the generated interface in a handler under `pkg/core/api/`

## File layout

```
pkg/core/api/openapi/
├── core-api.yaml               # Operations (paths, request/response)
├── core-schema.yaml            # Shared types/schemas
└── parts/
    ├── _oapi-schema-config.yaml    # oapi-codegen config for types
    └── _oapi-service-config.yaml   # oapi-codegen config for server interface
```

## Generation command

```bash
make openapi
```

## Generated output locations

| Target              | Location                              |
| ------------------- | ------------------------------------- |
| Backend Go types    | `api/openapi/core/core-schema.go`     |
| Backend Go interface| `api/openapi/core/core-service.go`    |
| TypeScript client   | `../core-fe-lib/lib/openapi/core/`    |

> `core-be-lib` and `core-fe-lib` must be sibling directories for frontend generation to work.

## Handler implementation

- Handlers in `pkg/core/api/` must implement the generated `StrictServerInterface`
- Register handlers via `core.RegisterHandlersWithOptions` in `api/handlers/handlers.go`
- Never define route paths manually — they come from the OpenAPI spec
