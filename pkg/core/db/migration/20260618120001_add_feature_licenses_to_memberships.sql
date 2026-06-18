-- +goose Up
BEGIN;

-- Per-user feature licenses (seats) live on the membership, not the user: a user
-- can belong to several tenants, and a seat is scoped to one (user, tenant) pair.
-- Defaults to '{}' so every existing membership reads as "no per-user restriction"
-- (inherits the tenant entitlement) — engaging the user-level gate only once an
-- admin assigns a seat. This default is what prevents a deploy from locking out
-- existing users.
ALTER TABLE core_user_tenant_memberships
    ADD COLUMN feature_licenses JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMIT;

-- +goose Down
BEGIN;

ALTER TABLE core_user_tenant_memberships DROP COLUMN feature_licenses;

COMMIT;
