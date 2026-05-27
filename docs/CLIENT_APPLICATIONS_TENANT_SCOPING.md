# Client Applications & API Tokens — Tenant Scoping

Client applications (`core_client_applications`) and their API tokens
(`core_api_tokens`) are either **tenant-specific** or **global**. The scope is
derived from a single string, the tenant ID, that flows from the request down
to the SQL filter.

## The rule

- **Tenant ID is set** (`"acme"`) → the row is **tenant-specific**. Only that
  tenant can see or manage it.
- **Tenant ID is empty** (`""`) → the row is **global**, managed by
  `SUPER_ADMIN`.

Scoping is **strict**: a tenant sees only its own rows, never globals, and a
global (empty-tenant) caller sees only globals, never any tenant's rows.

## Where the tenant ID comes from

Handlers in `pkg/core/api/client_application_handler.go` read it from the
request context — never hardcoded:

```go
tenantID := c.GetString(auth.AUTH_TENANT_ID_KEY)
```

`AUTH_TENANT_ID_KEY` is set by the tenant middleware
(`pkg/shared/service/tenant_middleware.go`) from the subdomain:

- Root / admin / `auth` subdomain → `""` → **global** scope.
- Tenant subdomain (`acme.example.com`) → the tenant's ID → **tenant** scope.

The auth middleware only overwrites this key when the authenticated user
actually has a tenant (`user.TenantID != ""`), so a `SUPER_ADMIN` at the root
domain keeps the empty (global) scope.

> Consequence: an `ADMIN`/`SUPER_ADMIN` acting on a tenant subdomain manages
> **that tenant's** applications, not the global ones.

## Storage

`core_client_applications.tenant_id` is `VARCHAR(64) NULL`:

- Global rows historically store `NULL`; an empty string `''` is treated
  identically (see the query below). No migration is required to mix the two.
- Tenant-specific rows store the tenant ID.

API tokens have **no** `tenant_id` column — they inherit their scope from the
parent client application through the foreign key, enforced via a `JOIN` in the
token queries.

## The SQL filter

Defined in `pkg/core/db/query/client_application.sql` and
`pkg/core/db/query/api_token.sql` (regenerate with `make sqlc` after editing):

```sql
WHERE (
    (sqlc.narg('tenant_id')::varchar IS NULL AND (tenant_id IS NULL OR tenant_id = ''))
    OR tenant_id = sqlc.narg('tenant_id')::varchar
)
```

- **No tenant param** (`narg` is `NULL`) → matches global rows only
  (`tenant_id IS NULL OR tenant_id = ''`).
- **Tenant param provided** → matches `tenant_id = $param` only.

The service layer (`pkg/shared/service/client_application_service.go`) maps the
empty string to a `NULL` parameter:

```go
var tenantIDParam *string
if tenantID != "" {
    tenantIDParam = &tenantID
}
// repository param: util.ToNullableText(tenantIDParam)
```

> Why not `tenant_id = $1` alone? Because global rows are `NULL`, and
> `NULL = NULL` (or `NULL = ''`) is never true in SQL — a plain `=` would make
> global/super-admin queries return zero rows. The filter above keeps `=` for
> the tenant case while still matching `NULL`/`''` globals when no tenant is
> given.
>
> Why not `OR tenant_id IS NULL` on every query? Because that leaks globals into
> a tenant's results and, worse, lets a tenant `UPDATE`/`DELETE`/`REVOKE` global
> rows — a privilege escalation. Strict matching prevents it.

## Mutations are scoped too

`CreateAPIToken` and `RevokeAPIToken` take a `tenantID` and scope their internal
client-application / token lookups, so a tenant cannot mint or revoke tokens on
an out-of-scope (global or other-tenant) application. `GetAPITokenByID` (used by
the GET / delete / revoke / audit handlers as an ownership gate) is likewise
tenant-scoped.

## Exception: token authentication

`GetAPITokenByHash` (the `APITokenMiddleware` path) is intentionally **not**
tenant-scoped. An incoming token is looked up by its hash globally, and the
tenant is then derived from the token's application and set on the context:

```go
if apiToken.TenantID.Valid {
    c.Set(auth.AUTH_TENANT_ID_KEY, apiToken.TenantID.String)
}
```
