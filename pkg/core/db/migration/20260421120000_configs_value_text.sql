-- +goose Up
-- Some configs (e.g. long prompt templates) exceed 255 characters.
ALTER TABLE core_global_configs ALTER COLUMN value TYPE TEXT;
ALTER TABLE core_tenant_configs ALTER COLUMN value TYPE TEXT;

-- +goose Down
ALTER TABLE core_global_configs ALTER COLUMN value TYPE VARCHAR(255);
ALTER TABLE core_tenant_configs ALTER COLUMN value TYPE VARCHAR(255);
