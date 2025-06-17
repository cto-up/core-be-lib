-- Remove the old core_roles column
ALTER TABLE public.core_users DROP COLUMN core_roles;

-- 1. Drop the existing UNIQUE constraint on "name"
ALTER TABLE public.core_tenant_configs
DROP CONSTRAINT IF EXISTS core_tenant_configs_name_key;

-- 2. Add a new UNIQUE constraint on (name, tenant_id)
ALTER TABLE public.core_tenant_configs
ADD CONSTRAINT core_tenant_configs_name_tenant_id_key UNIQUE (name, tenant_id);