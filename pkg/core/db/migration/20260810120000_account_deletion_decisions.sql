-- +goose Up
BEGIN;

-- What the person chose to happen to the content they own, per tenant, answered
-- while closing the account and replayed when the deletion actually runs (ADR
-- 040 §4). Empty means "nobody said" — every module falls back to its own
-- policy, which is what happens for accounts closed before this existed.
ALTER TABLE core_users
    ADD COLUMN deletion_decisions JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMIT;

-- +goose Down
BEGIN;

ALTER TABLE core_users DROP COLUMN deletion_decisions;

COMMIT;
