# WABA / WhatsApp Cloud API — architecture note

**Status:** design rationale, superseded for execution by
[`../plans/waba-cloud-api.md`](../plans/waba-cloud-api.md).
**Date:** 2026-11-22
**Current WaMux:** `1.0.1` + 29 commits
**Reference:** Reforged `3.3.0` (`8c223ce`), `src/api/integrations/channel/meta/`

> **Correction:** Meta's webhook verify token is configured **per Meta app**, not
> per WABA or instance. BSUID/usernames are mandatory, and Cloud API now also
> has Groups, Calling, Flows and typing indicators. See
> [`../plans/waba-cloud-api.md`](../plans/waba-cloud-api.md) Revision 2 for the
> corrected scope and implementation. The snippets below predate those
> corrections.

---

## 0. What “WABA API” means

“WABA” is the **WhatsApp Business Account**. The official transport is the
**WhatsApp Cloud API** (Meta-hosted Graph API). The old on-premise WhatsApp
Business API is no longer a viable target.

Cloud API is fundamentally different from WhatsApp Web/Baileys/whatsmeow:

| | WhatsApp Web (Baileys/whatsmeow) | Cloud API |
|---|---|---|
| Session | QR/pairing, persistent socket | No socket; stateless HTTPS |
| Inbound | Socket events | Meta POSTs webhooks |
| Outbound | Encrypted protobuf | Graph API JSON |
| History | Phone sync + local store | No chat history API |
| Contacts | Local store | No contact list API |
| Groups | Supported | Groups API for OBA (deferred here) |
| Communities/newsletters | Supported | Not supported |
| Free-form messages | Always | Only inside the 24-hour customer service window |
| Templates | Not a concept | Required outside the window; approval workflow |
| Media | Upload/download via WhatsApp | Media ID upload, then download by ID |
| Pricing | Free (unofficial) | Meta conversation pricing |
| Reliability | Socket lifecycle | Webhook retries + Graph rate limits |

So this is a **new channel**, not a new set of send handlers for the existing
one. Some WaMux features (communities, newsletters, albums, polls, status,
full presence, general chat history) have no Cloud API equivalent and must
return a clear “not supported on this channel” error. Groups, Calling, Flows
and a limited typing indicator do exist in Cloud and are deliberately deferred;
the capability response must say that accurately.

---

## 1. How Reforged does it (and what not to copy)

Their implementation is small:

- `src/api/integrations/channel/meta/whatsapp.business.service.ts` (~1,771 lines)
- `meta.controller.ts` (72 lines)
- `meta.router.ts` (24 lines)
- `src/api/services/template.service.ts`
- `src/api/routes/template.router.ts`
- `WA_BUSINESS_*` global config

It selects the provider at instance creation with
`integration: "WHATSAPP-BUSINESS"`, requires an instance `token`, uses the
`number` field as the Cloud `phone_number_id`, and posts to
`https://graph.facebook.com/{version}/{phoneNumberId}/messages`.

Their implementation is a useful map of payload shapes, but **not a production
blueprint**:

- No `X-Hub-Signature-256` verification on the webhook.
- One global verify token for every instance.
- No per-instance credential separation beyond one token.
- No 24-hour-window enforcement.
- No robust Graph rate-limit/backoff handling.
- No template lifecycle beyond a thin Graph wrapper.
- No media size/type/expiry controls.
- No separation between “Wamux instance token” and “Meta access token”.
- Uses `v18.0` as a default; Graph versions age out.

WaMux should implement the same capability with its existing security posture:
encrypted secrets, signed webhooks, tenant scoping, audit and tests.

### 1.1 Does upstream Evolution API meet these requirements?

It depends on what “original” means:

- **Node `evolution-foundation/evolution-api` 2.3.7:** yes, it has a **basic
  functional** Cloud API channel.
- **`evolution-foundation/evolution-go`:** no. That is WaMux’s upstream, and it
  only implements whatsmeow.
- **Reforged 3.3.0:** inherits the Node Cloud API code almost unchanged. It did
  not harden it.

Upstream’s Cloud API is enough for a demo or a simple text/template sender, but
it does **not** meet the production requirements in this plan:

