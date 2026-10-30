-- +goose Up
-- hub#652 (ADR 089): the agent module's feature flag, tenant config keys and
-- personal-data resources are named 'agent'; nothing reads 'aiemployee' any
-- more. Rewrite what core stores for it.
UPDATE core_tenants
   SET features = (features - 'aiemployee') || jsonb_build_object('agent', features->'aiemployee')
 WHERE features ? 'aiemployee';
UPDATE core_tenants
   SET feature_licenses = (feature_licenses - 'aiemployee') || jsonb_build_object('agent', feature_licenses->'aiemployee')
 WHERE feature_licenses ? 'aiemployee';
UPDATE core_user_tenant_memberships
   SET feature_licenses = (feature_licenses - 'aiemployee') || jsonb_build_object('agent', feature_licenses->'aiemployee')
 WHERE feature_licenses ? 'aiemployee';

UPDATE core_tenant_configs
   SET name = 'agent.' || substr(name, length('aiemployee.') + 1)
 WHERE name LIKE 'aiemployee.%';

UPDATE core_personal_data_access_log
   SET resource = 'agent.' || substr(resource, length('aiemployee.') + 1)
 WHERE resource LIKE 'aiemployee.%';

-- +goose Down
UPDATE core_tenants
   SET features = (features - 'agent') || jsonb_build_object('aiemployee', features->'agent')
 WHERE features ? 'agent';
UPDATE core_tenants
   SET feature_licenses = (feature_licenses - 'agent') || jsonb_build_object('aiemployee', feature_licenses->'agent')
 WHERE feature_licenses ? 'agent';
UPDATE core_user_tenant_memberships
   SET feature_licenses = (feature_licenses - 'agent') || jsonb_build_object('aiemployee', feature_licenses->'agent')
 WHERE feature_licenses ? 'agent';

UPDATE core_tenant_configs
   SET name = 'aiemployee.' || substr(name, length('agent.') + 1)
 WHERE name LIKE 'agent.%';

UPDATE core_personal_data_access_log
   SET resource = 'aiemployee.' || substr(resource, length('agent.') + 1)
 WHERE resource LIKE 'agent.%';
