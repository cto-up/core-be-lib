-- +goose Up
ALTER TABLE core_tenants
  ADD COLUMN contract_end_date TIMESTAMPTZ NULL,
  ADD COLUMN is_disabled       BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE core_tenants
  DROP COLUMN contract_end_date,
  DROP COLUMN is_disabled;
