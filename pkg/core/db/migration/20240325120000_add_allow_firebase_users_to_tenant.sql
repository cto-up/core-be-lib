-- +goose Up
ALTER TABLE core_tenants
ADD COLUMN allow_sign_up BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE core_tenants
DROP COLUMN allow_sign_up;
