
-- Drop trigger
DROP TRIGGER IF EXISTS update_tenants_modtime ON public.core_tenants;

-- Drop indexes
DROP INDEX IF EXISTS idx_tenants_name;
DROP INDEX IF EXISTS idx_tenants_subdomain;

-- Drop table
DROP TABLE IF EXISTS public.core_tenants;