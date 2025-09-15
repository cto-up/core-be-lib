-- Create email verification tokens table
CREATE TABLE core_email_verification_tokens (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    user_id varchar NOT NULL,
    tenant_id varchar(64) NOT NULL,
    token varchar(255) NOT NULL UNIQUE,
    token_hash bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT email_verification_tokens_pk PRIMARY KEY (id),
    CONSTRAINT fk_email_verification_user FOREIGN KEY (user_id) REFERENCES core_users(id) ON DELETE CASCADE
);

-- Create indexes for performance
CREATE INDEX idx_email_verification_tokens_user_id ON core_email_verification_tokens(user_id);
CREATE INDEX idx_email_verification_tokens_token ON core_email_verification_tokens(token);
CREATE INDEX idx_email_verification_tokens_expires_at ON core_email_verification_tokens(expires_at);
CREATE INDEX idx_email_verification_tokens_tenant_id ON core_email_verification_tokens(tenant_id);

-- Create trigger for updated_at
CREATE TRIGGER update_email_verification_tokens_modtime
BEFORE UPDATE ON core_email_verification_tokens
FOR EACH ROW
EXECUTE FUNCTION update_modified_column();

-- Create function to clean up expired tokens
CREATE OR REPLACE FUNCTION cleanup_expired_email_verification_tokens()
RETURNS void AS $$
BEGIN
    DELETE FROM core_email_verification_tokens 
    WHERE expires_at < clock_timestamp() - INTERVAL '7 days';
END;
$$ LANGUAGE plpgsql;
