
-- Drop indexes first
DROP INDEX IF EXISTS idx_core_translations_tenant;
DROP INDEX IF EXISTS idx_core_translations_field;
DROP INDEX IF EXISTS idx_core_translations_language;
DROP INDEX IF EXISTS idx_core_translations_entity;

-- Drop the translations table
DROP TABLE IF EXISTS public.core_translations;

