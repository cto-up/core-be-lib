-- Remove the foreign key constraint
ALTER TABLE core_user_tenant_memberships
DROP CONSTRAINT IF EXISTS fk_user;
