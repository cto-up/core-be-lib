-- +goose Up
-- User-Tenant Membership Table
CREATE TABLE core_user_tenant_memberships (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    user_id VARCHAR(128) NOT NULL,
    tenant_id VARCHAR(64) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'USER',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    invited_by VARCHAR(128),
    invited_at timestamptz,
    joined_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT user_tenant_memberships_pk PRIMARY KEY (id),
    CONSTRAINT user_tenant_memberships_unique UNIQUE (user_id, tenant_id),
    CONSTRAINT fk_tenant FOREIGN KEY (tenant_id) REFERENCES core_tenants(tenant_id) ON DELETE CASCADE
);

CREATE INDEX idx_user_tenant_memberships_user_id ON core_user_tenant_memberships(user_id);
CREATE INDEX idx_user_tenant_memberships_tenant_id ON core_user_tenant_memberships(tenant_id);
CREATE INDEX idx_user_tenant_memberships_status ON core_user_tenant_memberships(status);

CREATE TRIGGER update_user_tenant_memberships_modtime
BEFORE UPDATE ON core_user_tenant_memberships
FOR EACH ROW
EXECUTE FUNCTION update_modified_column();

-- Possible roles: USER, ADMIN, CUSTOMER_ADMIN
-- Possible statuses: pending, active, suspended, removed

-- +goose Down
DROP TABLE IF EXISTS core_user_tenant_memberships;