| Requirement | Upstream Evolution API | Reforged |
|---|---|---|
| `WHATSAPP-BUSINESS` channel selector | ✅ | ✅ |
| Per-instance WABA / phone number / token fields | ⚠️ uses `number`, `businessId`, instance `token` | same |
| Dedicated encrypted Meta access token | ❌ (reuses instance token) | ❌ |
| Per-app webhook verify token | ❌ (one global `WA_BUSINESS_TOKEN_WEBHOOK`) | ❌ |
| `X-Hub-Signature-256` verification | ❌ | ❌ |
| Route webhook by `phone_number_id` | ✅ | ✅ |
| Webhook idempotency/replay protection | ❌ | ❌ |
| Text send | ✅ | ✅ |
| Media upload/download | ✅ basic | ✅ basic |
| Template CRUD + send | ✅ basic Graph wrapper | ✅ |
| Location/contact/reaction/interactive | ✅ basic | ✅ |
| 24-hour customer-service window | ❌ | ❌ |
| Graph rate-limit/backoff/error mapping | ❌ | ❌ |
| Media size/type/expiry hardening | ❌ | ❌ |
| Status mapping incl. failed | ⚠️ basic uppercase status | ⚠️ |
| Unsupported-feature gating | ❌ | ❌ |
| Encrypted credentials at rest | ❌ | ❌ |
| Audit log | ❌ | ❌ |
| Cloud API tests | ❌ | ❌ (fork tests do not cover Meta) |
| Manager onboarding/credentials UI | ⚠️ basic | ⚠️ |
| Embedded Signup / token rotation | ❌ | ❌ |

So the correct statement is: **the Node original supports Cloud API, but it does
not meet the production bar described here.** The Reforged fork did not close
that gap. WaMux should use it only as a payload-shape reference, not as a
security or reliability blueprint.

---

## 2. Proposed architecture

### 2.1 Add a channel boundary

Today the send path, instance lifecycle and event handling are all coupled to
`pkg/whatsmeow/service` and `*whatsmeow.Client`. Do **not** bolt Cloud API into
every `SendX` method with `if instance.Integration == ...` branches; that spreads
the difference across the codebase.

Introduce a small channel package:

```go
package channel

type Kind string
const (
    KindWhatsAppWeb Kind = "WHATSAPP-BAILEYS"
    KindCloudAPI    Kind = "WHATSAPP-BUSINESS"
)

type Status struct {
    Connected bool
    LoggedIn  bool
    Detail    string
}

type Channel interface {
    Kind() Kind
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Status(ctx context.Context) (Status, error)

    SendText(ctx context.Context, to string, text string) (SendResult, error)
    SendMedia(ctx context.Context, to string, media Media) (SendResult, error)
    SendTemplate(ctx context.Context, to string, tpl Template) (SendResult, error)
}
```

Then:

- `pkg/channel/baileys` wraps the existing `sendService`/whatsmeow path.
- `pkg/channel/cloud` implements the Graph client.
- `instanceService` selects the adapter from `instance.Integration`.
- Existing `SendService` methods keep their validation but delegate transport to
  the instance’s channel. For unavailable methods (albums, polls, newsletters,
  full presence), the Cloud adapter returns a typed `ErrUnsupportedOnChannel`.
  Groups, Calling and Flows are Cloud-supported but deferred; report them as
  “not implemented in WaMux,” not as absent from Cloud.

This refactor can be staged: first make `SendText`, `SendMedia` and
`SendTemplate` channel-aware; leave the rest on Baileys and return
`unsupported` from Cloud.

### 2.2 Instance model

Add a separate table rather than 15 nullable columns on `instances`:

```sql
-- Superseded by the detailed plan: app secrets and verify-token hashes live in
-- cloud_apps; this table holds only the per-instance WABA/phone/token.
CREATE TABLE cloud_api_credentials (
  instance_id     uuid PRIMARY KEY REFERENCES instances(id) ON DELETE CASCADE,
  app_id          text NOT NULL REFERENCES cloud_apps(app_id),
  waba_id         text NOT NULL,          -- WhatsApp Business Account id
  phone_number_id text NOT NULL,          -- used in Graph URL and webhook routing
  business_id     text,
  access_token_enc text NOT NULL,          -- AES-256-GCM, never serialized
  created_at      timestamptz NOT NULL DEFAULT NOW(),
  updated_at      timestamptz NOT NULL DEFAULT NOW(),
  UNIQUE (phone_number_id)
);
```

Also add `instances.integration` (`WHATSAPP-BAILEYS` default, so existing rows and
all current behavior are untouched). Store the Meta access token with the
existing `tokencrypt`/AES-GCM helper, not the instance `Token` column.

A Cloud instance is always “connected” from WaMux’s point of view once
credentials exist; `Start` verifies the token with a lightweight Graph call
(`GET /{phone-number-id}?fields=display_phone_number,verified_name`), and
`Status` reports the verification result. There is no QR or persistent socket.

