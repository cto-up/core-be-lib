# ADR 003: An administrator exercises a member's data-protection rights

Status: accepted

## Context

People often ask their organization, not the platform, to send them their data
or to delete their account. Until now an admin's tools went around the
membership lifecycle: `remove-from-tenant` (no leave impacts, no decisions, no
dormancy purge) and the super-admin hard delete (no grace, no export, no log).
The member's own rights already had one implementation
(`MembershipLifecycleService`: Export, LeavePreview, Leave, ScheduleDeletion).

## Decision

The on-behalf actions reuse that service, behind the user handlers' guards
(`refuseSelf`, `refuseIfTargetOutranks`, plus a refusal when the target holds
the global SUPER_ADMIN role and the caller does not). They are `userOps`
methods, so `/admin-api` (host tenant) and `/superadmin-api/v1/tenants/{id}`
(named tenant) share one body:

| Endpoint (`…/users/{userid}/…`) | Does                                   | Tenant ADMIN, member of other tenants |
| ------------------------------- | -------------------------------------- | ------------------------------------- |
| `GET export`                    | `Export`, logged as `core.user_export` | 409 `member_of_other_tenants`         |
| `GET leave-preview`             | `LeavePreview` for this tenant         | allowed                               |
| `POST leave` `{decisions}`      | `Leave` → 204 (409 + preview if open)  | allowed                               |
| `POST deletion` `{reason?}`     | `ScheduleDeletionByAdmin`              | 409 `member_of_other_tenants`         |

**The rule.** Export and closure reach every organization the person belongs to,
so a tenant ADMIN may do them only for somebody with no other active membership.
A SUPER_ADMIN may always. Leaving concerns the admin's own tenant only, so it is
always theirs. The 409 body is `MemberActionRefusal {reason, message}`; it never
names the other organizations.

**Closure.** `ScheduleDeletionByAdmin` is `ScheduleDeletion` without the typed
email confirmation (that check belongs to the holder), with the reason
`closed by an administrator[: <admin's reason>]`, no stored decisions (each
module's policy applies), and an email built from
`templates/account-closure-scheduled-by-admin.html` giving the date and the way
to keep the account (Organizations page → Keep my account). The account appears
in `GET /admin-api/v1/pending-deletions`. A failed email is logged and does not
undo the closure.

## Consequences

- Each consumer copies `templates/account-closure-scheduled-by-admin.html` into
  its own `templates/`, like `account-inactive-warning.html`; without it the
  closure still happens and the notice is logged as not sent.
- `DELETE /api/v1/users/{userid}/remove-from-tenant` and the super-admin hard
  delete are unchanged; the admin UI uses the leave flow instead of the former.
