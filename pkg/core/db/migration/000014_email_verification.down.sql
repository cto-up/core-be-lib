-- Drop function
DROP FUNCTION IF EXISTS cleanup_expired_email_verification_tokens();

-- Drop trigger
DROP TRIGGER IF EXISTS update_email_verification_tokens_modtime ON core_email_verification_tokens;

-- Drop indexes
DROP INDEX IF EXISTS idx_email_verification_tokens_tenant_id;
DROP INDEX IF EXISTS idx_email_verification_tokens_expires_at;
DROP INDEX IF EXISTS idx_email_verification_tokens_token;
DROP INDEX IF EXISTS idx_email_verification_tokens_user_id;

-- Drop email verification tokens table
DROP TABLE IF EXISTS public.core_email_verification_tokens;
