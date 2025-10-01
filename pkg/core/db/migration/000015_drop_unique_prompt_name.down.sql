ALTER TABLE core_prompts
ADD CONSTRAINT core_unique_prompt_name_per_tenant UNIQUE (tenant_id, name);
