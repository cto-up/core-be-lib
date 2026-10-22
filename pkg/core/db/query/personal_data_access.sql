-- name: InsertPersonalDataAccess :exec
INSERT INTO core_personal_data_access_log (
    tenant_id, actor_user_id, subject_user_id, resource, resource_id
) VALUES (
    @tenant_id, @actor_user_id, @subject_user_id, @resource, sqlc.narg(resource_id)
);

-- name: ListPersonalDataAccess :many
-- Newest first. A NULL filter matches everything; a NULL tenant is the
-- super-admin's view across tenants.
SELECT * FROM core_personal_data_access_log
WHERE (sqlc.narg(tenant_id)::text IS NULL OR tenant_id = sqlc.narg(tenant_id)::text)
  AND (sqlc.narg(subject_user_id)::text IS NULL OR subject_user_id = sqlc.narg(subject_user_id)::text)
  AND (sqlc.narg(actor_user_id)::text IS NULL OR actor_user_id = sqlc.narg(actor_user_id)::text)
  AND (sqlc.narg(accessed_from)::timestamptz IS NULL OR accessed_at >= sqlc.narg(accessed_from)::timestamptz)
  AND (sqlc.narg(accessed_to)::timestamptz IS NULL OR accessed_at < sqlc.narg(accessed_to)::timestamptz)
ORDER BY accessed_at DESC
LIMIT @row_limit OFFSET @row_offset;

-- name: PurgePersonalDataAccessBefore :execrows
DELETE FROM core_personal_data_access_log
WHERE accessed_at < @cutoff;
