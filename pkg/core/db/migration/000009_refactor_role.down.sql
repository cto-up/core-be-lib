DROP INDEX IF EXISTS idx_core_users_roles;

ALTER TABLE public.core_users 
DROP COLUMN roles;
