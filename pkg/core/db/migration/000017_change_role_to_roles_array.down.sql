-- Revert roles array back to single role column

-- Add back role column
ALTER TABLE core_user_tenant_memberships 
    ADD COLUMN role VARCHAR(50);

-- Migrate data: take first role from array (or highest priority role)
UPDATE core_user_tenant_memberships 
SET role = CASE 
    WHEN 'ADMIN' = ANY(roles) THEN 'ADMIN'
    WHEN 'CUSTOMER_ADMIN' = ANY(roles) THEN 'CUSTOMER_ADMIN'
    WHEN 'USER' = ANY(roles) THEN 'USER'
    ELSE roles[1]
END;

-- Make role NOT NULL with default
ALTER TABLE core_user_tenant_memberships 
    ALTER COLUMN role SET NOT NULL,
    ALTER COLUMN role SET DEFAULT 'USER';

-- Drop roles column and related objects
DROP INDEX IF EXISTS idx_user_tenant_memberships_roles;
ALTER TABLE core_user_tenant_memberships DROP CONSTRAINT IF EXISTS roles_not_empty;
ALTER TABLE core_user_tenant_memberships DROP COLUMN roles;
