-- Drop the unique constraint on (tenant_id, name)
ALTER TABLE core_prompts
DROP CONSTRAINT IF EXISTS core_unique_prompt_name_per_tenant;
