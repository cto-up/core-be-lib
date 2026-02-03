-- +goose Up
UPDATE core_users 
SET roles = (
    SELECT ARRAY_AGG(cr.name ORDER BY cr.name)
    FROM core_roles cr
    WHERE cr.id = ANY(core_users.core_roles)
    AND core_users.core_roles IS NOT NULL
)
WHERE core_users.core_roles IS NOT NULL;
-- +goose Down
DO $$
BEGIN
    -- Begin transaction manually
    BEGIN
        -- Restore core_roles from roles
        UPDATE core_users 
        SET core_roles = (
            SELECT ARRAY_AGG(cr.id ORDER BY cr.id)
            FROM core_roles cr
            WHERE cr.name = ANY(core_users.roles)
            AND core_users.roles IS NOT NULL
        )
        WHERE core_users.roles IS NOT NULL;

        -- Verification
        DECLARE
            users_with_roles INTEGER;
            users_with_core_roles INTEGER;
            rollback_success BOOLEAN;
        BEGIN
            SELECT COUNT(*) INTO users_with_roles 
            FROM core_users 
            WHERE roles IS NOT NULL AND array_length(roles, 1) > 0;
            
            SELECT COUNT(*) INTO users_with_core_roles 
            FROM core_users 
            WHERE core_roles IS NOT NULL AND array_length(core_roles, 1) > 0;
            
            rollback_success := (users_with_roles = users_with_core_roles);
            
            RAISE NOTICE 'Rollback verification:';
            RAISE NOTICE 'Users with roles: %', users_with_roles;
            RAISE NOTICE 'Users with core_roles: %', users_with_core_roles;
            RAISE NOTICE 'Rollback successful: %', rollback_success;
            
            IF NOT rollback_success THEN
                RAISE EXCEPTION 'Rollback verification failed. Rolling back.';
            END IF;
        END;

        -- Commit happens automatically if no exception
    EXCEPTION
        WHEN OTHERS THEN
            -- Rollback explicitly
            RAISE NOTICE 'Rolling back due to error: %', SQLERRM;
            RAISE;
    END;
END $$;
