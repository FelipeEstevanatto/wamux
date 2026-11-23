# WABA / Cloud API — next work plan

**Status:** implemented (core waves 1–4); Groups/Calling/Flows remain separate programs.
**Baseline:** `develop` after `581471c` / Cloud Phase 1–3, latest migration `0029`.
**Parent plan:** [`waba-cloud-api.md`](./waba-cloud-api.md).
**Companion references:** [`wapi.go`](https://github.com/wapikit/wapi.go),
[`piusalfred/whatsapp`](https://github.com/piusalfred/whatsapp),
[`waba-cli`](https://github.com/jjuanrivvera/waba-cli) and the
[official Meta Cloud API docs](https://developers.facebook.com/documentation/business-messaging/whatsapp/overview).

---

## 0. Implementation status (2026-11-22)

Wave 1–4 core work is implemented in this branch:

- identity continuity, typed webhooks, metrics/health, per-phone limiter;
- `wamuxctl`, template/account-event UI and coverage manifest;
- Business Management assets/analytics/catalogs and payload expansion;
- Embedded Signup backend + manager selection flow.

Groups, Calling and Flows are now implemented as separate opt-in programs
(`pkg/cloud/{groups,calling,flows}`, `CLOUD_GROUPS`, `CLOUD_CALLING`,
`CLOUD_FLOWS`). Calling owns the control plane only; WebRTC/SIP media remains an
external TURN/SFU service. Live Meta staging validation is still an external
acceptance step.

---

## 1. Executive summary

The first Cloud implementation covers the production core: BSUID-first
identity, encrypted credentials, signed webhooks, phone registration, text,
media, templates, interactive messages, the 24-hour window, diagnostics and
manager onboarding.

This plan covers the next layer:

1. **Identity continuity** — keep chats and windows intact when Meta changes a
   BSUID.
2. **Cloud observability** — Graph latency/errors, webhook results, window
   expiries, media backlog and unknown send outcomes.
3. **Per-phone rate limiting** — replace the process-wide limiter with a
   shared, phone-number-scoped bucket.
4. **Typed webhook coverage** — type every event we care about and never drop
   an unknown field silently.
5. **CLI/doctor/dry-run** — a dedicated operator CLI and a coverage manifest.
6. **Template/account-event UI** — finish the manager surface.
7. **Message payload expansion** — catalog/product, product lists, CTA URL,
   contact requests, request-contact-info, carousel/LTO templates.
8. **Business Management API** — phone numbers, WABA assets, analytics,
   catalogs and products.
9. **Embedded Signup** — the real onboarding improvement.
10. **Groups, Calling and Flows** — separate projects, never folded into the
    core channel.

**Estimate:** 11–15 engineer-weeks sequentially for items 1–8; 9–11 weeks if
identity, metrics, rate limiting and typed webhooks run in parallel with the
CLI/UI work. Embedded Signup adds 2–3 weeks. Groups/Calling/Flows are separate
programs.

---

## 2. Principles and invariants

These carry over from the parent plan and must not regress:

- `WHATSAPP-BAILEYS` remains the default; existing rows and behavior do not
  change.
- Cloud credentials stay encrypted and write-only.
- BSUID is the primary Cloud contact identity; phone is optional.
- Every webhook POST is signature-verified over raw bytes.
- `POST /messages` is never automatically retried after an ambiguous timeout.
- The 24-hour window is enforced locally and also mapped from Meta errors.
- No new mandatory runtime service (no Redis/Kafka/MySQL).
- No telemetry, license heartbeat or new outbound service.
- Additive, idempotent migrations; no destructive schema changes.
- Unsupported features return a typed error, not a silent failure.
- Webhook/event payloads are a contract: add fields, never rename/remove.
- Status only moves forward through the existing rank logic.
- New behavior needs tests; API/config changes need docs/Swagger.

---

## 3. Current state and gap map

| Area | Current state | Gap |
|---|---|---|
| Identity | BSUID/phone contacts, windows keyed by `contact_id` | No `user_id_update` merge; no identity history |
| Metrics | Logs and `/cloud/diagnostics` | No Cloud Prometheus series |
| Rate limit | Process-wide adaptive limiter | Not per phone/WABA; not shared across nodes |
| Webhooks | Messages/status + common account/template events | Many fields stored raw; no typed structs |
| CLI | HTTP API + diagnostics endpoint | No `wamuxctl`, dry-run or coverage manifest |
| Templates | Backend CRUD/sync; manager list/sync/delete | Create/edit/carousel UI |
| Account events | Stored in `cloud_account_events` | No list endpoint or manager view |
| Payloads | Text/media/template/location/contact/reaction/buttons/lists | Catalog/product/CTA/contact-request/LTO/carousel |
| Business API | App/subscription, phone registration, templates | Phone assets, WABA info, analytics, catalogs/products |
| Onboarding | Manual credentials | Embedded Signup |
| Groups/Calling/Flows | Capability notes only | Separate projects |

---

## 4. Consolidated workstreams

The original list contains duplicates:

- “Business Management API” and “Full Business Management API surface” are one
  workstream.
- “Groups, Calling, Flows” appears twice; it is one separate program.
- “Broad typed webhook coverage” is a cross-cutting workstream, not a feature
  page.

### Priority order (dependency-aware)

| Wave | Workstreams | Why this order |
|---|---|---|
| 1 | Identity continuity, metrics, per-phone limiter, typed webhook coverage | Correctness and operational safety |
| 2 | CLI/doctor/dry-run, template/account-event UI | Makes the existing feature usable |
| 3 | Message payload expansion, Business Management API | Breadth after the core is observable and usable |
| 4 | Embedded Signup | Onboarding needs more breadth and Meta review |
| Separate | Groups, Calling, Flows | Each has its own infrastructure and product risk |

---

## 5. Workstream A — identity continuity

### A.1 Problem

Meta can create, change or regenerate a BSUID (`user_id_update`), and a user can
change a phone number. Today WaMux stores the current `cloud_contacts` row and
keys messages/windows by `contact_id`; a BSUID change would create a new
contact and split the conversation/window.

### A.2 Source of truth

`wapi.go` has the right shape:

- `UserIdUpdateEvent` fields: WABA id, `wa_id`, `old_user_id`, `new_user_id`,
  `parent_user_id`.
- It treats identity as WABA/portfolio-scoped, never global.

Use Meta's
[BSUID docs](https://developers.facebook.com/documentation/business-messaging/whatsapp/business-scoped-user-ids/)
and the `user_id_update` webhook field as the contract. Keep a raw event even
when fields are absent.

### A.3 Data model

Add:

```text
cloud_identity_events
  id UUID PK
  instance_id UUID
  event_type TEXT          -- user_id_update, customer_identity_changed, ...
  wa_id TEXT
  old_bsuid TEXT
  new_bsuid TEXT
  parent_bsuid TEXT
  event_time TIMESTAMPTZ
  received_at TIMESTAMPTZ
  payload JSONB
  applied_at TIMESTAMPTZ
  error TEXT
```

```text
cloud_contact_identities
  instance_id UUID
  contact_id TEXT
  identity_type TEXT       -- bsuid, parent_bsuid, wa_id, username
  identity_value TEXT
  valid_from TIMESTAMPTZ
  valid_to TIMESTAMPTZ     -- NULL means current
  source_event_id UUID
  PRIMARY KEY (instance_id, identity_type, identity_value, valid_from)
```

Keep `cloud_contacts` as the current projection. Do **not** change historical
`messages.message_id`; only the contact/chat linkage.

### A.4 Merge algorithm

On a `user_id_update`/identity event:

1. Resolve old BSUID contact, new BSUID contact, and phone contact.
2. Choose a canonical `contact_id` with this precedence:
   - phone contact if it exists;
   - old BSUID contact;
   - new BSUID contact;
   - otherwise create a new phone/BSUID contact.
3. In one transaction:
   - update `cloud_contacts` current fields;
   - mark old identity `valid_to=event_time`, insert new identity;
   - update `messages.contact_id` for old rows;
   - update `cloud_chat_windows.contact_id` (preserve the later expiry);
   - update `cloud_user_preferences.contact_id`;
   - update `cloud_status_events.recipient_user_id` when appropriate;
   - leave a tombstone/history row, never delete the old identity.
4. If both contacts have messages in different `chat_jid` values, merge into
   the canonical chat key only when it does not create a duplicate-message
   conflict. Otherwise keep the chat key and record the merge in identity
   history.
5. Publish `CloudIdentityChanged` (new event type) with old/new IDs.
6. Be idempotent: the same event twice must not move rows twice.

### A.5 API

```text
GET /cloud/contacts/:contactId
GET /cloud/contacts/:contactId/identities
GET /cloud/identity-events?before=&limit=
POST /cloud/contacts/:contactId/reconcile   (admin/operator repair)
```

Contact response adds:

```json
{
  "contactId": "phone:5511...",
  "bsuid": "BR.new...",
  "parentBsuid": null,
  "waId": "5511...",
  "username": "ana",
  "identities": [
    {"type":"bsuid","value":"BR.old...","validTo":"..."},
    {"type":"bsuid","value":"BR.new...","validFrom":"..."}
  ],
  "window": {"expiresAt":"..."},
  "messageCount": 42
}
```

### A.6 Tests

- New BSUID, no old contact.
- Old BSUID + new BSUID, no phone.
- Phone exists; BSUID changes; message/window merge.
- Both old and new contacts have messages.
- Duplicate event.
- Event with missing/blank fields.
- Window remains open across the merge.
- `messages.contact_id` and `chat_jid` consistency.
- Migration from existing `cloud_contacts` rows.

### A.7 Estimate

**5–7 days.**

---

## 6. Workstream B — Cloud metrics and health

### B.1 Metrics

Extend the existing dependency-free `pkg/metrics` with a small histogram
primitive (fixed buckets) or, if a histogram is too much, use counters plus a
last-value gauge. Prefer a histogram for Graph latency.

Add:

| Metric | Type | Labels |
|---|---|---|
| `wamux_cloud_graph_requests_total` | counter | `operation`, `result` |
| `wamux_cloud_graph_request_duration_seconds` | histogram | `operation` |
| `wamux_cloud_send_total` | counter | `type`, `result` |
| `wamux_cloud_webhook_total` | counter | `field`, `result` |
| `wamux_cloud_webhook_signature_failures_total` | counter | — |
| `wamux_cloud_window_expired_total` | counter | — |
| `wamux_cloud_media_jobs_total` | counter | `status` |
| `wamux_cloud_media_queue_depth` | gauge | — |
| `wamux_cloud_send_outcome_unknown_total` | counter | — |
| `wamux_cloud_rate_limit_rps` | gauge | `scope` |

Keep labels low-cardinality: never label by phone number or contact id.

### B.2 Wiring

- Add `pkg/cloud/observability.go` with a small `Observer` interface.
- `CloudService` receives an optional observer; all calls are nil-safe.
- `server_handler.Observability` implements the interface and registers the
  series in the same registry as the existing `evo_*` metrics.
- `CloudClient` reports Graph operation/latency/result; `CloudService` reports
  sends, webhooks, media jobs and window expiries.
- `/server/health` adds Cloud counts and last webhook time; `/cloud/diagnostics`
  remains the per-instance view.
- Add alerts runbook:
  - webhook failure rate > 1%;
  - Graph 429 rate;
  - media backlog > 100;
  - unknown send outcomes > 0.

### B.3 Tests

- Registry values after one send/webhook/media job.
- Labels do not include phone numbers.
- Histogram buckets render in Prometheus format.
- 429/error paths increment the right result.
- Unknown outcome increments once.
- Health response shape.

### B.4 Estimate

**3–4 days** (add 2 days if adding a histogram primitive).

---

## 7. Workstream C — CLI, doctor and dry-run

### C.1 Binary

Add a separate `cmd/wamuxctl` binary. Do not overload `cmd/wamux`, which is the
server entrypoint.

Configuration:

```text
WAMUX_URL=https://wamux.example.com
WAMUX_API_KEY=global-admin-key
WAMUX_INSTANCE_TOKEN=optional-instance-token
```

Flags:

```text
--url, --api-key, --instance-token, --json, --timeout, --verbose, --dry-run
```

### C.2 Commands

```text
wamuxctl cloud doctor [--instance id]
wamuxctl cloud status --instance id
wamuxctl cloud credentials get|set --instance id
wamuxctl cloud verify --instance id
wamuxctl cloud register --instance id --pin ******
wamuxctl cloud send text --instance id --to 5511... --text "..."
wamuxctl cloud send media --instance id --to 5511... --file ./x.jpg
wamuxctl cloud send template --instance id --to 5511... --name order_update --lang en_US
wamuxctl cloud send ... --dry-run
wamuxctl cloud templates list|sync|create|update|delete
wamuxctl cloud account-events list
wamuxctl cloud contacts get|identities
wamuxctl cloud api METHOD PATH [--body @file]
wamuxctl cloud coverage
```

### C.3 Doctor checks

- `CLOUD_API_ENABLED`/server reachability.
- Instance credentials present.
- App secret set.
- Token valid (`/cloud/diagnostics`).
- Phone registered.
- App subscribed.
- Last webhook age.
- Template status counts.
- Rate-limit state.
- Last Graph error.
- Unknown send outcomes.

Exit codes: `0` healthy, `1` failed check, `2` invalid arguments.

### C.4 Dry-run safety

- `--dry-run` never sends a message, creates a template or changes credentials.
- It prints method, URL, body and the expected Cloud payload with the access
  token redacted.
- For template operations, print the Graph payload.
- A write without `--dry-run` requires `--yes` if destructive.

### C.5 Coverage manifest

Add `docs/archive/plans/cloud-api-coverage.md` and a machine-readable
`cloud-api-coverage.json`:

| Operation | Endpoint | Permission | Status | Tests |
|---|---|---|---|---|
| send text | `POST /messages` | messaging | implemented | yes |
| create catalog | `POST /{business-id}/owned_product_catalogs` | management | deferred | — |
| groups create | `POST /{phone-number-id}/groups` | management | separate | — |

CI test: every route in the manifest has a matching handler or an explicit
deferred reason; every current Cloud route appears in the manifest.

### C.6 Tests

- Golden command output (`--json` and table).
- Doctor failure/success exit codes.
- Dry-run makes zero HTTP requests.
- Token redaction.
- CLI config precedence.
- Coverage manifest route check.
- Network error and 401 handling.

### C.7 Estimate

**5–7 days.**

---

## 8. Workstream D — template and account-event UI

### D.1 Backend

Add:

```text
GET /cloud/account-events?field=&before=&limit=
GET /cloud/account-events/:id
GET /cloud/templates/:name/:language
```

`GET /cloud/templates` already exists. Keep `POST/PUT/DELETE` existing routes.
Include status, quality, category, rejected reason, components and last sync.

### D.2 Manager

Split `CloudSettingsCard` into tabs:

- **Overview** — verified, registered, subscribed, quality, last webhook.
- **Credentials** — app/WABA/phone/token.
- **Phone** — register/deregister, PIN input.
- **Templates** — list/sync/create/edit/delete.
- **Account events** — template quality/status/category, phone quality,
  security, username, user preference.
- **Diagnostics** — token, app secret, webhook, rate limit, recent errors.

Add a `TemplateManager` with:

- name, language, category;
- header/body/footer;
- buttons, carousel/LTO components;
- example values;
- validation before submit;
- status/quality/rejected reason;
- delete confirmation.

Add `AccountEventsPanel`:

- filter by field;
- before/limit paging;
- newest first;
- no secrets or raw phone numbers in logs.

Add capability-aware send UI:

- Cloud hides albums/polls/carousels (until supported);
- Cloud shows templates and window state;
- Cloud shows failed errors and unknown-outcome state.

### D.3 Tests

- Create template validation.
- Template status/quality rendering.
- Delete confirmation.
- Account event filters/paging.
- Cloud credentials redaction.
- Capability gating.
- Window-closed send path.

### D.4 Estimate

**5–7 days.**

---

## 9. Workstream E — Business Management API

### E.1 Scope

Split into three sub-projects so phone support is not blocked by catalogs:

**E1. Phone numbers and WABA assets**

- `GET /{waba-id}/phone_numbers`
- `GET /{phone-number-id}`
- `POST /{phone-number-id}/register`
- `POST /{phone-number-id}/deregister`
- `POST /{phone-number-id}/request_code`
- `POST /{phone-number-id}/verify_code`
- `GET /{waba-id}?fields=...`
- `GET /{business-id}/owned_whatsapp_business_accounts`
- `GET /{business-id}/client_whatsapp_business_accounts`

**E2. Analytics**

- `GET /{waba-id}/analytics`
- messaging, conversation, pricing and template analytics;
- date range and granularity;
- cache short-lived results; do not persist raw analytics.

**E3. Catalogs and products**

- `GET/POST /{business-id}/owned_product_catalogs`
- `GET/POST /{catalog-id}/products`
- `GET/POST /{catalog-id}/product_sets`
- only after message payload support for product/catalog is ready.

### E.2 API routes

```text
GET  /cloud/business/phone-numbers
POST /cloud/business/phone-numbers/:id/register
POST /cloud/business/phone-numbers/:id/deregister
POST /cloud/business/phone-numbers/:id/request-code
POST /cloud/business/phone-numbers/:id/verify-code
GET  /cloud/business/waba
GET  /cloud/business/analytics
GET  /cloud/business/catalogs
POST /cloud/business/catalogs
GET  /cloud/business/catalogs/:id/products
POST /cloud/business/catalogs/:id/products
```

Use `AuthAdmin` for cross-WABA operations; instance-token routes may expose only
the instance’s own WABA/phone. Never return access tokens.

### E.3 Data model

Prefer live Graph queries with short in-process caches. Persist only what the
manager needs for stable UX:

```text
cloud_phone_numbers
  instance_id, phone_number_id, display_phone_number, verified_name,
  quality_rating, status, registered, synced_at

cloud_catalogs
  instance_id, catalog_id, name, product_count, synced_at
```

Do not duplicate Meta as a full local catalog database.

### E.4 Tests

- Fake Graph fixtures for phone list/register/analytics/catalogs.
- Pagination/cursors; do not follow absolute `paging.next`.
- Permission errors `10`, `200`, `133010`.
- Instance token cannot read another WABA.
- Empty/expired analytics response.
- Catalog product pagination.

### E.5 Estimate

- E1: **5–7 days**
- E2: **3–4 days**
- E3: **5–7 days**
- Full E: **2–3 weeks**

---

## 10. Workstream F — message payload expansion

### F.1 Additive channel DTOs

Extend `pkg/channel`:

- `CTAURLMessage`
- `ProductMessage`
- `ProductListMessage`
- `CatalogMessage`
- `ContactRequestMessage`
- `RequestContactInfoMessage`
- interactive `Flow` and `CTA URL` variants
- template `Carousel` and `LimitedTimeOffer` components

### F.2 Graph payloads

- CTA URL: `interactive.type=cta_url`.
- Product: `interactive.type=product` with `catalog_id` and
  `product_retailer_id`.
- Product list: `interactive.type=product_list`.
- Catalog: `interactive.type=catalog_message`.
- Contact request: `interactive.type=request_contact_info` on supported
  templates/messages.
- Request contact info template button.
- Template carousel: `type=carousel`, cards with header/body/buttons.
- Limited-time-offer: `type=limited_time_offer` with expiry.
- Validate current Meta schema from official docs at implementation time.

### F.3 Router/routes

Map existing structs where possible:

| WaMux | Cloud |
|---|---|
| `/send/product` | product template or interactive product |
| `/send/carousel` | template carousel (or explicit unsupported if no approved template) |
| `/send/button` URL | `cta_url` |
| new `/send/contact-request` | `request_contact_info` |
| new `/send/request-contact-info` | template/interactive contact request |
| new `/send/catalog` | catalog message |

Return `unsupported_on_channel` for Baileys or when a catalog/template does not
exist. Do not fake an approved template.

### F.4 Tests

- Payload fixture for each type.
- Catalog ID/product ID validation.
- Window rules: free-form product/interactive inside; templates outside.
- BSUID target.
- Unsupported/invalid type.
- BSUID `131062` mapping.

### F.5 Estimate

**6–10 days** depending on carousel/LTO and catalog scope.

---

## 11. Workstream G — per-phone-number rate limiting

### G.1 Problem

The current limiter is process-wide. Meta limits are per app/WABA/phone number
and percentage-budget-based; two Cloud instances on different phone numbers
should not share the same bucket, and multiple WaMux nodes should not each send
at the full rate.

### G.2 Design

Add `cloud_rate_buckets`:

```sql
CREATE TABLE cloud_rate_buckets (
  phone_number_id TEXT PRIMARY KEY,
  tokens DOUBLE PRECISION NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  effective_rps DOUBLE PRECISION NOT NULL,
  last_429_at TIMESTAMPTZ
);
```

Add `pkg/cloud/ratelimit.go`:

- `Limiter.Wait(ctx, phoneNumberID) error`
- `Limiter.Observe(phoneNumberID, resp)` for 429/usage headers.
- Postgres implementation:
  - transaction with `SELECT ... FOR UPDATE`;
  - refill `tokens = LEAST(burst, tokens + elapsed*rate)`;
  - if `tokens >= 1`, subtract and commit; otherwise wait until refill.
- SQLite/in-memory fallback for tests and single-node dev.
- Config:
  - `CLOUD_API_RPS` default;
  - `CLOUD_API_BURST` default;
  - optional per-credential overrides.
- 429: halve the phone's `effective_rps`, respect `Retry-After`, recover
  gradually (+25% per 10s of calm), cap at configured rate.
- Never retry a POST; the limiter only delays the first attempt.

### G.3 Tests

- 100 concurrent waits respect the budget.
- Two phone numbers have independent buckets.
- 429 halves only that phone.
- Recovery after quiet period.
- Postgres persistence across service instances.
- SQLite fallback.
- Context cancellation.
- `Retry-After` wins over computed delay.

### G.4 Estimate

**4–6 days.**

---

## 12. Workstream H — Embedded Signup

### H.1 Scope

A Meta Embedded Signup wizard that lets a customer connect their own WABA and
phone number without manually copying IDs/tokens.

### H.2 Flow

1. Admin configures the Meta app, Facebook Login for Business configuration ID,
   redirect URI and permissions.
2. Operator clicks **Connect with Meta** in WaMux.
3. Frontend launches Facebook Login for Business with the config ID.
4. Callback receives the code/state.
5. Backend exchanges the code for a business token.
6. Backend discovers shared WABAs and phone numbers.
7. Operator selects WABA/phone number.
8. WaMux creates the Cloud instance/credentials, writes an encrypted token,
   subscribes the app and verifies registration.
9. Audit the onboarding; never expose the token.

### H.3 Security

- CSRF `state` stored server-side with short TTL.
- Validate redirect URI exactly.
- No token in URL fragments/logs.
- Tenant-scoped state; one operator cannot attach another tenant's WABA.
- Rate-limit onboarding attempts.
- Token expiry/rotation support.

### H.4 UI/API

```text
POST /cloud/embedded/start
GET  /cloud/embedded/callback
POST /cloud/embedded/select
GET  /cloud/embedded/assets
```

Manager: `EmbeddedSignupWizard.tsx`.

### H.5 Tests

- Fake code exchange/token exchange.
- State mismatch/expiry.
- No shared WABA.
- Multiple WABAs/phone numbers.
- Token redaction.
- App subscription failure.
- Audit.
- Real Meta sandbox/staging test.

### H.6 Prerequisites

- Meta app review and permissions.
- Privacy/terms update.
- Business verification.
- Public HTTPS callback.

### H.7 Estimate

**10–15 days**, plus Meta review wait.

---

## 13. Workstream I — Groups, Calling and Flows

These are **separate projects**. Do not add them to `pkg/cloud` core or the
existing send router until each has its own design and flag.

### Groups API

- OBA only; max 8 participants; max 10,000 groups per business number.
- Text/media/text-template messages only; no interactive/auth/commerce/calls.
- Group creation/deletion, invite links, join requests, participants, settings,
  group webhooks.
- Separate `pkg/cloud/groups` with its own handler/routes/flag.
- Estimate: **2–3 weeks**.

### Calling API

- WebRTC and SIP signaling, call permissions, call settings, `calls` webhooks.
- Media path needs WebRTC/TURN; do not put it in the Cloud send service.
- Separate `pkg/cloud/calling` and operational runbook.
- Estimate: **3–4 weeks** plus infrastructure.

### Flows

- Flow JSON creation/publish, endpoint callbacks, `nfm_reply`, Flow status
  webhooks, encryption/signing.
- Separate `pkg/cloud/flows`.
- Estimate: **2–4 weeks**.

### Analytics

Analytics is part of Business Management (E2), not the messaging core.

---

## 14. Workstream J — typed webhook coverage

### J.1 Typed structs

Move webhook types from `webhook.go` into `pkg/cloud/webhook_types.go` and add
fields for:

- `user_id_update`
- `customer_identity_changed`
- `customer_number_changed`
- `message_template_status_update`
- `message_template_quality_update`
- `template_category_update`
- `message_template_components_update`
- `phone_number_quality_update`
- `account_alerts`
- `account_review_update`
- `business_capability_update`
- `security`
- `business_username_updates`
- `user_preferences`
- `payment_configuration_update`
- `partner_solutions`
- `history`
- `smb_message_echoes`
- `calls`
- group/flow fields as deferred stubs

### J.2 Fallback behaviour

- Unknown field: persist `cloud_account_events` with `field`, raw `payload`,
  `event_type="unknown"`, and increment a metric.
- Never return 500 for an unknown field; return 200.
- Publish `CloudAccountEvent` with typed data when known.
- Keep a `webhook field → typed handler` table and a coverage test.

### J.3 Tests

- Fixture per field.
- Unknown field.
- Malformed field.
- Multiple entries/changes.
- Duplicate delivery.
- Signature/body limits.
- Event payload shape.

### J.4 Estimate

**5–8 days** for the core fields; group/call/flow-specific types follow their
separate projects.

---

## 15. Execution plan

### Wave 1 — correctness and operations (4–5 weeks)

| ID | Work | Depends on | Est. |
|---|---|---|---:|
| W1.1 | Identity tables/migration and merge algorithm | — | 3 d |
| W1.2 | `user_id_update` webhook + API | W1.1 | 2 d |
| W1.3 | Identity tests/fixtures | W1.1 | 1 d |
| W1.4 | Cloud metrics/histogram | — | 3 d |
| W1.5 | Per-phone limiter/table | migration 0030 | 4 d |
| W1.6 | Typed webhook structs/fallback | — | 5 d |
| W1.7 | Tests/docs | W1.1–W1.6 | 3 d |

**Exit:** BSUID changes preserve chats/windows; dashboards show Cloud health;
two phone numbers are independently limited; every webhook field is typed or
explicitly stored as unknown.

### Wave 2 — usability (2–3 weeks)

| ID | Work | Depends on | Est. |
|---|---|---|---:|
| W2.1 | `cmd/wamuxctl` skeleton/config | — | 2 d |
| W2.2 | doctor/status/verify/register | W2.1 | 2 d |
| W2.3 | send/template dry-run | W2.1 | 2 d |
| W2.4 | coverage manifest + CI check | W2.1 | 1 d |
| W2.5 | Template manager UI | backend CRUD | 3 d |
| W2.6 | Account events API/UI | typed events | 3 d |
| W2.7 | Tests/docs | W2.1–W2.6 | 2 d |

**Exit:** an operator can diagnose a Cloud instance from the terminal and the
manager can create/inspect templates and account events.

### Wave 3 — breadth (5–7 weeks)

| ID | Work | Depends on | Est. |
|---|---|---|---:|
| W3.1 | CTA URL/contact-request/request-contact-info | — | 2 d |
| W3.2 | Product/catalog/product-list payloads | catalogs | 3 d |
| W3.3 | Template carousel/LTO | — | 3 d |
| W3.4 | Business phone/WABA assets | — | 5 d |
| W3.5 | Analytics | W3.4 | 3 d |
| W3.6 | Catalog/product management | W3.2 | 5 d |
| W3.7 | Tests/docs/coverage updates | W3.1–W3.6 | 4 d |

**Exit:** the advertised Cloud payloads are implemented, and the manager/API can
manage the business assets those payloads need.

### Wave 4 — Embedded Signup (2–3 weeks)

- Meta app review, permissions and callback.
- Wizard/API/state/token exchange.
- Asset selection and instance creation.
- Token rotation.
- Sandbox/real staging test.

### Separate programs

| Program | Estimate | Gate |
|---|---:|---|
| Groups | 2–3 weeks | OBA + product need |
| Calling | 3–4 weeks | WebRTC/SIP + ops |
| Flows | 2–4 weeks | Flow authoring/endpoints |

Do not put these in the 11–15-week estimate.

---

## 16. Data model and migrations

Latest current migration: `0029_cloud_user_preferences`.

Proposed additions:

| Migration | Contents |
|---|---|
| `0030_cloud_identity_events` | raw BSUID/identity events |
| `0031_cloud_contact_identities` | current/historical identity rows |
| `0032_cloud_rate_buckets` | per-phone bucket state |
| `0033_cloud_phone_numbers` | cached phone assets |
| `0034_cloud_catalogs` | cached catalog/product counts |
| `0035_cloud_account_event_indexes` | field/received_at index |

All additive. No existing table rewrites. The identity merge updates rows in
place inside a transaction; it does not change `message_id` or delete history.

---

## 17. API summary

```text
# Identity
GET  /cloud/contacts/:contactId
GET  /cloud/contacts/:contactId/identities
GET  /cloud/identity-events
POST /cloud/contacts/:contactId/reconcile

# Metrics/health
GET  /metrics                         (existing)
GET  /server/health                   (existing, add Cloud fields)
GET  /cloud/diagnostics               (existing)

# CLI-facing routes use the same API; no separate backend.

# Templates/account
GET  /cloud/account-events
GET  /cloud/account-events/:id
GET  /cloud/templates/:name/:language
POST /cloud/templates
PUT  /cloud/templates/:name/:language
DELETE /cloud/templates/:name/:language
POST /cloud/templates/sync

# Business Management
GET  /cloud/business/phone-numbers
POST /cloud/business/phone-numbers/:id/register
POST /cloud/business/phone-numbers/:id/deregister
POST /cloud/business/phone-numbers/:id/request-code
POST /cloud/business/phone-numbers/:id/verify-code
GET  /cloud/business/waba
GET  /cloud/business/analytics
GET  /cloud/business/catalogs
POST /cloud/business/catalogs
GET  /cloud/business/catalogs/:id/products
POST /cloud/business/catalogs/:id/products

# Messaging expansion
POST /send/product
POST /send/product-list
POST /send/catalog
POST /send/contact-request
POST /send/request-contact-info
POST /send/carousel              (template-backed or typed unsupported)
```

---

## 18. CLI contract

Use a separate `cmd/wamuxctl`; it talks only to the WaMux HTTP API.

```text
wamuxctl cloud doctor --instance <id> [--json]
wamuxctl cloud send text --instance <id> --to ... --text ... [--dry-run]
wamuxctl cloud send media --instance <id> --to ... --file ... [--dry-run]
wamuxctl cloud send template --instance <id> --to ... --name ... --lang ...
wamuxctl cloud templates list|sync|create|update|delete
wamuxctl cloud account-events list
wamuxctl cloud business phone-numbers
wamuxctl cloud business analytics --start ... --end ... --granularity DAY
wamuxctl cloud api GET /v27.0/... -q fields=...
```

Rules:

- `--dry-run` never makes a mutating request.
- The access token/Meta token is never printed.
- `doctor` exits non-zero on a failed check.
- Destructive commands require `--yes`.
- JSON output is stable for scripts.

---

## 19. Test plan

### Unit

- Identity merge permutations and idempotency.
- Window/message/chat linkage after merge.
- Metric registration and label cardinality.
- Histogram buckets.
- Per-phone limiter math, 429 recovery, context cancellation.
- Typed webhook fixtures for every field.
- CLI parsers/dry-run/redaction.
- Business Management payload builders/pagination.
- Payload expansion fixtures.
- Template carousel/LTO validation.

### Integration

- Real Postgres for identity/rate-bucket transactions (use
  `EVO_BENCH_POSTGRES_DSN` or the smoke container).
- Fake Graph for phone/analytics/catalog.
- Webhook signed fixtures end-to-end.
- Multi-node rate-limit simulation.
- Manager Vitest for template/account UI and capability gating.

### Contract

- Meta Graph fixtures, no live dependencies in CI.
- One manual staging test per release for:
  - BSUID change;
  - phone registration;
  - template status/quality;
  - catalog/product message;
  - Embedded Signup sandbox.

### Load/race

- 1k webhook burst.
- 100 concurrent sends for one phone and two phones.
- Media backlog drain.
- `go test -race -shuffle=on ./...`.
- `golangci-lint`, manager lint/build/test, image smoke.

---

## 20. Security and compliance

- Identity history contains personal data; scope by `instance_id`, retain per
  policy, and never log raw BSUIDs/phones.
- Business Management tokens remain the same encrypted access token; do not
  create a separate plaintext store.
- CLI must redact tokens in output and shell history guidance.
- Embedded Signup state is CSRF-protected and single-use.
- Dry-run output must redact the access token and app secret.
- Analytics/catalog caches must not leak between tenants.
- Metrics must not label by phone number/contact ID.
- All new admin routes use `AuthAdmin`; instance routes use `Auth`.

---

## 21. Rollout and feature flags

Add separate flags so workstreams can ship independently:

```env
CLOUD_IDENTITY_V2=false
CLOUD_RATE_LIMIT_V2=false
CLOUD_CLI_ENABLED=false
CLOUD_BUSINESS_API=false
CLOUD_PAYLOAD_V2=false
CLOUD_EMBEDDED_SIGNUP=false
CLOUD_GROUPS=false
CLOUD_CALLING=false
CLOUD_FLOWS=false
```

Rollout per wave:

1. Migrations first, flags off.
2. Enable in staging with fake Graph + Postgres.
3. Enable one internal instance.
4. Canary one customer.
5. Open to all.

Rollback: set the flag off; keep schema and data. Identity merges must be
reversible via the identity history/audit, but never by dropping tables.

---

## 22. Acceptance criteria

### Identity

- A BSUID change updates messages/windows/preferences to the canonical contact.
- The chat/window does not split.
- Old identity remains in history.
- Duplicate events are idempotent.
- A cross-tenant identity cannot be merged.

### Metrics

- `/metrics` exposes Graph request count/latency, send results, webhook results,
  window expiries, media backlog, unknown outcomes and rate-limit state.
- No phone-number/contact-id metric labels.
- Health includes Cloud counts and last webhook age.

### CLI

- `wamuxctl cloud doctor` detects invalid token, unregistered phone, missing
  subscription, stale webhook and template problems.
- `--dry-run` sends nothing.
- Coverage manifest has no unaccounted Cloud route.

### UI

- Operator can create/edit/delete/sync templates.
- Account/quality/security/user-preference events are visible and filterable.
- Cloud-only limitations are clear and unsupported controls are hidden.

### Business Management

- Phone numbers can be listed/registered/deregistered.
- WABA assets and analytics are readable with correct permissions.
- Catalogs/products can be listed/created and paginated safely.
- Cross-tenant access is impossible.

### Payload expansion

- CTA URL, product, product list, catalog, contact request and request-contact-
  info payloads pass fixture tests.
- Carousel/LTO templates are created/sent or explicitly unsupported with a
  reason.
- 24-hour window/BSUID rules still apply.

### Rate limiting

- Two phone numbers do not share a bucket.
- 429 affects only the affected phone.
- Multi-node Postgres limiter is correct.
- Send POSTs are still never auto-retried.

### Typed webhooks

- Every field has a typed struct or explicit unknown fallback.
- Unknown fields return 200 and are stored.
- No unknown field is silently dropped.

### Separate programs

- Groups/Calling/Flows remain behind their own flags and modules.
- None of them changes the core send behavior by default.

---

## 23. Risks

| Risk | Impact | Mitigation |
|---|---|---|
| BSUID merge mistakes split history | data loss/duplicate chats | transaction, dry-run/reconcile endpoint, full fixtures |
| Identity history grows | storage/privacy | retention policy, scoped access |
| Metrics cardinality | scrape/memory | fixed labels, no phone/contact IDs |
| Postgres rate limiter contention | send latency | short row lock, per-phone buckets, timeout |
| CLI token exposure | credential leak | redaction, env/file 0600, no shell history |
| Business Management permissions | Graph 10/200 errors | validation, admin-only, clear hints |
| Catalog/product payload drift | rejected messages | fixtures + official docs check |
| Embedded Signup review delay | launch slip | start Meta review early; manual onboarding remains |
| Groups/Calling/Flows scope creep | core destabilization | separate packages, flags and estimates |
| Metrics test environment | missed regressions | render tests + staging scrape |

---

## 24. Reference mapping

| Workstream | Reference to study | What to borrow |
|---|---|---|
| Identity | `wapi.go` `UserIdUpdateEvent` and `user_id_update` | event shape, WABA scoping |
| Metrics | none | WaMux-specific; use existing registry |
| CLI/doctor | `waba-cli` `doctor.go`, `retry.go`, `ratelimit.go` | checks, dry-run, safe retries |
| Template UI | `wapi.go` template components | component/parameter types |
| Business API | `piusalfred` business/analytics packages; `waba-cli` 102/105 ops | coverage and request shapes |
| Payload expansion | `wapi.go` components; `piusalfred` message/interactive | catalog/product/CTA/contact-request shapes |
| Rate limiting | `waba-cli` adaptive limiter | 429/usage behavior |
| Embedded Signup | Meta docs | OAuth/config ID/asset discovery |
| Groups/Calls/Flows | `piusalfred` groups/calls/flows; `waba-cli` | scope and payloads; separate modules |
| Typed webhooks | `wapi.go` events; `piusalfred` webhook fields | exhaustive field list |

Do not blindly copy default Graph versions (the references still use older
defaults in places). Keep WaMux's configurable `v27.0` and contract tests.

---

## 25. Definition of done

- [x] Wave 1 identity/metrics/limiter/webhooks merged and flagged.
- [x] `wamuxctl` doctor/dry-run/coverage merged.
- [x] Template creation and account-event UI merged.
- [x] Payload expansion fixtures and handlers merged.
- [x] Business phone/WABA/analytics/catalog API merged.
- [x] Embedded Signup wizard merged (sandbox test pending external credentials).
- [x] Groups/Calling/Flows each have separate plans, flags and estimates, plus opt-in implementations.
- [x] Full Go race suite, lint, manager tests/lint/build, docs check and image
      smoke pass.
- [ ] Staging Meta test number validates BSUID change, templates, media and
      business assets.
- [ ] Rollout/rollback runbook reviewed.

---

## 26. Sources

Official Meta documentation:

- [WhatsApp Business Platform overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/overview)
- [Business-scoped user IDs](https://developers.facebook.com/documentation/business-messaging/whatsapp/business-scoped-user-ids/)
- [Webhooks overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/overview)
- [Phone number registration](https://developers.facebook.com/documentation/business-messaging/whatsapp/business-phone-numbers/registration)
- [Message templates](https://developers.facebook.com/documentation/business-messaging/whatsapp/templates)
- [Groups API](https://developers.facebook.com/documentation/business-messaging/whatsapp/groups)
- [Cloud API Calling](https://developers.facebook.com/documentation/business-messaging/whatsapp/calling)
- [WhatsApp Flows](https://developers.facebook.com/documentation/business-messaging/whatsapp/flows)
- [Embedded Signup](https://developers.facebook.com/documentation/business-messaging/whatsapp/embedded-signup/overview)
- [Cloud API pricing](https://developers.facebook.com/documentation/business-messaging/whatsapp/pricing)

Reference implementations:

- [`wapikit/wapi.go`](https://github.com/wapikit/wapi.go) — especially
  `pkg/events/user_id_update_event.go`, BSUID base message fields, phone
  registration, template components and typed webhook events.
- [`piusalfred/whatsapp`](https://github.com/piusalfred/whatsapp) — especially
  `webhooks/listener.go` (4 MiB/panic/per-request config), `webhooks/field.go`,
  `groups/`, `calls/`, `flow/`, `business/analytics/`.
- [`jjuanrivvera/waba-cli`](https://github.com/jjuanrivvera/waba-cli) — especially
  `internal/api/retry.go` (idempotent-only retries), `internal/api/ratelimit.go`,
  `internal/api/errors.go`, `commands/doctor.go`, `api-manifest.json`.

Use the official docs as the source of truth when a reference and Meta disagree.
The references have older default Graph versions in places; keep WaMux's
configurable `v27.0` until a contract test proves a newer version.
