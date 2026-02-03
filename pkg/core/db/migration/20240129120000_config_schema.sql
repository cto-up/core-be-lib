-- +goose Up

-- core_global_configs definition
CREATE TABLE core_global_configs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
        name VARCHAR(50) NOT NULL UNIQUE,
        value VARCHAR(255),
        user_id varchar(128) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT global_configs_pk PRIMARY KEY (id)
);

CREATE INDEX idx_global_configs_name ON core_global_configs("name");

CREATE TRIGGER update_global_configs_modtime
BEFORE UPDATE ON core_global_configs
FOR EACH ROW
EXECUTE FUNCTION update_modified_column();

-- core_tenant_configs definition
CREATE TABLE core_tenant_configs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
        name VARCHAR(50) NOT NULL UNIQUE,
        value VARCHAR(255),
        user_id varchar(128) NOT NULL,
    tenant_id varchar(64) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT tenant_configs_pk PRIMARY KEY (id)
);

CREATE INDEX idx_tenant_configs_name ON core_tenant_configs("name");
CREATE INDEX idx_tenant_configs_tenant_id ON core_tenant_configs("tenant_id");

CREATE TRIGGER update_tenant_configs_modtime
BEFORE UPDATE ON core_tenant_configs
FOR EACH ROW
EXECUTE FUNCTION update_modified_column();



-- +goose Down

-- Drop trigger
DROP TRIGGER IF EXISTS update_global_configs_modtime ON core_global_configs;

-- Drop indexes

-- Drop table
DROP TABLE IF EXISTS core_global_configs;


-- Drop trigger
DROP TRIGGER IF EXISTS update_tenant_configs_modtime ON core_tenant_configs;

-- Drop indexes

-- Drop table
DROP TABLE IF EXISTS core_tenant_configs;