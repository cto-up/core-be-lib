-- +goose Up
ALTER TABLE core_tenants ADD COLUMN is_reseller BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE core_tenants ADD COLUMN reseller_id VARCHAR(64);
ALTER TABLE core_tenants ADD CONSTRAINT fk_core_tenants_reseller FOREIGN KEY (reseller_id) REFERENCES core_tenants(tenant_id);

-- +goose Down
ALTER TABLE core_tenants DROP COLUMN reseller_id;
ALTER TABLE core_tenants DROP COLUMN is_reseller;
