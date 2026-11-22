# Decision memo — Cloud API and MySQL

**Status:** decision memo; not scheduled.
**Date:** 2026-11-22

## 1. WhatsApp Cloud API (Meta)

### What it is

An official Meta transport: inbound webhooks (JSON), outbound Graph API calls,
message templates, per-message pricing and a different message model from
WhatsApp Web. Reforged supports it as an instance integration.

### Cost

- 4–8 weeks for a production-quality provider: webhook verification, Graph
  client, template lifecycle, media upload/download, status callbacks, error
  mapping, per-instance credentials, rate limits and tests.
- Ongoing: Meta API versions, policy and pricing changes; a second set of
  message semantics (`template` vs free-form, 24-hour windows) in the manager,
  history and events.
- It does not reuse whatsmeow or the current send pipeline; it is effectively a
  second product behind the same API.

### Why WaMux might do it

- Customers who need official WhatsApp compliance, higher volume or templates.
- The manager, auth, events, S3 and audit can be shared.

### Why not now

- It dilutes the Go/whatsmeow performance and reliability story.
- It is a large surface with a different operational model (Meta app, tokens,
  webhooks, billing).
- Reforged already serves that need and is a drop-in Evolution API distribution.

### Recommendation

**Do not pursue unless a paying deployment requires official Meta messaging.**
If it becomes required, the concrete plan is in
[`../plans/waba-cloud-api.md`](../plans/waba-cloud-api.md): a separate channel
adapter, signed webhooks, encrypted credentials, text/media/templates first,
then interactive messages and Embedded Signup.

## 2. MySQL

### What it is

A second SQL dialect for the users database. WaMux is PostgreSQL-first; several
hot paths use Postgres-specific SQL (recursive CTE, `ON CONFLICT`, array
functions, trigger-based counters) and the auth DB uses Postgres advisory locks
for single-writer ownership.

### Cost

- 2–4 weeks to make migrations, queries, upserts and tests dialect-neutral.
- Ongoing: every new query must be tested on both engines.
- The ownership guarantee needs a MySQL equivalent (GET_LOCK or a lease-only
  design), which is weaker or more complex than `pg_advisory_lock`.
- SQLite already covers local development; production is documented as Postgres.

### Why not

- No user has asked; the operational cost is permanent.
- Reforged's own MySQL support is not at parity with its fork-specific
  PostgreSQL migrations, which is evidence the dual-dialect tax is real.

### Recommendation

**Stay PostgreSQL-only for production.** Keep SQLite for tests/development of the
auth DB. Revisit only with a concrete customer requirement and a funded
migration, not as a parity exercise.
