-- Client Applications schema
CREATE TABLE public.core_client_applications (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    tenant_id VARCHAR(64) NULL, -- NULL means it's a global application (SUPER_ADMIN)
    active BOOLEAN NOT NULL DEFAULT true,
    created_by varchar NOT NULL, -- Reference to the user who created this application
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    last_used_at TIMESTAMPTZ NULL,
    CONSTRAINT client_applications_pk PRIMARY KEY (id)
);

-- Create indexes for better performance
CREATE INDEX idx_client_applications_tenant_id ON public.core_client_applications (tenant_id);
CREATE INDEX idx_client_applications_active ON public.core_client_applications (active);

-- Add trigger for updating the modification timestamp
CREATE TRIGGER update_client_applications_modtime
BEFORE UPDATE ON core_client_applications
FOR EACH ROW
EXECUTE FUNCTION update_modified_column();

-- API Tokens schema with encryption and security best practices
CREATE TABLE public.core_api_tokens (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    client_application_id uuid NOT NULL REFERENCES core_client_applications(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    -- Store only the hash (SHA-256) of the token for security
    token_hash BYTEA NOT NULL,
    -- Token prefix (first 8 chars) stored in plain text for identification
    token_prefix VARCHAR(8) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT false,
    revoked_at TIMESTAMPTZ NULL,
    revoked_reason TEXT NULL,
    revoked_by VARCHAR(128) NULL,
    created_by VARCHAR(128) NOT NULL,
    scopes TEXT[] NULL, -- Array of permission scopes
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    last_used_at TIMESTAMPTZ NULL,
    last_used_ip VARCHAR(45) NULL, -- IPv6 address can be up to 45 chars
    CONSTRAINT api_tokens_pk PRIMARY KEY (id),
    CONSTRAINT unique_token_hash UNIQUE (token_hash)
);

-- Create indexes for better performance
CREATE INDEX idx_api_tokens_client_application_id ON public.core_api_tokens (client_application_id);
CREATE INDEX idx_api_tokens_token_prefix ON public.core_api_tokens (token_prefix);
CREATE INDEX idx_api_tokens_revoked ON public.core_api_tokens (revoked);
CREATE INDEX idx_api_tokens_expires_at ON public.core_api_tokens (expires_at);

-- Add trigger for updating the modification timestamp
CREATE TRIGGER update_api_tokens_modtime
BEFORE UPDATE ON core_api_tokens
FOR EACH ROW
EXECUTE FUNCTION update_modified_column();

-- API Token audit log for security tracking
CREATE TABLE public.core_api_token_audit_logs (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    token_id uuid NOT NULL REFERENCES core_api_tokens(id) ON DELETE CASCADE,
    action VARCHAR(50) NOT NULL, -- CREATE, USE, REVOKE, etc.
    ip_address VARCHAR(45) NULL,
    user_agent TEXT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    additional_data JSONB NULL,
    CONSTRAINT api_token_audit_logs_pk PRIMARY KEY (id)
);

-- Create index for faster queries
CREATE INDEX idx_api_token_audit_logs_token_id ON public.core_api_token_audit_logs (token_id);
CREATE INDEX idx_api_token_audit_logs_timestamp ON public.core_api_token_audit_logs (timestamp);