### 2.3 Inbound webhook

Add public routes (no `apikey`):

```
GET  /webhook/cloud   -> hub.challenge verification
POST /webhook/cloud   -> message/status events
```

`GET`:

- `hub.mode == subscribe`
- `hub.verify_token` matches a configured **per-Meta-app** verify-token hash;
  respond with `hub.challenge`.
- Deliberately do not reveal whether a token exists (constant-time compare).
- Meta's verify token is app-level; it cannot be unique per WABA/instance under
  one app. The POST signature is the actual authentication.

`POST`:

1. Read the raw body (bounded, e.g. 2 MiB).
2. Verify `X-Hub-Signature-256 = sha256=<HMAC-SHA256(app_secret, raw_body)>` in
   constant time. Reject unsigned/invalid requests with 401.
3. Parse `object == whatsapp_business_account` and iterate `entry[].changes[]`.
4. Resolve the instance by `value.metadata.phone_number_id`. Unknown number:
   log a warning and return 200 (Meta retries otherwise; do not create data).
5. Handle:
   - `messages`: text, media, location, contacts, reaction, interactive reply,
     button reply, order, system/error.
   - `statuses`: sent, delivered, read, failed, deleted, with `errors[]`.
   - `message_template_status_update`: update local template status.
6. Deduplicate by `message.id` / `status.id` before processing. Meta retries
   webhooks; the existing `firstDelivery` logic is Baileys-specific.
7. Persist the message/status using the existing `messageRepository`, then
   normalize into the existing event payload shape and publish through the same
   webhook/RabbitMQ/NATS/WebSocket producers.
8. Return 200 quickly; heavy media download must run on a bounded worker, not in
   the webhook request.

Add `channel`/`integration` to event payloads so consumers can distinguish the
transport. Keep the existing `instanceId`/`instanceName` and HMAC signing.

### 2.4 Outbound Graph client

New `pkg/cloudapi` (or `pkg/channel/cloud`) with a testable interface:

```go
type Client interface {
    SendMessage(ctx context.Context, phoneNumberID string, payload any) (SendResponse, error)
    UploadMedia(ctx context.Context, phoneNumberID string, r io.Reader, mime string, filename string) (MediaID, error)
    GetMediaURL(ctx context.Context, mediaID string) (string, error)
    DownloadMedia(ctx context.Context, mediaURL string) (io.ReadCloser, int64, error)
    ListTemplates(ctx context.Context, wabaID string) ([]Template, error)
    CreateTemplate(ctx context.Context, wabaID string, t Template) (Template, error)
    DeleteTemplate(ctx context.Context, wabaID string, name string) error
    VerifyPhoneNumber(ctx context.Context, phoneNumberID string) (PhoneInfo, error)
}
```

Use `net/http` with a dedicated client:

- base URL `https://graph.facebook.com/{version}/` (configurable for tests)
- `Authorization: Bearer <token>`
- timeout, max response body, retry on 429/5xx with jitter
- respect `X-Business-Use-Case-Usage` and Graph error codes
- never log the token or full request body with media

Endpoint mapping for existing sends:

| WaMux send | Cloud payload |
|---|---|
| `/send/text` | `type=text`, `text.body`, optional `preview_url` |
| `/send/media` image/video/document/audio | upload media first, then `type=<media>` with `id`/`caption`/`filename` |
| `/send/location` | `type=location` |
| `/send/contact` | `type=contacts` |
| `/send/button` / `/send/list` | `type=interactive` (`button`/`list`) |
| `/send/template` (new) | `type=template` |
| `/message/react` | `type=reaction` |
| quoted reply | `context.message_id` |
| album/carousel/poll/product/status/full presence/communities/newsletters | return `unsupported_on_channel` |
| groups/calls/flows | Cloud-supported but deferred; return `not_implemented_on_channel` |

Media rules:

- Cloud requires an upload for reliable sends; support `link` only where Meta
  allows it, and prefer upload for private/local files.
- Inbound media: `GET /{media-id}` returns a short-lived URL; download with the
  same token. Enforce a max size, verify `mime_type`, and store through the
  existing media path (local/S3). Never trust the webhook’s filename.
- Media IDs expire; do not persist them as the only copy. Store the downloaded
  object, as WaMux does for Baileys.

### 2.5 Templates and the 24-hour window

Templates are mandatory for business-initiated conversations outside the
customer service window:

- Add `/template` CRUD:
  - `POST /template`
  - `GET /template`
  - `PUT /template/:id`
  - `DELETE /template/:id`
