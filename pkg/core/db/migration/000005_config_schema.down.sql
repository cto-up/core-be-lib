
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