# ADR 001: Accounts inactive for N months are warned, then closed

Status: accepted · follow-up to lms ADR 040 (self-service membership lifecycle)

## Context

RGPD Art. 5(1)(e) (storage limitation): an account nobody uses must not keep its
data forever. ADR 040 gave coreapp a deletion path — `ScheduleDeletion`, a 30-day
grace, contributor `Erase(EraseAccountDeletion)` — but only the owner could start
it. `core_users.last_seen_at` (stamped by `LastSeenRecorder`) already records
activity; nothing swept on it.

## Decision

A third step of the lifecycle sweeper, `MembershipLifecycleService.ProcessInactiveAccounts`:

- **Setting.** Global config `core.retention.inactive_account_months`
  (`core_global_configs`). Absent: 36. `0`: the rule is off. `1`, a negative or a
  non-number is refused and the step does nothing (an error is logged), so a typo
  can never delete accounts.
- **Warning.** At month N−1 without a request, the `account-inactive-warning.html`
  email is sent and `core_users.inactivity_warned_at` stamped. The stamp is a
  claim (`WHERE inactivity_warned_at IS NULL`), so two sweeper replicas send one
  email; if the email cannot be sent the stamp is withdrawn and retried next run.
- **Closing.** An account warned at least one month ago and quiet for N months is
  closed through `ScheduleDeletion` with reason `inactivity`. The 30-day grace,
  the cancel path and every contributor's erasure apply unchanged — there is one
  deletion path.
- **Sign of life.** Any authenticated request (`TouchSharedUserLastSeen`) clears
  `inactivity_warned_at` in the same UPDATE that stamps `last_seen_at`.
- **Skipped.** Super-admins (`SUPER_ADMIN` in `core_users.roles`), accounts with a
  deletion already scheduled, and accounts with no email (they cannot be warned,
  so they are never closed for inactivity).
- **Before tracking.** An account with no `last_seen_at` counts from 2026-08-22,
  when stamping began, or from `created_at` if later. Silence we never measured
  is not inactivity.

## Consequences

- Consumers resolve email templates from their own `templates/` directory: each
  must ship `templates/account-inactive-warning.html` (copy it from this repo, or
  override it per domain). Without it no warning is sent — and so nothing is
  closed for inactivity.
- The warning states the closing date: N months after the last activity, but
  never less than a month after the email.
