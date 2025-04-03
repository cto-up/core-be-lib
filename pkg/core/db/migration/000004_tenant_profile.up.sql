ALTER TABLE core_tenants
    ADD COLUMN IF NOT EXISTS
    profile jsonb NOT NULL DEFAULT '{}';

ALTER TABLE core_tenants
    ADD COLUMN IF NOT EXISTS
    features jsonb NOT NULL DEFAULT '{}';
