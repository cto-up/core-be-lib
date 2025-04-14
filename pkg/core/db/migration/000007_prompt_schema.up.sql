
-- public.core_prompts definition
CREATE TABLE public.core_prompts (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    content TEXT NOT NULL,
    tags VARCHAR[] NOT NULL DEFAULT '{}',
    parameters VARCHAR[] NOT NULL DEFAULT '{}',
    sample_parameters JSONB NOT NULL DEFAULT '{}',
    format VARCHAR(10) NOT NULL DEFAULT 'text',
    format_instructions TEXT,
    user_id varchar(128) NOT NULL,
    tenant_id varchar(64) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT core_prompts_pk PRIMARY KEY (id),
    CONSTRAINT core_unique_prompt_name_per_tenant UNIQUE (tenant_id, name)
);
CREATE INDEX idx_core_prompts_tenant_id ON public.core_prompts ("tenant_id");

CREATE TRIGGER update_core_prompts_modtime
BEFORE UPDATE ON core_prompts
FOR EACH ROW
EXECUTE FUNCTION update_modified_column();
