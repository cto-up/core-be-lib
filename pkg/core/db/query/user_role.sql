-- name: GetUserRoleByID :one
SELECT core_users.id, core_users.profile, core_users.email, core_users.created_at, core_users.tenant_id, JSON_AGG(core_roles.*) as core_roles
FROM core_users
LEFT JOIN core_roles ON core_users.core_roles @> ARRAY[core_roles.id]
WHERE core_users.id = $1 
AND core_users.tenant_id = sqlc.arg(tenant_id)::text
GROUP BY core_users.id
LIMIT 1;

-- name: ListUsersRoles :many
SELECT core_users.id, core_users.profile, core_users.email, core_users.created_at, core_users.tenant_id, JSON_AGG(core_roles.*) as core_roles
FROM core_users
LEFT JOIN core_roles ON core_users.core_roles @> ARRAY[core_roles.id]
WHERE core_users.tenant_id = sqlc.arg(tenant_id)::text
AND (UPPER(core_users.profile->>'name') LIKE UPPER(sqlc.narg('like')) OR sqlc.narg('like') IS NULL)
GROUP BY core_users.id
ORDER BY core_users.created_at
LIMIT $1
OFFSET $2;
