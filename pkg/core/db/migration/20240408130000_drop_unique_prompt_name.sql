-- +goose Up
-- Drop the unique constraint on (tenant_id, name)
ALTER TABLE core_prompts
DROP CONSTRAINT IF EXISTS core_unique_prompt_name_per_tenant;

-- +goose Down
ALTER TABLE core_prompts
ADD CONSTRAINT core_unique_prompt_name_per_tenant UNIQUE (tenant_id, name);
