-- +goose Up
BEGIN;

ALTER TABLE core_tenants
    ADD COLUMN feature_licenses JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMIT;

-- +goose Down
BEGIN;

ALTER TABLE core_tenants DROP COLUMN feature_licenses;

COMMIT;
