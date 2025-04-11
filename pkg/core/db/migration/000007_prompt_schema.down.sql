
-- Drop trigger
DROP TRIGGER IF EXISTS update_prompts_modtime ON public.core_prompts;

-- Drop indexes
DROP INDEX IF EXISTS idx_prompts_tenant_id;

-- Drop table
DROP TABLE IF EXISTS public.core_prompts;
