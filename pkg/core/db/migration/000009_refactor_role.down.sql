DROP INDEX IF EXISTS idx_core_users_roles;

ALTER TABLE core_users 
DROP COLUMN roles;
