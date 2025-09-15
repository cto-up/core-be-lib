UPDATE core_users 
SET roles = (
    SELECT ARRAY_AGG(cr.name ORDER BY cr.name)
    FROM core_roles cr
    WHERE cr.id = ANY(core_users.core_roles)
    AND core_users.core_roles IS NOT NULL
)
WHERE core_users.core_roles IS NOT NULL;