# ADR 002: One personal-data access log, written by every module

Status: accepted

## Context

RGPD Art. 32 and the CNIL 2021 recommendation on logging: when somebody other
than the data subject reads personal data, it must be traceable. The existing
audit tables record other events (API-token use, MCP tool calls, content edits,
vault decrypts). Without a shared table each app would invent its own.

## Decision

coreapp owns `core_personal_data_access_log` and one call to write it:

```go
service.RecordPersonalDataAccess(ctx, service.PersonalDataAccess{
    ActorUserID: adminID, SubjectUserID: ownerID,
    Resource: "aiemployee.run", ResourceID: runID,
})
```

**Log** a read when the actor is a staff member (ADMIN, CUSTOMER_ADMIN,
SUPER_ADMIN, or a tool acting for one) and the record belongs to another person:
a learner's transcript, run, conversation, roster entry, circle. Call it from the
handler that serves the read, once per record served (a list of N learners'
records is N rows, or one row per subject if the list is per subject).

**Do not log** reads of one's own data (the call drops them: actor == subject),
the content read, request or response bodies, search terms, or anything else
personal beyond the two user ids. `Resource` is a kind (`<module>.<kind>`),
`ResourceID` an opaque id.

The write is best effort and off the request path: it never slows or fails the
read. `TenantID` defaults to the request's tenant when `ctx` is the
`*gin.Context`; set it explicitly otherwise (a super-admin reading across
tenants should name the subject's tenant).

Rows are kept 12 months (`PersonalDataAccessRetentionMonths`) and purged by the
lifecycle sweeper. `GET /admin-api/v1/personal-data-access` lists them — the host
tenant's for an ADMIN, every tenant's for a SUPER_ADMIN on a host with no tenant
— filtered by `subjectUserId`, `actorUserId`, `from`, `to`, paged with `page` /
`pageSize` (at most 100).

## Consequences

- The core server config wires the store (`SetPersonalDataAccessStore`). A
  consumer that builds its server another way must call it, or every record is
  dropped with a warning.
- The log outlives an account deletion for its 12 months: it holds only ids and
  serves the controller's accountability, not the subject's profile.
