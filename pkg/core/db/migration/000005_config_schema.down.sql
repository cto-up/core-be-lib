
-- Drop trigger
DROP TRIGGER IF EXISTS update_global_configs_modtime ON public.core_global_configs;

-- Drop indexes

-- Drop table
DROP TABLE IF EXISTS public.core_global_configs;


-- Drop trigger
DROP TRIGGER IF EXISTS update_tenant_configs_modtime ON public.core_tenant_configs;

-- Drop indexes

-- Drop table
DROP TABLE IF EXISTS public.core_tenant_configs;