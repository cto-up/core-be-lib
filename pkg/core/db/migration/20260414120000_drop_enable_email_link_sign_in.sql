-- +goose Up
ALTER TABLE core_tenants DROP COLUMN IF EXISTS enable_email_link_sign_in;

-- +goose Down
ALTER TABLE core_tenants ADD COLUMN enable_email_link_sign_in boolean NOT NULL DEFAULT false;
