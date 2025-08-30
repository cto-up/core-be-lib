-- name: CreateEmailVerificationToken :one
INSERT INTO core_email_verification_tokens (
  user_id, tenant_id, token, token_hash, expires_at
) VALUES (
  $1, $2, $3, $4, $5
)
RETURNING id, user_id, tenant_id, token, expires_at, used_at, created_at, updated_at;

-- name: GetEmailVerificationToken :one
SELECT * FROM core_email_verification_tokens
WHERE token = $1 
AND tenant_id = $2 
AND expires_at > clock_timestamp()
AND used_at IS NULL
LIMIT 1;

-- name: GetEmailVerificationTokenByUserID :one
SELECT * FROM core_email_verification_tokens
WHERE user_id = $1 
AND tenant_id = $2
AND expires_at > clock_timestamp()
AND used_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: MarkEmailVerificationTokenAsUsed :exec
UPDATE core_email_verification_tokens 
SET used_at = clock_timestamp()
WHERE token = $1 AND tenant_id = $2;

-- name: DeleteExpiredEmailVerificationTokens :exec
DELETE FROM core_email_verification_tokens 
WHERE expires_at < clock_timestamp();

-- name: DeleteEmailVerificationTokensByUserID :exec
DELETE FROM core_email_verification_tokens 
WHERE user_id = $1 AND tenant_id = $2;


