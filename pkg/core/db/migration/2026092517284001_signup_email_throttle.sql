-- +goose Up
-- Self-service sign-up mails whoever owns the address typed into the form, so
-- without a cap anyone can fill a stranger's inbox by resubmitting it. One row
-- per address, keyed by its hash: the cap needs to recognise the address, not
-- to know it. Shared state because both replicas answer the same form.
CREATE TABLE core_signup_email_throttle (
    email_hash        text        PRIMARY KEY,
    last_sent_at      timestamptz NOT NULL,
    window_started_at timestamptz NOT NULL,
    window_count      integer     NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS core_signup_email_throttle;
