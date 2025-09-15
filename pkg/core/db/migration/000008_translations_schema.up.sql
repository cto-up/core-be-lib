-- Create a translations table for all entities
CREATE TABLE core_translations (
    id uuid NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    entity_type varchar NOT NULL, -- 'business_sector', 'commercial_level', etc.
    entity_id uuid NOT NULL,
    language varchar(10) NOT NULL,
    field varchar(180) NOT NULL, -- field name to translate
    value varchar(180) NOT NULL, -- translation
    tenant_id varchar NOT NULL,
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (entity_type, field, entity_id, language, tenant_id)
);

-- Add indexes for better performance
CREATE INDEX idx_core_translations_entity ON core_translations (entity_type, entity_id);
CREATE INDEX idx_core_translations_language ON core_translations (language);
CREATE INDEX idx_core_translations_field ON core_translations (field);
CREATE INDEX idx_core_translations_tenant ON core_translations (tenant_id);

