-- Add foreign key constraint with CASCADE DELETE for user_id
-- This ensures that when a user is deleted from core_users, 
-- all their tenant memberships are automatically deleted

ALTER TABLE core_user_tenant_memberships
ADD CONSTRAINT fk_user 
FOREIGN KEY (user_id) 
REFERENCES core_users(id) 
ON DELETE CASCADE;
