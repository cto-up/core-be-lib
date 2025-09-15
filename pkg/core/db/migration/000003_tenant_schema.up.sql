
-- core_tenants definition
CREATE TABLE core_tenants (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
        tenant_id VARCHAR(64) NOT NULL UNIQUE,
        name VARCHAR(128) NOT NULL UNIQUE,
        subdomain VARCHAR(255) NOT NULL UNIQUE,
        enable_email_link_sign_in boolean NOT NULL,
        allow_password_sign_up boolean NOT NULL,
        user_id varchar(128) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT tenants_pk PRIMARY KEY (id)
);
CREATE INDEX idx_tenants_name ON core_tenants ("name");
CREATE INDEX idx_tenants_subdomain ON core_tenants ("subdomain");

CREATE TRIGGER update_tenants_modtime
BEFORE UPDATE ON core_tenants
FOR EACH ROW
EXECUTE FUNCTION update_modified_column();
