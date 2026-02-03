-- +goose Up
ALTER TABLE core_tenants
    ADD COLUMN IF NOT EXISTS
    profile jsonb NOT NULL DEFAULT '{}';

ALTER TABLE core_tenants
    ADD COLUMN IF NOT EXISTS
    features jsonb NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE core_tenants DROP COLUMN IF EXISTS profile;

ALTER TABLE core_tenants DROP COLUMN IF EXISTS features;
