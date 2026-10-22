-- +goose Up
-- One row each time a staff member reads another person's personal data
-- (RGPD Art. 32, CNIL 2021 logging recommendation). Who, whose, what kind and
-- when — never the content read. Kept 12 months, then purged by the sweeper.
CREATE TABLE core_personal_data_access_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    actor_user_id TEXT NOT NULL,
    subject_user_id TEXT NOT NULL,
    resource TEXT NOT NULL,
    resource_id TEXT,
    accessed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_core_pda_log_tenant_subject
    ON core_personal_data_access_log (tenant_id, subject_user_id, accessed_at);

-- +goose Down
DROP TABLE IF EXISTS core_personal_data_access_log;
