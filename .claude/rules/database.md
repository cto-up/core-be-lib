---
paths:
  - "pkg/core/db/**"
  - "**/*.sql"
  - "**/sqlc.yaml"
  - "**/migration/**"
---

# Database Rules

## MANDATORY: Never write raw DB code by hand

1. **Schema changes** → create a new Goose migration in `pkg/core/db/migration/`
2. **SQL queries** → add to files under `pkg/core/db/query/`, then run `make sqlc`
3. **Use generated code** → import from `pkg/core/db/repository/` — never bypass sqlc-generated functions

## Migration file naming

```
pkg/core/db/migration/
YYYYMMDDHHMMSS_description.sql
```

Goose format — always use `-- +goose Up` / `-- +goose Down` markers.

## SQLC generation

```bash
make sqlc
```

Configured in `pkg/core/db/sqlc.yaml`. Generates into `pkg/core/db/repository/` — **DO NOT EDIT** generated files.

## Store pattern

```go
// pkg/core/db/store.go
type Store struct {
    *repository.Queries
    ConnPool *pgxpool.Pool
}
```

## DB access pattern

- Connection: `pgx/v5` with `pgxpool` — never use `database/sql`
- Tenant isolation: always filter by tenant ID from `auth.AUTH_TENANT_ID_KEY`
- New types: add to `pkg/core/db/types.go`

## Integration testing

Use Testcontainers helpers from `pkg/core/db/testutils/`:

```go
func TestSomething(t *testing.T) {
    store := testutils.NewTestStore(t)  // spins up pgvector container + runs migrations
    // test code...
}
```

Docker must be running. Container is cleaned up automatically via `t.Cleanup()`.

## pgvector (RAG)

- Vector columns use `pgvector` extension (included in the `docker/docker-compose-postgresql.yml` image)
- Use `pgvector-go` types for embedding fields
- Vector support managed in `pkg/shared/pgvector/`