- Store local metadata (`cloud_templates`: instance, name, language, category,
  status, components, rejected reason, updated_at).
- `POST /send/template` (or `/send/template` if the generic send route is
  preferred) sends `name`, `language.code`, `components`.
- Track `last_inbound_at` per chat from webhook messages. A free-form send outside
  24 hours returns `400 cloud_window_expired` with the template path in the
  message.
- Surface template approval status; Meta rejects/limits templates outside this
  service.

### 2.6 Credentials and onboarding

MVP (manual, recommended first):

- `POST /instance/create` accepts `integration: "WHATSAPP-BUSINESS"` and a
  `cloud` object: `wabaId`, `phoneNumberId`, `businessId`, `accessToken`,
  `appId`, `appSecret`, `verifyToken`, `graphVersion`.
- All secret fields are write-only; `GET /instance/info` returns only
  `tokenSet`, `appSecretSet`, `wabaId`, `phoneNumberId`, `graphVersion`.
- `POST /instance/cloud/credentials` rotates the token/secret.
- `POST /instance/cloud/verify` calls Graph and returns the display phone number.
- On connect, subscribe the app to the WABA:
  `POST /{waba-id}/subscribed_apps`; report failures without losing the instance.
- Audit credential changes with `pkg/audit` (no secrets in detail).

Later (only if needed):

- **Embedded Signup** with Facebook Login for Business: exchange the code for a
  token, list shared WABAs/phone numbers, create the instance, subscribe the app.
  This is a separate 2–3 week project and requires a Meta app review.
- Token rotation/expiry reminders. System-user tokens can be long-lived; user
  tokens expire. Do not assume a token is permanent.

### 2.7 Data model and events

- `instances.integration` (default `WHATSAPP-BAILEYS`).
- `cloud_api_credentials` (above).
- `cloud_templates`.
- `chat_cloud_state` or a `last_inbound_at` column in `chat_states` for the
  24-hour window (additive; nullable).
- `messages` already stores `message_id`, `chat_jid`, `source`, `status`; add a
  `channel` column (default `baileys`) so Cloud and Web rows can coexist.
- Status mapping: `sent → Sent`, `delivered → Delivered`, `read → Read`,
  `failed → Failed` (new, lowest rank; must not downgrade `Read`).
- Event payload: add `channel` and `template` metadata; keep existing fields.
- Audit: Cloud credential changes, template create/delete, webhook verification
  failures (rate-limited to avoid log flooding).

### 2.8 Rate limits, retries, and errors

- Per-phone-number token bucket and bounded send concurrency. Reuse
  `pkg/httpguard`/`walimits` concepts.
- Retry only idempotent failures; Cloud send retries can duplicate messages.
  Use the existing idempotency key/operation table where possible, or
  `biz_opaque_callback_data` to correlate.
- Map Graph errors to Wamux errors:
  - `190` invalid token → `cloud_auth_failed` (no retry)
  - `131047` re-engagement/window → `cloud_window_expired`
  - `132000`/`132001` template params/not found → `cloud_template_error`
  - `131026` undeliverable → `cloud_undeliverable`
  - `80007`/`130429` rate limit → backoff + `cloud_rate_limited`
  - `368` temporarily blocked → surface with retry-after
- Log request ID/correlation ID, never the bearer token.

### 2.9 Observability

- Metrics: `cloud_requests_total{op,result}`, `cloud_request_seconds`,
  `cloud_webhook_total{result}`, `cloud_template_status{status}`,
  `cloud_window_expired_total`.
- Health: Cloud instance `Status` is the last successful Graph verification, not
  a socket; surface `graphVerifiedAt` and `lastWebhookAt`.
- Audit: credential/template changes.
- Logs: webhook signature failures at WARN with source IP, no body.

---

## 3. What is unavailable or deferred

These must return a clear typed error, not pretend to work:

- **Unavailable:** communities, newsletters, status/stories broadcasts, albums,
  carousels, polls, general chat-history/contact-list sync, QR/pairing/passkey
  sessions, full presence parity.
- **Deferred:** Groups (OBA, max 8 participants, text/media/text templates),
  Calling (WebRTC/SIP), Flows, product/catalog interactive messages. These are
  Cloud-supported; WaMux has not implemented them yet.
- **Limited:** typing indicator via mark-read, not full presence.
- **SMB-only:** `history` and `smb_message_echoes` webhooks for onboarding
  WhatsApp Business app users through a solution provider.

The API should expose `channel_capabilities` and capability notes on the
instance so clients can distinguish “not supported by Cloud” from “not
implemented in WaMux.”

