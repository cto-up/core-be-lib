-- Change role column to roles array to support multiple roles per tenant
ALTER TABLE core_user_tenant_memberships 
    ADD COLUMN roles TEXT[] DEFAULT ARRAY['USER']::TEXT[];

-- Migrate existing data: convert single role to array
UPDATE core_user_tenant_memberships 
SET roles = ARRAY[role]::TEXT[]
WHERE role IS NOT NULL;

-- Drop old role column
ALTER TABLE core_user_tenant_memberships 
    DROP COLUMN role;

-- Add constraint to ensure roles array is not empty
ALTER TABLE core_user_tenant_memberships
    ADD CONSTRAINT roles_not_empty CHECK (array_length(roles, 1) > 0);

-- Create GIN index for efficient role queries
CREATE INDEX idx_user_tenant_memberships_roles ON core_user_tenant_memberships USING GIN(roles);

-- Possible roles: USER, ADMIN, OWNER (can have multiple)
-- Example: roles = ['USER', 'ADMIN'] means user has both USER and ADMIN roles
