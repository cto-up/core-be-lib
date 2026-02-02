-- Critical for tenant-scoped queries
CREATE INDEX idx_user_tenant_memberships_tenant_status 
ON core_user_tenant_memberships(tenant_id, status) 
INCLUDE (user_id, roles);

-- For user lookups
CREATE INDEX idx_user_tenant_memberships_user_status 
ON core_user_tenant_memberships(user_id, status);

-- For email searches
CREATE INDEX idx_users_email_upper 
ON core_users(UPPER(email));

-- For role-based queries
CREATE INDEX idx_users_roles_gin 
ON core_users USING GIN(roles);

-- Composite for common access patterns
CREATE INDEX idx_user_tenant_memberships_composite 
ON core_user_tenant_memberships(user_id, tenant_id, status);

-- Ensure status is valid
ALTER TABLE core_user_tenant_memberships
ADD CONSTRAINT check_valid_status 
CHECK (status IN ('active', 'inactive', 'pending', 'suspended'));

-- Ensure joined_at is set for active memberships
ALTER TABLE core_user_tenant_memberships
ADD CONSTRAINT check_active_has_joined 
CHECK (status != 'active' OR joined_at IS NOT NULL);