---

## 4. Suggested API surface

```
POST /instance/create
  { "name": "...", "token": "...", "integration": "WHATSAPP-BUSINESS",
    "cloud": { "wabaId": "...", "phoneNumberId": "...", "accessToken": "...",
               "appSecret": "...", "verifyToken": "...", "graphVersion": "v27.0" } }

GET  /instance/info/:id          -> integration, cloud ids, tokenSet flags
POST /instance/cloud/credentials -> rotate/update
POST /instance/cloud/verify
POST /instance/connect           -> verify credentials + subscribe app
GET  /instance/status            -> stateless Cloud status

GET  /webhook/cloud              -> Meta verification
POST /webhook/cloud              -> signed events

POST /send/text | /send/media | /send/location | /send/contact
POST /send/button | /send/list | /message/react
POST /send/template
GET  /template | POST /template | PUT/DELETE /template/:id
```

Existing Baileys endpoints keep their current shape; `integration` defaults to
`WHATSAPP-BAILEYS`, so no migration breakage.

---

## 5. Testing plan

- Unit: Graph payload builders for every supported type; status/error mapping;
  24-hour-window decision; signature verification; verify-token matching;
  phone-number-id routing; template component validation.
- HTTP fake: a `httptest` Graph server for send/media/template/verify; assert
  headers, body, retries, timeouts, no token in logs.
- Webhook fixtures: text, media, status sent/delivered/read/failed, template
  update, duplicate delivery, unknown phone number, invalid signature, oversized
  body, malformed JSON.
- Integration: Cloud instance create → webhook → message row → event sink;
  Media download → S3/local; template create → send.
- Migration: additive, idempotent; existing Baileys rows untouched.
- Security: signature bypass, replay, token leakage in `/instance/all`, tenant
  A cannot read tenant B’s Cloud instance.
- Load: webhook burst, Graph 429, send concurrency, media download pool.
- Optional: Meta test number in CI is not practical; use a recorded fixture and
  a manual staging test.

---

## 6. Phases and estimate

| Phase | Scope | Estimate |
|---|---|---|
| 0. Prerequisites | Meta app, WABA, test number, system-user token, HTTPS callback, app review plan | 1–3 days |
| 1. Channel + credentials + webhook MVP | `integration` field, encrypted credentials, GET/POST `/webhook/cloud` with signature, text inbound/outbound, statuses, events, tests | **2–3 weeks** |
| 2. Media + templates + window | Upload/download, S3/local, `/template` CRUD, `/send/template`, 24h enforcement | **2–3 weeks** |
| 3. Interactive + operations | Buttons/lists/location/contacts/reactions, Graph error/rate-limit handling, metrics, manager forms, capability matrix | **1–2 weeks** |
| 4. Embedded Signup + rotation | Facebook Login for Business, shared WABA discovery, token rotation, onboarding UI | **2–3 weeks** |
| **Total to production** | Phases 1–3 + hardening | **6–8 weeks** one engineer |

This is deliberately not “port Reforged’s 1,771-line file.” Most of the work is
webhook correctness, credentials, media, templates, and error handling.

---

## 7. Operational prerequisites (non-code)

- Meta Business Portfolio + verified business.
- WhatsApp Business Account (WABA) and a phone number not already on WhatsApp
  (or migrate it).
- Meta app with WhatsApp product, app ID/secret, system-user token with
  `whatsapp_business_messaging` and `whatsapp_business_management`.
- Public HTTPS callback URL for `/webhook/cloud` (Cloud API does not call
  localhost). Use a tunnel for development.
- Template approval and category selection.
- Understanding of Meta conversation pricing; WaMux does not bill.
- Privacy/terms update: messages now transit Meta’s Cloud API.

---

## 8. Recommendation

1. **Only build this if a real deployment needs official Meta messaging.** It is
   a second product surface, not a parity checkbox.
2. Start with **Phase 1–2**: text, media, templates, signed webhooks, statuses.
   That covers most business messaging and is enough to validate demand.
3. Do **not** copy Reforged’s webhook security or global verify token. Use
   per-app credentials/verify-token hashes, per-instance Meta tokens,
   `X-Hub-Signature-256`, encrypted secrets and the existing audit log.
4. Add a `channel_capabilities` response so clients know that groups, albums,
   polls, newsletters and presence are unavailable on Cloud.
5. Keep the Baileys path untouched; `integration` defaults to it, and the channel
   adapter isolates the new transport.
6. Revisit Embedded Signup only after manual onboarding works and the Meta app is
   approved.
