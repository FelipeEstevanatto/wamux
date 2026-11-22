# WABA / WhatsApp Cloud API — implementation plan

**Status:** Phases 1–3 implemented on `develop`; Phases 4–5 deferred.
**Target:** WaMux `1.1.0` (or the next minor release).
**Companion design:** [`../spikes/waba-cloud-api.md`](../spikes/waba-cloud-api.md).
**Baseline:** WaMux `1.0.1` + 29 commits, latest migration `0017`.
**Reference implementations:** Evolution API `2.3.7`, Reforged `3.3.0`,
[`wapikit/wapi.go`](https://github.com/wapikit/wapi.go),
[`piusalfred/whatsapp`](https://github.com/piusalfred/whatsapp) and
[`jjuanrivvera/waba-cli`](https://github.com/jjuanrivvera/waba-cli). They are
references, not dependencies; none is Meta-endorsed.

> **Revision 2 changes:** BSUID/usernames are mandatory; Cloud Groups, Calling,
> Flows and typing indicators exist and must be represented accurately; sends
> must not be automatically retried; phone registration, webhook hardening,
> adaptive rate limiting and richer error handling are in scope. See
> [Sources](#sources) for the official and reference material behind these
> changes.

**Implementation note:** the repository now has the channel boundary, encrypted
credentials, signed webhook, BSUID identity, phone registration, text/media/
template/interactive sends, the 24-hour window, account/template events, media
jobs, diagnostics, manager onboarding and Cloud API tests. Embedded Signup
(Phase 4), Groups/Calling/Flows (Phase 5) and the full metrics/account-event UI
remain deferred.

---

## 1. Executive summary

Add the official WhatsApp Cloud API as a second channel, selected by
`instances.integration`:

- `WHATSAPP-BAILEYS` — existing whatsmeow/WhatsApp Web behavior; the default for
  every existing row.
- `WHATSAPP-BUSINESS` — Meta Cloud API; stateless HTTPS, signed webhooks,
  templates, media upload/download, 24-hour customer-service window.

The work is a **channel integration**, not a set of `if integration == ...`
branches in every send method. A `SendRouter` owns the selection, a
`pkg/cloud` adapter owns Graph API behavior, and Baileys keeps its current
implementation untouched.

First production slice:

1. Cloud instance creation with encrypted Meta credentials.
2. Phone registration and verification.
3. Signed `GET/POST /webhook/cloud`, including BSUID identity mapping.
4. Inbound text/status events and outbound text with operation tracking.
5. Cloud status, disconnect, delete and capability reporting.

Then media, templates, the 24-hour window, interactive messages, diagnostics and
manager onboarding.

**Estimate:** 8–10 engineer-weeks for production Phases 1–3. BSUID support,
safe send semantics, phone registration and the expanded operational surface
add roughly two weeks over the earlier 6–8 week estimate. A demo/text-only
channel is 2–3 weeks, but is not the acceptance bar.

---

## 2. Goals and non-goals

### Goals

- Support the official Meta Cloud API without changing existing Baileys
  behavior or requiring a second runtime service.
- Keep credentials encrypted at rest; never expose the Meta access token in API
  responses or logs.
- Verify every inbound webhook with `X-Hub-Signature-256`.
- Make unsupported Cloud features explicit (`unsupported_on_channel`), not
  silently broken.
- Reuse existing message persistence, event producers, audit, idempotency,
  media storage and manager patterns.
- Make the channel safe for multiple tenants on one WaMux deployment.
- Support BSUID and usernames from the first release; a phone number is no
  longer guaranteed to be present on inbound events.
- Make send outcomes explicit: never silently retry a non-idempotent message
  send after an ambiguous timeout.

### Non-goals

- Feature parity with Baileys or with Reforged.
- Implementing Cloud Groups, Calling, Flows or commerce in Phases 1–3. These
  are Cloud API products, not “unsupported features”; they are deliberately
  deferred to a separate project.
- Communities, newsletters, status/stories, albums, polls, carousels, product
  catalogs and full presence parity.
- Embedded Signup in the first release (Phase 4).
- WaMux billing or Meta conversation pricing.
- A durable outbound queue or a new Redis/MySQL/Kafka dependency.
- On-premise WhatsApp Business API (not a viable target).

---

## 3. Corrected Meta facts

These correction points matter because the earlier design assumed per-instance
verify tokens.

### 3.1 The verify token is per Meta app, not per WABA or instance

Meta configures a webhook callback at the **app** level. That callback has one
verify token. All WABAs/phone numbers under that app share it. A per-WABA verify
token is not how the Cloud API works.

Therefore:

- Store `cloud_apps` (app id, encrypted app secret, verify-token hash, Graph
  version).
- `GET /webhook/cloud` checks `hub.verify_token` against configured app rows.
- `POST /webhook/cloud` is authenticated by the **signature**, not by the
  verify token.
- Manual onboarding uses one Meta app per WaMux deployment initially. The
  schema supports multiple apps for later; the UI starts with one.

### 3.2 The 24-hour window is a Meta policy, not just a local check

A user message opens a 24-hour customer-service window. Free-form messages
outside it are rejected by Meta (`131047`). Templates are allowed outside it.
WaMux should pre-check the window for a good error, but it must also map Meta's
error because clocks and Meta state can disagree.

### 3.3 Cloud API has no general chat-history or contact-list API

WaMux can store messages received by webhook, but it cannot import a phone's
arbitrary history or contact list. The manager must not show a “sync history”
action for direct Cloud instances.

There is a `history` webhook field, but it is for onboarding WhatsApp Business
app users through a solution provider, not a general history fetch. It can
contain thousands of messages and must be handled asynchronously if WaMux ever
implements SMB onboarding. See
[Meta webhook fields](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/overview).

### 3.4 Cloud API does not accept a client-generated message ID

The Graph API returns a `wamid`. WaMux's `id` field cannot be used as the
WhatsApp message ID. For Cloud, `id` is only a WaMux/idempotency hint and may be
sent as `biz_opaque_callback_data`. The response ID is the Graph `wamid`.

### 3.5 A token is not necessarily permanent

System-user tokens can be long-lived, but user tokens expire. Store
`token_expires_at` when known, surface verification failures, and provide a
rotation endpoint. Do not build automatic refresh unless a refresh token and a
testable flow are available.

### 3.6 BSUID and usernames are mandatory

Meta now requires BSUID support for partners and directly integrated businesses
because a user may enable a username and hide their phone number. The send API
accepts a `recipient` field for a BSUID, and webhooks include `user_id`,
`from_user_id`, `parent_user_id` and `recipient_user_id`.

Consequences:

- A Cloud contact is not guaranteed to have a phone number.
- A message send is not always addressed by `number`/`to`; it may be addressed
  by `recipient`.
- The 24-hour window and chat identity must be keyed by a Cloud contact
  identity, not by `chat_jid` alone.
- Error `131062` means BSUID recipients are not supported for that message type.
  One-tap, zero-tap and copy-code authentication templates still require a
  phone number.

See [Business-scoped user IDs](https://developers.facebook.com/documentation/business-messaging/whatsapp/business-scoped-user-ids/).

### 3.7 Cloud API is broader than the original plan assumed

The Cloud API now includes products that earlier notes called unavailable:

- **Groups API** — available to Official Business Accounts (OBA). Max 8
  participants, max 10,000 groups per business number, text/media/text-template
  messages only; no interactive, auth, commerce, calling, disappearing,
  view-once, edit or delete. See
  [Groups API](https://developers.facebook.com/documentation/business-messaging/whatsapp/groups).
- **Calling API** — WebRTC/SIP calling, call permissions, call settings and
  `calls` webhooks. It is a separate project. See
  [Cloud API Calling](https://developers.facebook.com/documentation/business-messaging/whatsapp/calling).
- **Flows** — interactive Flows and `nfm_reply` messages. Deliberately
  deferred. See
  [WhatsApp Flows](https://developers.facebook.com/documentation/business-messaging/whatsapp/flows).
- **Typing indicator** — marking a message as read can include a
  `typing_indicator`; this is not full presence parity.

The capability matrix must say “Cloud supports this, WaMux has not implemented
it” where applicable, not “Cloud does not support it.”

### 3.8 Phone registration is part of onboarding

A verified phone number may still fail to send with `133010` until it is
registered for Cloud API. The registration call requires a six-digit PIN:

```text
POST /{phone-number-id}/register
{"messaging_product":"whatsapp","pin":"123456"}
```

Add registration status to the instance lifecycle and never store the PIN.
Registration is limited to 10 requests per business number in a 72-hour window;
map `133016` and stop retrying. See
[phone number registration](https://developers.facebook.com/documentation/business-messaging/whatsapp/business-phone-numbers/registration).

---

## 4. Scope matrix

| Capability | Phase 1 | Phase 2 | Phase 3 | Notes |
|---|---:|---:|---:|---|
| Cloud instance + credentials | ✅ | | | encrypted, write-only |
| Phone verification + registration | ✅ | | | `register` with six-digit PIN |
| Signed webhook GET/POST | ✅ | | | app-level verify token |
| BSUID/username identity | ✅ | | | phone is optional; `recipient` send field |
| Inbound text | ✅ | | | persisted + event |
| Inbound statuses | ✅ | | | sent/delivered/read/failed |
| Outbound text | ✅ | | | `wamid`, safe send outcomes |
| Instance status/disconnect/delete | ✅ | | | no QR/socket |
| Cloud diagnostics | ✅ basic | ✅ | ✅ full | token, registration, subscription, webhook |
| Cloud media upload | | ✅ | | local/multipart and URL |
| Cloud media download | | ✅ | | queued, S3/local |
| Templates CRUD/send | | ✅ | | Graph + local cache |
| 24-hour window | | ✅ | | BSUID-aware; pre-check + Meta error |
| Buttons/lists | | | ✅ | reply/list subset; granular capability |
| CTA URL/Flows/product/catalog | | | Deferred | Cloud supports; separate project |
| Location/contact/reaction | | | ✅ | Cloud payload mapping |
| Sticker | | | ✅ | if Meta account supports it |
| Typing indicator | | | ✅ | mark-read + `typing_indicator`; not presence |
| Rate-limit/error mapping | ✅ basic | ✅ | ✅ full | adaptive limiter + typed hints |
| Capability discovery | ✅ | ✅ | ✅ | API + manager |
| Manager onboarding | | | ✅ | create/settings/templates/diagnostics |
| Embedded Signup | | | | Phase 4 |
| Groups API | | | | Cloud-supported (OBA), deferred |
| Calling API | | | | Cloud-supported, separate project |
| Flows | | | | Cloud-supported, deferred |
| Albums/polls/carousels/status/communities/newsletters | ❌ | ❌ | ❌ | no Cloud equivalent |
| General chat history/contact sync | ❌ | ❌ | ❌ | no API; SMB `history` webhook is separate |
| Full presence parity | ❌ | ❌ | ❌ | typing indicator only |

---

## 5. Architecture

### 5.1 Component diagram

```
                         ┌─────────────────────────┐
  API client ───────────▶│  /send/* + /template/*  │
                         └────────────┬────────────┘
                                      │
                              ┌───────▼────────┐
                              │  SendRouter    │  chooses by instance.Integration
                              └───┬────────┬───┘
                                  │        │
                    ┌─────────────▼──┐  ┌──▼─────────────────┐
                    │ Baileys sender │  │ Cloud sender       │
                    │ (existing)     │  │ (pkg/cloud)        │
                    └───────┬────────┘  └──┬─────────┬───────┘
                            │              │         │
                      WhatsApp Web     Graph API   Webhook
                                      (HTTPS)     verifier
                                                     ▲
                                                     │
                                           Meta Cloud API webhook
```

### 5.2 Channel model

Add `pkg/channel` for shared, WhatsApp-neutral types:

```go
package channel

type Kind string

const (
    KindWhatsAppWeb Kind = "WHATSAPP-BAILEYS"
    KindCloud       Kind = "WHATSAPP-BUSINESS"
)

type Capabilities struct {
    Text            bool `json:"text"`
    Media           bool `json:"media"`
    Templates       bool `json:"templates"`
    Interactive     bool `json:"interactive"`
    Location        bool `json:"location"`
    Contacts        bool `json:"contacts"`
    Reactions       bool `json:"reactions"`
    BSUID           bool `json:"bsuid"`
    TypingIndicator bool `json:"typingIndicator"`
    Groups          bool `json:"groups"`
    Calls           bool `json:"calls"`
    Flows           bool `json:"flows"`
    Albums          bool `json:"albums"`
    Polls           bool `json:"polls"`
    Newsletters     bool `json:"newsletters"`
    Status          bool `json:"status"`
    Presence        bool `json:"presence"`
    History         bool `json:"history"`
}

var (
    ErrUnsupported     = errors.New("unsupported_on_channel")
    ErrNotImplemented  = errors.New("not_implemented_on_channel")
)

type UnsupportedError struct {
    Channel Kind
    Op      string
}

func (e *UnsupportedError) Error() string { ... }
func (e *UnsupportedError) Unwrap() error { return ErrUnsupported }

// NotImplementedError distinguishes "Cloud has this but WaMux has not built it"
// (Groups, Calling, Flows) from "Cloud has no equivalent" (albums, polls).
type NotImplementedError struct {
    Channel Kind
    Op      string
}

func (e *NotImplementedError) Error() string { ... }
func (e *NotImplementedError) Unwrap() error { return ErrNotImplemented }
```

Neutral DTOs (minimal shape; add fields as operations are implemented):

```go
type TextMessage struct {
    To            string
    Recipient     string // BSUID or parent BSUID
    RecipientType string // individual (default) or group
    Body          string
    PreviewURL    bool
    ReplyTo       string
    OperationID   string
}

type MediaMessage struct {
    To            string
    Recipient     string
    RecipientType string
    Kind          string // image/video/audio/document/sticker
    URL           string
    Data          []byte
    Filename      string
    MIME          string
    Caption       string
    ReplyTo       string
    OperationID   string
}

type Result struct {
    ID          string
    Timestamp   time.Time
    Channel     Kind
    Recipient   string // BSUID actually used, if any
    WAID        string // phone actually used, if any
    OperationID string
    Raw         map[string]any // optional, never serialized directly
}
```

Cloud-specific DTOs live in `pkg/channel` or `pkg/cloud`; they must not import
`pkg/sendMessage/service` (that package will import the cloud adapter through
the router). The router maps the existing public structs to these DTOs.

### 5.3 Send routing

Do not modify every Baileys send implementation with Cloud branches. Add
`pkg/sendMessage/service/router.go` implementing the existing `SendService`
interface:

```go
// CloudSender is implemented by pkg/cloud. It accepts channel-neutral DTOs so
// pkg/cloud never imports pkg/sendMessage/service.
type CloudSender interface {
    SendText(ctx context.Context, inst *instance_model.Instance, msg channel.TextMessage) (channel.Result, error)
    SendMedia(ctx context.Context, inst *instance_model.Instance, msg channel.MediaMessage) (channel.Result, error)
    // ... one method per supported Cloud operation
}

type Router struct {
    baileys SendService
    cloud   CloudSender
}

func (r *Router) SendText(data *TextStruct, inst *instance_model.Instance) (*MessageSendStruct, error) {
    if inst.Integration == channel.KindCloud {
        res, err := r.cloud.SendText(context.Background(), inst, toCloudText(data))
        return toMessageSend(res), err
    }
    return r.baileys.SendText(data, inst)
}
```

Rules:

- A single branch per public operation, centralized in the router.
- Cloud-unsupported operations return `channel.UnsupportedError` without
  touching Baileys or the network.
- Baileys delegates to the current implementation unchanged.
- The handler and routes keep their current shape.
- The router maps `TextStruct`/`MediaStruct`/etc. to neutral `channel` DTOs and
  maps `channel.Result` back to the compatibility response.
- For Cloud, the router fills a compatibility `MessageSendStruct`:
  - `Info.ID` = Graph `wamid`
  - `Info.Chat` = the compatibility chat key only; for a BSUID-only recipient,
    do not fabricate a phone JID. Put the BSUID in `cloud.recipient` and
    `contactId` instead.
  - `Info.Sender` = instance JID (or the Cloud phone number)
  - `Info.Timestamp` = send time
  - `Message` stays nil for non-media, or a minimal map for media metadata
  - add `Channel`, `contactId` and `Cloud` fields to the response envelope
    (additive)

If changing `MessageSendStruct` is too invasive, add a wrapper response with
`channel` and `externalId` while preserving `Info.ID`. Do not expose the raw
Graph response.

### 5.4 Instance lifecycle

`instances.Create`, `Connect`, `Status`, `Delete`, `Logout`, `Qr` and `Pair`
currently assume whatsmeow. They need channel checks, but not a wholesale
rewrite:

| Operation | Baileys | Cloud |
|---|---|---|
| Create | current | insert instance + encrypted credentials in one transaction |
| Connect | start client/QR | verify token/phone, subscribe app, check registration; no client |
| Register | n/a | six-digit PIN; `registered` status; no PIN stored |
| Status | socket state | cached verification/registration + webhook state |
| Disconnect | stop client | mark disconnected; keep credentials |
| Delete | current cleanup | delete child rows + media; no device store |
| Logout | current | `422 unsupported_on_channel` (rotate/delete instead) |
| QR/Pair | current | `422 unsupported_on_channel` |
| Force reconnect | current | re-run Cloud verification (optional alias) |

Cloud is stateless. Do not create a `whatsmeow.Client` for a Cloud instance. Do
not acquire an ownership lease for it.

### 5.5 Inbound/outbound flow

```
Meta ──POST /webhook/cloud──▶ signature ──▶ route by phone_number_id
                                                │
                                  ┌─────────────▼──────────────┐
                                  │ cloud_webhook_events       │ dedupe
                                  └─────────────┬──────────────┘
                                                │
                        ┌───────────────────────┼───────────────────────┐
                        ▼                       ▼                       ▼
                  contact identity        status event             template/account
                  (BSUID/phone)           cloud_status_events      cloud_templates
                        │                       │                       │
                  message row + media      account events          EventSink
                  job queue                                             │
                        └───────────────────────┴───────────▶ (Message/Receipt)
```

Outbound:

```
/send/* ──▶ SendRouter ──▶ CloudSender ──▶ Graph /messages
                                      ──▶ send_operations (pending/accepted/unknown)
                                      ──▶ persist row (channel=cloud, Sent)
                                      ──▶ publish SendMessage
```

---

## 6. Data model and migrations

### 6.1 Design rules

- All changes are additive. Do not rewrite `instances` or `messages`.
- Default every existing instance to `WHATSAPP-BAILEYS`.
- Cloud credentials live in a separate table, never in `instances.token`.
- `instances.token` remains the WaMux API token and is never sent to Meta.
- Use the existing `tokencrypt` codec for secrets. If `DATA_ENCRYPTION_KEY` is
  unavailable, Cloud creation must be refused rather than storing plaintext.
- New child tables use `ON DELETE CASCADE`; the instance delete service also
  removes child rows explicitly so SQLite builds without foreign-key
  enforcement do not leave orphans.

### 6.2 New/changed columns

`instances`:

```sql
ALTER TABLE instances ADD COLUMN integration TEXT NOT NULL DEFAULT 'WHATSAPP-BAILEYS';
CREATE INDEX idx_instances_integration ON instances (integration);
```

`messages`:

```sql
ALTER TABLE messages ADD COLUMN channel TEXT NOT NULL DEFAULT 'baileys';
ALTER TABLE messages ADD COLUMN contact_id TEXT;
ALTER TABLE messages ADD COLUMN sender_user_id TEXT;
ALTER TABLE messages ADD COLUMN recipient_user_id TEXT;
CREATE INDEX idx_messages_instance_channel ON messages (instance_id, channel);
CREATE INDEX idx_messages_instance_contact ON messages (instance_id, contact_id, "timestamp" DESC);
```

Use lowercase `baileys` / `cloud` for the row-level channel. The instance
integration remains the canonical `WHATSAPP-BAILEYS` / `WHATSAPP-BUSINESS`
value.

`contact_id` is the Cloud contact key: `bsuid:<BSUID>` when a BSUID is present,
otherwise `phone:<E.164 digits>`. `chat_jid` remains the manager-facing chat key;
Cloud read paths must not parse it as a phone JID when the contact is BSUID-only.

### 6.2a `cloud_contacts`

```sql
CREATE TABLE IF NOT EXISTS cloud_contacts (
    instance_id   UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    contact_id    TEXT NOT NULL,          -- bsuid:<value> or phone:<digits>
    bsuid         TEXT,
    parent_bsuid  TEXT,
    wa_id         TEXT,                   -- phone number, when Meta provides it
    username      TEXT,
    display_name  TEXT,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (instance_id, contact_id)
);

CREATE INDEX IF NOT EXISTS idx_cloud_contacts_bsuid
    ON cloud_contacts (instance_id, bsuid);
CREATE INDEX IF NOT EXISTS idx_cloud_contacts_wa_id
    ON cloud_contacts (instance_id, wa_id);
```

The identity row is updated on every message/status webhook. BSUID is the
primary identity; phone is optional. Handle customer identity/number-change
events by upserting the new mapping and preserving the conversation history.

### 6.3 `cloud_apps`

```sql
CREATE TABLE IF NOT EXISTS cloud_apps (
    app_id            TEXT PRIMARY KEY,
    app_secret_enc    TEXT NOT NULL,
    verify_token_hash TEXT NOT NULL,
    graph_version     TEXT NOT NULL DEFAULT 'v27.0',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cloud_apps_verify_token
    ON cloud_apps (verify_token_hash);
```

- `app_secret_enc` = AES-256-GCM of the Meta app secret.
- `verify_token_hash` = HMAC-SHA256 of the webhook verify token under the
  server key. The plaintext token is only needed to configure the Meta
  dashboard; it is write-only in the API.
- `graph_version` is per app so a version upgrade is explicit.

### 6.4 `cloud_api_credentials`

```sql
CREATE TABLE IF NOT EXISTS cloud_api_credentials (
    instance_id       UUID PRIMARY KEY REFERENCES instances(id) ON DELETE CASCADE,
    app_id            TEXT NOT NULL REFERENCES cloud_apps(app_id),
    waba_id           TEXT NOT NULL,
    phone_number_id   TEXT NOT NULL,
    business_id       TEXT,
    display_phone_number TEXT,
    verified_name     TEXT,
    access_token_enc  TEXT NOT NULL,
    token_expires_at  TIMESTAMPTZ,
    last_verified_at  TIMESTAMPTZ,
    last_verify_error TEXT,
    registered        BOOLEAN NOT NULL DEFAULT FALSE,
    registered_at     TIMESTAMPTZ,
    registration_status TEXT,
    last_registration_error TEXT,
    quality_rating    TEXT,
    messaging_limit   TEXT,
    enforce_window    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_cloud_credentials_phone
    ON cloud_api_credentials (phone_number_id);
CREATE INDEX IF NOT EXISTS idx_cloud_credentials_waba
    ON cloud_api_credentials (waba_id);
```

`phone_number_id` is the webhook routing key and must be globally unique.

### 6.5 `cloud_templates`

```sql
CREATE TABLE IF NOT EXISTS cloud_templates (
    id             UUID PRIMARY KEY,
    instance_id    UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    template_id    TEXT,
    name           TEXT NOT NULL,
    language       TEXT NOT NULL,
    category       TEXT,
    status         TEXT,
    components     JSONB NOT NULL DEFAULT '[]',
    rejected_reason TEXT,
    quality_score  JSONB,
    last_synced_at TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (instance_id, name, language)
);

CREATE INDEX IF NOT EXISTS idx_cloud_templates_instance
    ON cloud_templates (instance_id, status);
```

SQLite uses `TEXT` instead of `JSONB`; GORM tags can still use `type:jsonb` as
the message model does, but the migration must pass on both drivers.

### 6.6 `cloud_chat_windows`

```sql
CREATE TABLE IF NOT EXISTS cloud_chat_windows (
    instance_id     UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    contact_id      TEXT NOT NULL,          -- bsuid:<value> or phone:<digits>
    chat_jid        TEXT,                   -- display/compatibility key
    last_inbound_at TIMESTAMPTZ NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    last_message_id TEXT,
    PRIMARY KEY (instance_id, contact_id)
);

CREATE INDEX IF NOT EXISTS idx_cloud_windows_expiry
    ON cloud_chat_windows (expires_at);
```

Use the Meta message timestamp, not the webhook receipt time, so a delayed
webhook does not extend the window incorrectly. Key by `contact_id`, because a
BSUID user may have no phone number and a phone-number change must not split the
window. Keep rows; a later inbound upsert refreshes the window. Cleanup is
optional.

### 6.7 `cloud_webhook_events` (dedupe/replay)

```sql
CREATE TABLE IF NOT EXISTS cloud_webhook_events (
    event_key    TEXT PRIMARY KEY,
    instance_id  UUID,
    event_type   TEXT NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    attempts     INT NOT NULL DEFAULT 0,
    last_error   TEXT
);

CREATE INDEX IF NOT EXISTS idx_cloud_webhook_events_pending
    ON cloud_webhook_events (processed_at, received_at);
```

`event_key` is `sha256(instance_id || ":" || event_type || ":" || meta_id ||
":" || status/event timestamp)`.

Processing rule:

- `INSERT ... ON CONFLICT DO NOTHING` claims the event.
- If already processed, return 200.
- If claimed, process; set `processed_at`.
- On a processing error, delete the claim and return 500 so Meta can retry.
- Retain 30 days; a cleanup job removes old rows.

This table prevents duplicate `Message`/`Receipt` events. Message/status
persistence itself is also idempotent, so a crash between persistence and claim
completion is safe.

### 6.8 `cloud_media_jobs`

```sql
CREATE TABLE IF NOT EXISTS cloud_media_jobs (
    id             UUID PRIMARY KEY,
    instance_id    UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    message_id     TEXT NOT NULL,
    media_id       TEXT NOT NULL,
    mime_type      TEXT,
    filename       TEXT,
    declared_size  BIGINT,
    status         TEXT NOT NULL DEFAULT 'pending',
    attempts       INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error     TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (instance_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_cloud_media_jobs_ready
    ON cloud_media_jobs (status, next_attempt_at);
```

Statuses: `pending`, `downloading`, `stored`, `failed`. A worker claims rows
with `FOR UPDATE SKIP LOCKED` on Postgres (single-process loop on SQLite),
downloads, stores through the existing media storage, and updates the message
row when done. Retry with exponential backoff; cap attempts at 5.

### 6.9 `cloud_status_events`

```sql
CREATE TABLE IF NOT EXISTS cloud_status_events (
    id            UUID PRIMARY KEY,
    instance_id   UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    message_id    TEXT NOT NULL,
    status        TEXT NOT NULL,
    recipient_user_id TEXT,
    recipient_id  TEXT,
    error_code    TEXT,
    error_title   TEXT,
    error_message TEXT,
    occurred_at   TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (instance_id, message_id, status, occurred_at)
);

CREATE INDEX IF NOT EXISTS idx_cloud_status_message
    ON cloud_status_events (instance_id, message_id, occurred_at DESC);
```

`messages.status` remains the current state. `cloud_status_events` keeps the
history and Meta error detail for the manager and support.

Add `recipient_user_id TEXT` and `recipient_id TEXT` to this table so a failed
status can still be tied to a BSUID when the phone number is absent.

### 6.9a `cloud_account_events`

```sql
CREATE TABLE IF NOT EXISTS cloud_account_events (
    id          UUID PRIMARY KEY,
    instance_id UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    field       TEXT NOT NULL,             -- account_alerts, security, ...
    event_type  TEXT,
    payload     JSONB NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cloud_account_events_instance
    ON cloud_account_events (instance_id, received_at DESC);
```

Store quality, account-review, capability, security, username and template
quality/category/component updates here. Surface the latest state in the
manager; do not require a separate service.

### 6.9b `cloud_user_preferences`

```sql
CREATE TABLE IF NOT EXISTS cloud_user_preferences (
    instance_id        UUID NOT NULL REFERENCES instances(id) ON DELETE CASCADE,
    contact_id         TEXT NOT NULL,
    marketing_opt_out  BOOLEAN NOT NULL DEFAULT FALSE,
    source_event_at    TIMESTAMPTZ,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (instance_id, contact_id)
);
```

Meta can send `user_preferences` changes. WaMux should record them even though
the tenant owns the final opt-in/opt-out policy.

### 6.10 Status ordering

Extend the existing `statusRankSQL` in
`pkg/message/repository/message_repository.go`:

```go
array_position(ARRAY['Failed','Delivered','Read','Played']::text[], status)
```

Resulting order:

- `Sent`/`Received` = 0
- `Failed` = 1
- `Delivered` = 2
- `Read` = 3
- `Played` = 4

So `Failed` overrides `Sent`, but an out-of-order `Delivered` overrides
`Failed`; `Read` is never downgraded. Update both `statusRankSQL` and the
Go-side `statusRank` switch (`Failed=1`, `Delivered=2`, `Read=3`, `Played=4`)
and add tests for all transitions.

### 6.11 Migration list

Append only, in order:

| Migration | Contents |
|---|---|
| `0018_instances_integration` | `instances.integration`, default, index |
| `0019_cloud_apps` | app credentials + verify-token hash |
| `0020_cloud_api_credentials` | per-instance WABA/phone/token + registration status |
| `0021_cloud_contacts` | BSUID/username/phone identity mapping |
| `0022_cloud_templates` | template cache/lifecycle |
| `0023_cloud_chat_windows` | BSUID-aware 24-hour window state |
| `0024_cloud_webhook_events` | dedupe/replay claims |
| `0025_cloud_media_jobs` | durable media download queue |
| `0026_messages_cloud_identity` | message `channel`, `contact_id`, BSUID columns |
| `0027_cloud_status_events` | status/error history |
| `0028_cloud_account_events` | account/template/security/quality events |
| `0029_cloud_user_preferences` | marketing preference state |

Postgres steps must use `IF NOT EXISTS` where supported. SQLite cannot add a
column with `IF NOT EXISTS`; AutoMigrate runs first, so SQLite steps should only
backfill and create indexes/tables that AutoMigrate did not create. Test both
drivers.

### 6.12 Retention

- `cloud_webhook_events`: 30 days (Meta retries non-200 deliveries for up to
  7 days, so this comfortably covers retries).
- `cloud_status_events`: 90 days.
- `cloud_account_events`: 90 days.
- `cloud_user_preferences`: keep while the contact exists; it is state, not an
  event log.
- `cloud_media_jobs` failed rows: 30 days, or keep until manually retried.
- `cloud_templates`: keep while the instance exists; stale rows are harmless.
- Add these to the existing cleanup/retention path or a small `pkg/cloud/cleanup`
  worker. Do not create a separate daemon.

---

## 7. Configuration

Add to `pkg/config/config.go` and the environment reference:

| Variable | Default | Purpose |
|---|---|---|
| `CLOUD_API_ENABLED` | `false` | Master switch for routes/creation; phase rollout |
| `CLOUD_API_GRAPH_BASE_URL` | `https://graph.facebook.com` | Test override; production fixed |
| `CLOUD_API_GRAPH_VERSION` | `v27.0` | Default for app rows; confirm the supported version at implementation time |
| `CLOUD_API_APP_ID` | empty | Initial Meta app id |
| `CLOUD_API_APP_SECRET` | empty | Initial app secret (encrypted at boot/seed) |
| `CLOUD_API_VERIFY_TOKEN` | empty | Initial webhook verify token |
| `CLOUD_API_HTTP_TIMEOUT_SECONDS` | `30` | Graph JSON request timeout |
| `CLOUD_API_RESPONSE_HEADER_TIMEOUT_SECONDS` | `60` | Time to first response header |
| `CLOUD_API_MEDIA_TIMEOUT_SECONDS` | `120` | Media transfer timeout |
| `CLOUD_API_MAX_WEBHOOK_BYTES` | `4194304` | Webhook body cap; Meta documents 3 MB |
| `CLOUD_API_MAX_RESPONSE_BYTES` | `10485760` | Bounded JSON response body |
| `CLOUD_API_MAX_RESPONSE_HEADER_BYTES` | `1048576` | Max response header |
| `CLOUD_API_MAX_MEDIA_BYTES` | `104857600` | Inbound media cap |
| `CLOUD_API_MEDIA_WORKERS` | `2` | Media download worker count |
| `CLOUD_API_WINDOW_HOURS` | `24` | Window length; Meta policy default |
| `CLOUD_API_ENFORCE_WINDOW` | `true` | Default for new credentials |
| `CLOUD_API_RPS` | conservative value | Adaptive limiter floor |
| `CLOUD_API_RETRY_MAX` | `3` | Retries for idempotent requests only |

If `CLOUD_API_APP_ID`/`APP_SECRET`/`VERIFY_TOKEN` are set, seed/update the
single `cloud_apps` row at boot. If they are absent, the manager/API can create
an app row. Never log any of these values.

`DATA_ENCRYPTION_KEY` is mandatory for Cloud. If it is missing, `Create` with
`integration=WHATSAPP-BUSINESS` fails with a configuration error.

---

## 8. API contracts

All cloud routes are additive. Existing routes keep their request/response
shape unless explicitly noted.

### 8.0 Manage Meta apps (admin)

`POST /cloud/apps` (global admin key)

```json
{
  "appId": "1234567890",
  "appSecret": "EAAG...",
  "verifyToken": "a-long-random-value",
  "graphVersion": "v27.0"
}
```

- Write-only `appSecret` and `verifyToken`.
- `GET /cloud/apps` returns `appId`, `graphVersion`, `verifyTokenSet: true`,
  `appSecretSet: true`.
- `PUT /cloud/apps/:appId` rotates the secret/verify token.
- `DELETE /cloud/apps/:appId` is rejected while any instance references it.
- Audit every change without secrets.
- Setting `CLOUD_API_APP_ID`/`CLOUD_API_APP_SECRET`/`CLOUD_API_VERIFY_TOKEN`
  seeds the same row at boot; the API is for multi-app or post-boot changes.

### 8.1 Create a Cloud instance

`POST /instance/create`

```json
{
  "name": "acme-support",
  "token": "wamux-api-token",
  "websocketEnable": "enabled",
  "integration": "WHATSAPP-BUSINESS",
  "cloud": {
    "appId": "1234567890",
    "wabaId": "9876543210",
    "phoneNumberId": "1122334455",
    "businessId": "5566778899",
    "accessToken": "EAAG...",
    "displayPhoneNumber": "+5511999999999",
    "enforceWindow": true
  }
}
```

- `integration` defaults to `WHATSAPP-BAILEYS`.
- `cloud` is optional; without it the instance is created unconfigured and
  `Connect` returns a clear “credentials missing” error. This supports a
  two-step UI.
- If `cloud` is present, create the instance and credentials in one transaction.
- Never echo `accessToken`.
- Response adds `integration` and a `cloud` summary with `tokenSet: true`.

### 8.2 Rotate/update credentials

`PUT /instance/cloud/credentials` (instance token auth)

```json
{
  "appId": "1234567890",
  "wabaId": "9876543210",
  "phoneNumberId": "1122334455",
  "businessId": "5566778899",
  "accessToken": "EAAG...",
  "enforceWindow": true
}
```

- All fields optional; omitted fields stay unchanged.
- `accessToken` write-only.
- Changing `phoneNumberId` must be rejected if another instance owns it.
- Audit the change without secret values.
- Invalidate cached verification and status.

### 8.3 Verify/subscribe

`POST /instance/cloud/verify`

```json
{
  "connected": true,
  "subscribed": true,
  "registered": true,
  "displayPhoneNumber": "+5511999999999",
  "verifiedName": "Acme Support",
  "qualityRating": "GREEN",
  "messagingLimit": "TIER_1K",
  "checkedAt": "2026-11-22T14:00:00Z"
}
```

`POST /instance/connect` for Cloud performs verification,
`POST /{waba-id}/subscribed_apps`, and checks phone registration. It returns the
normal instance object plus `cloud`. If subscription or registration fails,
keep credentials, set `connected` only after verification, and return a clear
error with `subscription`/`registered` flags.

`POST /instance/cloud/register`

```json
{
  "pin": "123456"
}
```

- Calls `POST /{phone-number-id}/register` with
  `{"messaging_product":"whatsapp","pin":"..."}`.
- Six-digit PIN; never store the PIN.
- Treat an already-registered response as success.
- Map `133010` to a “register this number” action.

`GET /instance/cloud/diagnostics`

```json
{
  "tokenValid": true,
  "appSecretSet": true,
  "phoneRegistered": true,
  "subscribed": true,
  "lastWebhookAt": "2026-11-22T14:05:00Z",
  "lastGraphError": null,
  "rateLimitState": {"effectiveRPS": 8.5},
  "templates": {"approved": 12, "pending": 1, "rejected": 0}
}
```

Diagnostics are read-only and must not call Graph on every poll unless the
caller explicitly requests a refresh.

### 8.4 Status

`GET /instance/status`:

```json
{
  "connected": true,
  "loggedIn": true,
  "channel": "WHATSAPP-BUSINESS",
  "cloud": {
    "displayPhoneNumber": "+5511999999999",
    "verifiedName": "Acme Support",
    "lastVerifiedAt": "2026-11-22T14:00:00Z",
    "lastWebhookAt": "2026-11-22T14:05:00Z",
    "subscribed": true,
    "registered": true,
    "windowHours": 24,
    "capabilities": {
      "text": true,
      "media": true,
      "templates": true,
      "interactive": true,
      "bsuid": true,
      "typingIndicator": true,
      "groups": false,
      "calls": false,
      "flows": false,
      "albums": false,
      "polls": false,
      "newsletters": false,
      "status": false,
      "presence": false,
      "history": false
    },
    "capabilityNotes": {
      "groups": "Cloud Groups API exists for Official Business Accounts; not implemented in WaMux",
      "calls": "Cloud Calling API exists; separate project",
      "flows": "Cloud Flows exist; deferred",
      "history": "No general history API; the SMB history webhook is separate"
    }
  }
}
```

The status endpoint must not call Graph on every poll. Use the cached
verification result; `POST /instance/cloud/verify` refreshes it.
`GET /instance/info/:id` returns the same `cloud` summary (without secrets) so
the manager can render settings even when the instance is offline.

### 8.5 Send text

`POST /send/text` (existing shape; Cloud semantics below)

```json
{
  "number": "5511999999999",
  "recipient": "US.13491208655302741918",
  "recipientType": "individual",
  "text": "Hello from WaMux",
  "quoted": {"messageId": "wamid..."}
}
```

- `number` is normalized to digits; Cloud `to` is E.164 without `+`.
- `recipient` is a BSUID or parent BSUID and maps to Cloud `recipient`.
- If both `number` and `recipient` are present, `to` takes precedence, matching
  Meta.
- `recipientType` is `individual` (default) or `group` when Groups support is
  implemented.
- `quoted.messageId` maps to `context.message_id`; it must be a `wamid`.
- `mentionAll` / `mentionedJid` are unsupported on Cloud: return
  `422 unsupported_on_channel` rather than silently dropping mentions.
- `delay` is honored as a bounded local sleep; Cloud has no presence, but
  mark-read can optionally send a `typing_indicator`.
- `linkPreview` maps to `preview_url` only; rich link metadata is ignored.

Response (additive fields; `data.Info.ID` remains for compatibility):

```json
{
  "message": "success",
  "data": {
    "Info": {
      "ID": "wamid.HBgM...",
      "Timestamp": "2026-11-22T14:00:00Z"
    },
    "channel": "cloud",
    "cloud": {
      "wamid": "wamid.HBgM...",
      "recipient": "US.13491208655302741918",
      "waId": "5511999999999"
    }
  }
}
```

If the Graph request times out, return `202` with an operation record only
when the operation state is `unknown`; otherwise return the typed error. Do not
blindly retry the send. See §9.2.

### 8.6 Send media

`POST /send/media` (JSON with `url` or multipart with `file`)

- Cloud supports `image`, `video`, `audio`, `document`, and (where enabled)
  `sticker`.
- Upload local/multipart bytes to `POST /{phone-number-id}/media`, then send by
  media id. For a URL, download through the existing SSRF guard and upload; do
  not rely on Meta fetching private URLs.
- Captions apply to image/video/document; audio has no caption.
- `filename` applies to documents.
- `gifPlayback`, view-once, PTT-specific behavior and forwarding scores are
  best-effort or unsupported; document the mapping.

### 8.7 Send template

`POST /send/template` (new)

```json
{
  "number": "5511999999999",
  "recipient": "US.13491208655302741918",
  "recipientType": "individual",
  "name": "order_update",
  "language": "en_US",
  "components": [
    {
      "type": "body",
      "parameters": [{"type": "text", "text": "12345"}]
    }
  ]
}
```

- `name` + `language` identify the approved template.
- `components` is passed through after structural validation.
- Parameter types supported by Meta include text, currency, date-time, image,
  document, video, location, limited-time-offer and product. Unknown types are
  rejected with a clear validation error.
- If the local cache says the template is rejected/disabled, return
  `422 cloud_template_error` before calling Graph.
- Template sends bypass the 24-hour window.
- BSUID recipients cannot use one-tap, zero-tap or copy-code authentication
  templates (`131062`); require a phone number for those.

### 8.8 Templates

`GET /template` — list cached templates for the authenticated instance.
`POST /template` — create through Graph and cache.
`PUT /template/:id` — update components/category through Graph and cache.
`DELETE /template/:id` — delete through Graph and remove cache.
`POST /template/sync` — refresh from
`GET /{waba-id}/message_templates`.

Create body:

```json
{
  "name": "order_update",
  "language": "en_US",
  "category": "UTILITY",
  "components": []
}
```

Template status is Meta-owned: `APPROVED`, `PENDING`, `REJECTED`, `PAUSED`,
`DISABLED`, etc. Surface `rejected_reason`, `quality_score`, category and
component updates.

Template sync:

- Use `paging.cursors.after`; do not follow an absolute `paging.next` URL
  blindly. Rebuild the request against the configured Graph host.
- Store `template_id`, name, language, category, status, components, quality
  score and rejection reason.
- Handle `message_template_status_update`,
  `message_template_quality_update`, `template_category_update` and
  `message_template_components_update`.
- Template changes are audited without message content.

### 8.9 Webhook

`GET /webhook/cloud?hub.mode=subscribe&hub.verify_token=...&hub.challenge=...`

- Look up `verify_token_hash`; if matched, return `hub.challenge` as plain text.
- Otherwise `403`.
- No auth middleware; this is Meta's callback.
- Support `GET /webhook/cloud/:appId` for multi-app deployments; the app ID
  selects the verify-token hash/secret without being a secret itself.

`POST /webhook/cloud`

- Require `X-Hub-Signature-256: sha256=<hex>`.
- Verify HMAC-SHA256 over the raw body using the app secret selected by the
  event's WABA/phone number. If the WABA is unknown, try all configured app
  secrets (bounded); otherwise `401`.
- For multiple Meta apps, support `/webhook/cloud/:appId` and configure that
  URL in each app. The path selects the app secret before parsing, instead of
  trying every secret.
- Body cap `CLOUD_API_MAX_WEBHOOK_BYTES` (4 MiB default); oversized `413`.
- Valid signed event: return 200 with `{"status":"ok"}`.
- Invalid/missing signature: `401`.
- Unknown phone number but valid signature: 200 and ignore (avoid Meta retry
  storms).
- One malformed message/change: log, increment a metric, and continue with the
  rest. Do not fail the whole payload and trigger Meta retries for events that
  were already processed.
- Unknown webhook field: acknowledge with 200; store/log if useful.
- Panic in parsing/handling: recover, return 500, log stack; never crash the
  process.
- Infrastructure failure (DB unavailable): 500 after releasing the dedupe
  claim.
- Meta retries non-200 responses for up to 7 days; dedupe/data retention must
  cover that window.

### 8.10 Error contract

Keep the existing `{"error": "..."}` shape and add machine-readable fields:

```json
{
  "error": "cloud_window_expired",
  "message": "The 24-hour customer service window is closed; send an approved template instead.",
  "code": "cloud_window_expired",
  "details": {
    "windowExpiresAt": "2026-11-22T13:00:00Z",
    "templateRequired": true
  }
}
```

Status codes:

| Condition | Status | Code |
|---|---|---:|
| Validation | 400 | `invalid_request` |
| No Cloud equivalent | 422 | `unsupported_on_channel` |
| Cloud-supported, WaMux not implemented | 501 | `not_implemented_on_channel` |
| Window closed | 422 | `cloud_window_expired` |
| Template rejected/not found | 422 | `cloud_template_error` |
| Undeliverable | 422 | `cloud_undeliverable` |
| BSUID unsupported for message type | 422 | `cloud_bsuid_unsupported` |
| Phone not registered | 424 | `cloud_phone_not_registered` |
| Send outcome unknown | 202 | `cloud_send_outcome_unknown` |
| Invalid Meta token | 502 | `cloud_auth_failed` |
| Meta rate limit | 429 | `cloud_rate_limited` |
| Meta 5xx/timeout | 503 | `cloud_upstream_unavailable` |
| Cross-tenant/not found | 404 | `not_found` |

---

## 9. Cloud Graph client

### 9.1 Package layout

```
pkg/cloud/
    client.go          HTTP client, auth, retries, error parsing
    types.go           Graph request/response and Cloud DTOs
    send.go            payload builders and SendService adapter
    media.go           upload/download
    templates.go       template CRUD/send
    webhook.go         signature, parsing, routing, handlers
    signature.go       HMAC verification helpers
    credentials.go     app/credential service
    contacts.go        BSUID/username/phone identity resolution
    window.go          BSUID-aware 24-hour window
    status.go          status mapping
    account.go         quality/account/security/preferences events
    diagnostics.go     doctor/health checks
    worker.go          media download worker
    cleanup.go         retention
```

Keep the package flat enough to avoid import cycles. It may import
`pkg/instance/model`, `pkg/message/model`, `pkg/message/repository`,
`pkg/channel`, `pkg/tokencrypt`, `pkg/events/interfaces`, `pkg/ssrf`,
`pkg/storage/interfaces`, `pkg/media`, and `pkg/config`.

### 9.2 Client

```go
type Client interface {
    SendMessage(ctx context.Context, phoneNumberID string, payload any) (SendResponse, error)
    UploadMedia(ctx context.Context, phoneNumberID string, r io.Reader, mime, filename string) (MediaID, error)
    GetMedia(ctx context.Context, mediaID string) (MediaInfo, error)
    DownloadMedia(ctx context.Context, url string) (io.ReadCloser, int64, error)
    VerifyPhoneNumber(ctx context.Context, phoneNumberID string) (PhoneInfo, error)
    RegisterPhone(ctx context.Context, phoneNumberID, pin string) error
    DeregisterPhone(ctx context.Context, phoneNumberID string) error
    SubscribeApp(ctx context.Context, wabaID string) error
    ListTemplates(ctx context.Context, wabaID string, after string) ([]Template, string, error)
    CreateTemplate(ctx context.Context, wabaID string, t Template) (Template, error)
    UpdateTemplate(ctx context.Context, templateID string, t Template) (Template, error)
    DeleteTemplate(ctx context.Context, wabaID, name string) error
}
```

Implementation rules:

- Use a dedicated `http.Client`; never `http.DefaultClient`.
- `Authorization: Bearer <access token>`.
- Base URL and version from config/app row; no hardcoded Graph version in
  payload code. The reference version is `v27.0`; confirm it is still supported
  before release.
- Dial/TLS timeout 10 s, response-header timeout 60 s, JSON timeout 30 s.
  Media transfers use a 120 s per-phase/idle cap and a byte cap; do not apply a
  single overall timeout that cuts off a large upload.
- Bound JSON response bodies to 10 MiB and response headers to 1 MiB. Stream
  media with a byte cap.
- **Retry policy:** retry only idempotent methods (`GET`, `HEAD`, `PUT`,
  `DELETE`, `OPTIONS`) on 429/5xx with full jitter and `Retry-After`. Never
  automatically retry `POST /messages`, template-create, or phone-register
  requests on a network error or 5xx. A send timeout is an ambiguous outcome,
  not a retry instruction.
- For a 429 on a send, retry only when the response confirms the request was not
  accepted; otherwise return `cloud_rate_limited` with `Retry-After`.
- Parse `X-Business-Use-Case-Usage`; when any usage percentage is near 90%,
  halve the effective rate before hitting the hard block.
- Adaptive limiter: conservative configured floor, halve on 429/90% usage,
  restore gradually (+25% per 10 s of calm), full jitter, `Retry-After` wins.
- Capture `X-FB-Trace-ID`/`fbtrace_id` for support; never log the token.
- Parse the full error envelope: `message`, `type`, `code`,
  `error_subcode`, `error_user_title`, `error_user_msg`,
  `error_data.details` and `fbtrace_id`.
- Set a `WaMux/<version>` user agent.
- Treat a 200 with `error` as an error.
- Use `context.Context` end to end; cancellation must interrupt backoff.
- Shared transport with bounded idle connections and connection reuse.

### 9.2a Send operation state

WaMux already has a `send_operations` table for `Idempotency-Key`. Extend it
with a Cloud operation state:

- `pending` — request not yet written to Graph.
- `accepted` — Graph returned a `wamid`.
- `failed` — Graph returned a definitive failure.
- `unknown` — the request may have been accepted but the response was lost
  (timeout, connection reset, 5xx after write).

Rules:

- Set `unknown` on send timeout/network loss/ambiguous 5xx. Do not retry
  automatically.
- Return `202 cloud_send_outcome_unknown` with the operation ID and a
  “do not retry blindly” message.
- `GET /send/operations/:operationId` reports the state and any `wamid` later
  correlated from a status webhook.
- Include the operation ID in `biz_opaque_callback_data` (<=512 chars,
  URL-safe).
- A caller may explicitly retry with the same `Idempotency-Key`; WaMux must
  still warn that Graph may have accepted the first request.

### 9.3 Payload builders

```
POST /{version}/{phone-number-id}/messages
{
  "messaging_product": "whatsapp",
  "recipient_type": "individual", // or "group" when implemented
  "to": "<digits>",              // phone, optional when recipient is used
  "recipient": "US...",          // BSUID/parent BSUID, optional when to is used
  "type": "<type>",
  "biz_opaque_callback_data": "<waMux operation id>",
  ...
}
```

When both `to` and `recipient` are present, Meta gives `to` precedence. When
sending to a group, use the group ID and `recipient_type: "group"`.

Supported builders:

| Public route | Cloud type | Notes |
|---|---|---|
| `/send/text` | `text` | `preview_url` optional |
| `/send/link` | `text` | send URL as text; custom title/description not supported |
| `/send/media` | `image`/`video`/`audio`/`document`/`sticker` | upload or URL, caption/filename |
| `/send/location` | `location` | latitude/longitude/name/address |
| `/send/contact` | `contacts` | one or more contacts; map vCard fields |
| `/send/button` | `interactive` | reply/url/copy subset; pix unsupported |
| `/send/list` | `interactive` | Cloud list limits; trim/validate |
| `/message/react` | `reaction` | `message_id` + `emoji` |
| `/send/template` | `template` | name/language/components |
| quoted reply | `context.message_id` | applies to supported types |

Deliberately deferred or unavailable:

- Cloud-supported but not implemented: Groups, Calling, Flows, product/catalog
  interactive messages, CTA URL/contact/location-request interactions.
- No equivalent: `/send/album`, `/send/poll`, `/send/carousel` (unless mapped to
  a template), `/send/event`, `/send/status/text`, `/send/status/media`,
  communities, newsletters and status/stories. `/send/product` can be mapped to
  a Cloud product template later if catalog support is added.

### 9.4 Media

Outbound:

1. Validate size/mime against Meta limits (image ~5 MB, video ~16 MB, audio
   ~16 MB, document ~100 MB, sticker ~500 KB; confirm at implementation time).
2. Upload multipart to `/{phone-number-id}/media` with `type` and `file`.
3. Send `{type, <type>: {id, caption?, filename?}}`.
4. Persist the outbound row and local/S3 copy before returning.

Inbound:

1. Webhook gives `media.id`, `mime_type`, `sha256`, `filename`, optional caption.
2. Enqueue a `cloud_media_jobs` row.
3. Worker calls `GET /{media-id}` to obtain the short-lived URL and size.
4. Enforce `CLOUD_API_MAX_MEDIA_BYTES` and the declared `file_size`.
5. Download with the bearer token, verify SHA-256 when supplied, store through
   `MediaStorageFor(instance)` / local media.
6. Update the message row's `media_url`, `media_mimetype`, `file_name`,
   `file_size`; publish the `Message` event only after the stored URL exists
   (or publish once with `mediaStatus: "pending"` and again on completion; choose
   one and document it).
7. On failure, keep the message row, record the error, and allow retry.

Never trust the webhook's filename for a local path. Generate a safe name.

### 9.5 Error mapping

Parse `error.code`, `error_subcode`, `error_user_title`,
`error_user_msg`, `fbtrace_id`.

| Meta code | WaMux code | Retry? |
|---|---|---|
| `190` | `cloud_auth_failed` | no |
| `131009` | `cloud_invalid_message_id` | no |
| `131026` | `cloud_undeliverable` | no |
| `131030` | `cloud_recipient_not_allowed` | no; add number to the test allowlist |
| `131031` | `cloud_account_locked` | no |
| `131042` | `cloud_payment_issue` | no |
| `131047` | `cloud_window_expired` | no (template required) |
| `131048` | `cloud_rate_limited` | yes for idempotent requests only |
| `131051` | `unsupported_on_channel` | no |
| `131056` | `cloud_pair_rate_limited` | pause sends to this recipient |
| `131062` | `cloud_bsuid_unsupported` | no |
| `132000`/`132001` | `cloud_template_error` | no |
| `132005`/`132007`/`132012` | `cloud_template_error` | no |
| `133010` | `cloud_phone_not_registered` | no; register the number |
| `133016` | `cloud_registration_limit` | no; 10 registration attempts per 72 h |
| `147005` | `cloud_username_transfer_required` | no |
| `368` | `cloud_temporarily_blocked` | only after `Retry-After` |
| `80007`/`130429` | `cloud_rate_limited` | only for idempotent requests |
| unknown | `cloud_upstream_error` | 5xx for GET/PUT/DELETE only |

Include `error_user_title`, `error_user_msg`, `error_data.details` and
`fbtrace_id` in internal diagnostics. The public error should be actionable but
must not leak the access token or raw request bodies.

Always include the Meta trace id in structured logs, not in the public error
unless it helps support.

---

## 10. Inbound webhook processing

### 10.1 Signature verification

```
rawBody, err := readLimited(c.Request.Body, CLOUD_API_MAX_WEBHOOK_BYTES)
candidateApps := resolveAppsFromPathOrBody(c.Request, rawBody)
for each app secret:
    expected := "sha256=" + hex(HMAC-SHA256(secret, rawBody))
    if hmac.Equal([]byte(expected), []byte(header)) { verified = true }
if !verified { 401 }
```

If the route is `/webhook/cloud/:appId`, use that app’s secret first and do not
fall back to other app secrets. Only the single `/webhook/cloud` route needs
WABA/phone-number discovery and bounded fallback.

Important:

- Verify the **raw bytes** before unmarshalling.
- Use `hmac.Equal`, not string equality.
- Header is lowercase `x-hub-signature-256` after Gin normalization; parse
  exactly one `sha256=` prefix.
- Do not log the body on failure.
- If the body contains an unknown WABA, try all configured app secrets (bounded
  to 10) before rejecting; this handles events for numbers not yet registered.
- Prefer `/webhook/cloud/:appId` when multiple Meta apps are configured. The
  app ID is not a secret; it selects the secret used for signature verification.
- The verify token is **not** accepted as POST authentication.
- Body cap is 4 MiB by default (Meta documents 3 MB). Oversized requests get
  413.
- Wrap processing in `defer recover()`; a malformed event must not panic the
  server.

### 10.2 Event shape

```json
{
  "object": "whatsapp_business_account",
  "entry": [
    {
      "id": "WABA_ID",
      "changes": [
        {
          "field": "messages",
          "value": {
            "messaging_product": "whatsapp",
            "metadata": {
              "display_phone_number": "5511999999999",
              "phone_number_id": "1122334455"
            },
            "contacts": [
              {
                "profile": {"name": "Ana", "username": "ana.example"},
                "wa_id": "5511888888888",
                "user_id": "BR.13491208655302741918",
                "parent_user_id": "BR.ENT.13491208655302741918"
              }
            ],
            "messages": [
              {
                "from": "5511888888888",
                "from_user_id": "BR.13491208655302741918",
                "id": "wamid...",
                "timestamp": "1763900000",
                "type": "text",
                "text": {"body": "hello"}
              }
            ]
          }
        }
      ]
    }
  ]
}
```

Statuses arrive in the same `value` with a `statuses` array.

### 10.3 Processing steps

1. Validate `object == whatsapp_business_account`.
2. Iterate entries/changes; ignore unknown `field` values safely and record a
   metric. One malformed change must not fail the whole payload.
3. Resolve credentials by `value.metadata.phone_number_id`.
4. For each message:
   - Build `event_key`; claim in `cloud_webhook_events`.
   - Resolve the contact identity from `from_user_id`, `contacts[].user_id`,
     `wa_id`, `parent_user_id` and `profile.username`. Create/update
     `cloud_contacts`.
   - Compute `contact_id` (`bsuid:<value>` or `phone:<digits>`).
   - Map `type` to the internal message type.
   - Persist a `message_model.Message` with `channel="cloud"`,
     `status="Received"`, `is_from_me=false`, `contact_id`, BSUID columns and
     text/caption/media metadata.
   - Upsert `cloud_chat_windows` by `contact_id` using the Meta timestamp.
   - If media, insert `cloud_media_jobs` (ignore duplicate).
   - Publish a `Message` event through the shared `EventSink`.
   - Update `lastWebhookAt` for the instance.
5. For each status:
   - Claim dedupe key.
   - Resolve `recipient_user_id`/`recipient_id` to `contact_id` when present.
   - Insert `cloud_status_events` (unique key) and update `messages.status`
     using the rank rules.
   - Publish a `Receipt` event with `status` mapped to `Sent`/`Delivered`/
     `Read`/`Failed`.
6. Handle account/template events:
   - `message_template_status_update`, `message_template_quality_update`,
     `template_category_update`, `message_template_components_update`.
   - `phone_number_quality_update`, `account_alerts`, `account_review_update`,
     `business_capability_update`, `security`, `business_username_updates`.
   - `user_preferences` → `cloud_user_preferences`.
   - Store in `cloud_account_events`/`cloud_templates`; publish a `Template` or
     account event where useful.
7. Handle customer identity/number-change events by updating `cloud_contacts`
   and preserving the existing conversation. Do not create a duplicate chat
   just because the phone/BSUID changed.
8. Return 200 only after all non-media work committed. Media work remains
   queued.

### 10.4 Event mapping

Use the existing event names so consumers do not need a new API:

- inbound message → `Message`
- status → `Receipt`
- credentials verified/registered → `Connected`
- template/account/quality/security update → `Template` (new event type) or
  `CloudAccountEvent`
- user preference change → `CloudUserPreference`

Add to every Cloud event:

```json
{
  "channel": "cloud",
  "instanceId": "...",
  "instanceName": "...",
  "data": { ... }
}
```

Payload shape should stay close to the Baileys event shape where possible:
`key.remoteJid` (the compatibility chat key), `key.fromMe`, `key.id`,
`pushName`, `message`, `messageType`, `messageTimestamp`, `source`. Add:
`contactId`, `bsuid`, `parentBsuid`, `username`, `waId`. For media, include the
stored `mediaUrl`/`mediaMimetype`, not the expiring Meta URL.

`remoteJid` must not be treated as a phone number when the contact is
BSUID-only. Consumers should use `contactId` and the identity fields.

### 10.4a Inbound type mapping

| Cloud `type` | Internal type | Payload handling |
|---|---|---|
| `text` | `text` | `text.body` |
| `image` | `image` | media id, caption, mime |
| `video` | `video` | media id, caption, mime |
| `audio` | `audio` | media id, mime, voice flag |
| `document` | `document` | media id, filename, caption |
| `sticker` | `sticker` | media id, mime |
| `location` | `location` | lat/long/name/address |
| `contacts` | `contact` | vCard/contact array |
| `reaction` | `reaction` | `message_id`, `emoji` |
| `button` | `button_reply` | `payload`, `text` |
| `interactive` | `interactive_reply` | `button_reply`, `list_reply`, `nfm_reply`, `call_permission_reply` |
| `order` | `order` | store raw metadata; no product sync |
| `system` | `system` | store/log only |
| `request_contact_info` | `contact_request` | contact info/vCard; update identity |
| unknown | `unknown` | persist raw type, emit event, never panic |

Every inbound message should carry `from_user_id`, `from`, and the contact’s
`user_id`/`parent_user_id`/username when present. If only a BSUID is available,
create a BSUID-keyed contact and do not invent a phone number.

For unsupported inbound types, persist the message metadata and log a warning;
do not drop the webhook. The manager can show a generic “unsupported message”
row.

### 10.5 Dedupe and ordering

- Cloud retries webhooks. The unique key plus `processed_at` is mandatory.
- Statuses may arrive out of order; status rank handles it.
- Do not use receipt time for the 24-hour window; use Meta message timestamp.
- If a duplicate arrives while the first is processing, return 200; the first
  will either complete or release the claim for retry.

---

## 11. Instance lifecycle details

### 11.1 Create

- Validate `integration` against the allowed enum.
- If Cloud:
  - Require `DATA_ENCRYPTION_KEY`.
  - Validate numeric-looking `appId`, `wabaId`, `phoneNumberId`; access token
    non-empty.
  - Insert instance + credentials in one transaction.
  - Set `Connected=false`, `registered=false`, `AutoReconnect=false` until
    verified.
  - Never write the Meta token to `instances.token` or store the registration
    PIN.
- If Baileys: current behavior, `integration` defaults; no other changes.

### 11.2 Connect

- Load credentials; missing → `400 cloud_credentials_missing`.
- Decrypt token; if decryption fails → `500` with a config/rotation message.
- Call `GET /{phone-number-id}?fields=display_phone_number,verified_name,quality_rating,status`.
- Call `POST /{waba-id}/subscribed_apps`.
- Check phone registration; if unregistered, return `cloud_phone_not_registered`
  and require `cloud/register`.
- Update cached display name/phone, `last_verified_at`, `last_verify_error`,
  `registered`, `registration_status`, `quality_rating`, `messaging_limit`.
- Set `Connected=true` on successful verification; store `subscribed` and
  `registered` separately.
- No QR, no `whatsmeow.Client`, no ownership lease.
- On failure, keep the instance and credentials; return the error.

### 11.2a Register phone

- `POST /instance/cloud/register` accepts the six-digit PIN.
- Call Graph `POST /{phone-number-id}/register`.
- On success: `registered=true`, `registered_at=now`, clear error; publish
  `Connected`/diagnostic update.
- Never persist the PIN; leave it in memory only for the request.
- If Meta says the number is already registered, treat it as success.
- Registration is limited to 10 attempts per number per 72 hours; surface
  `133016` and do not retry automatically.
- `deregister` is an explicit admin action for retiring a number, not part of
  instance deletion.

### 11.3 Status

- Do not call Graph on every poll.
- Return cached verification, `lastWebhookAt`, subscription and registration
  flags, quality/messaging limit and capabilities.
- If credentials are missing, return `connected=false` plus a clear reason.
- `GET /instance/cloud/diagnostics` can refresh token/registration/webhook
  checks explicitly; status itself stays cheap.

### 11.4 Disconnect

- Set `Connected=false`, `AutoReconnect=false`.
- Keep credentials/templates/windows.
- Do not unsubscribe the app; other instances may share it.

### 11.5 Delete

- Delete child rows (`cloud_status_events`, `cloud_account_events`,
  `cloud_user_preferences`, `cloud_media_jobs`, `cloud_webhook_events`,
  `cloud_chat_windows`, `cloud_contacts`, `cloud_templates`,
  `cloud_api_credentials`) in a transaction, then the instance.
- Remove local media.
- Skip `whatsmeow` logout/device-store cleanup.
- Audit deletion without secrets.

### 11.6 Logout/QR/Pair

- Cloud: return `422 unsupported_on_channel` with a message telling the operator
  to rotate credentials or delete the instance.
- Do not let a generic handler accidentally start a WhatsApp socket.

### 11.7 Force reconnect

- For Cloud, optionally alias to `POST /instance/cloud/verify`.
- Do not call `whatsmeowService.ForceReconnect` for Cloud.

### 11.8 Boot behavior

- `ConnectOnStartup` and the reconnect loop must skip Cloud instances.
  `Integration` is checked before any `whatsmeowService.StartInstance`.
- Do not acquire an ownership lease for a Cloud instance.
- Do not make a Graph request for every Cloud instance at boot. Verification is
  lazy (`Connect`/`verify`/status cache) so a Meta outage cannot slow startup.
- Cloud media workers and webhook routes start with the process; they are
  stateless and need no per-instance socket.

### 11.9 Read-side endpoints and capability gating

The send path is not the only place that assumes whatsmeow. Audit every route
that touches the client and give Cloud an explicit answer:

| Surface | Cloud behavior |
|---|---|
| `GET /chat/history`, `GET /chat/chats` | Works from `messages`; no contact sync; resolve Cloud identity by `contact_id` |
| `GET /chat/contacts` / contact resolution | BSUID/username/`wa_id` fallback; no address book |
| `POST /chat/read` / `POST /message/read` | Graph “mark as read”, optionally with `typing_indicator` |
| `GET /message/*` history | Works from DB |
| `POST /message/react` | Supported in Phase 3 |
| `GET /user/*` (check, avatar, privacy, block) | `unsupported_on_channel` |
| `GET /group/*`, `/community/*`, `/newsletter/*` | `unsupported_on_channel`; Cloud Groups is a separate deferred project |
| `GET /instance/overview` | Return zero/null profile fields; do not call whatsmeow |
| `GET /instance/qr`, `POST /pair`, `POST /logout` | `unsupported_on_channel` |
| `GET /server/health` | Include Cloud instance counts, registration, webhook/media state |

Implement this as a small `ChannelCapabilities` check in handlers, not as
nil-pointer defensive code. A Cloud instance must never reach
`clientPointer.Get()` in a path that assumes a connected Baileys client.

For BSUID-only contacts, `chat_jid` may be a synthetic compatibility key. The
manager and chat service should use `contact_id`/`cloud_contacts` for display
name, username and phone, and must not parse `chat_jid` as a phone JID.

### 11.10 Typebot/chatbot dispatch

After persisting an inbound Cloud text message, dispatch to the Typebot
processor asynchronously, like Baileys does. The Cloud service can depend on the
existing `TypebotProcessor` interface (not the Typebot service package) to avoid
a cycle. If no processor is configured, skip. Test that a Typebot reply goes
back through `SendRouter` and is sent by the Cloud channel.

---

## 12. Security

- **Secrets:** AES-256-GCM via `pkg/tokencrypt`; app secret, access token and
  registration PIN never appear in API responses, logs, audit detail or errors.
- **Webhook:** HMAC-SHA256 over raw bytes, constant-time compare, 4 MiB body cap,
  signature-before-side-effects, panic recovery.
- **BSUID/identity:** treat BSUID/username as personal data; scope all identity
  lookups by `instance_id`; do not merge contacts across instances or business
  portfolios.
- **SSRF:** outbound URL media fetches use `pkg/ssrf`; Graph base URL is fixed
  except for tests. Inbound media URLs returned by Graph must be HTTPS,
  public-IP validated, and (defensively) restricted to Meta/CDN host suffixes;
  never fetch an arbitrary URL supplied by a webhook.
- **Tenant isolation:** every repository query is scoped by `instance_id`;
  `phone_number_id` is globally unique and maps to one instance.
- **Replay:** `cloud_webhook_events`; message/status upserts are idempotent.
- **Token rotation:** write-only endpoint; old token not echoed; audit a
  fingerprint (e.g. last 4 chars) if needed, never the token.
- **Rate limiting:** webhook route gets its own generous limiter; do not let
  Meta bursts exhaust the admin/API limiter. Invalid signatures should be
  counted and may be blocked per source after a threshold.
- **Audit:** cloud credential create/update/delete, template create/delete,
  instance connect/disconnect, webhook signature failures (sampled).
- **Encryption at rest:** ensure DB backups include the key management plan;
  document that losing `DATA_ENCRYPTION_KEY` loses Cloud credentials.

---

## 13. Observability

Add metrics (existing registry):

- `wamux_cloud_graph_requests_total{operation,result}`
- `wamux_cloud_graph_request_duration_seconds{operation}`
- `wamux_cloud_send_total{type,result}`
- `wamux_cloud_webhook_events_total{type,result}`
- `wamux_cloud_webhook_signature_failures_total`
- `wamux_cloud_window_expired_total`
- `wamux_cloud_template_status_total{status}`
- `wamux_cloud_media_jobs_total{status}`
- `wamux_cloud_media_queue_depth`
- `wamux_cloud_send_outcome_unknown_total`
- `wamux_cloud_rate_limit_usage{scope}`
- `wamux_cloud_registration_total{result}`
- `wamux_cloud_account_events_total{field}`
- `wamux_cloud_webhook_partial_failures_total`

Health:

- `/server/health`: add `cloud_instances`, `cloud_webhook_last_received_at`,
  `cloud_media_backlog`.
- Per-instance status: `lastVerifiedAt`, `lastWebhookAt`, `lastGraphError`,
  `subscribed`.
- Structured logs: `instanceId`, `operation`, `phoneNumberId`, `fbtrace_id`,
  `httpStatus`, `duration`; never token/body.

---

## 14. Manager

### 14.1 Create instance

- Add a channel selector: “WhatsApp Web (QR)” vs “WhatsApp Cloud API”.
- Cloud fields: app (select or app id), WABA id, phone number id, business id,
  access token, enforce window.
- Create in one request; no QR dialog.
- After creation, show **Verify connection** and **Register phone** steps.
- Registration asks for the six-digit PIN and never stores it.
- Show a diagnostics button that checks token, registration, subscription and
  webhook health.

### 14.2 Instances list

- Cloud badge and display phone number/username.
- Replace “Connect/QR” with “Verify/Register” for Cloud.
- Hide QR/pair/logout actions; show rotate credentials, diagnostics/delete.
- Status panel: verified, registered, subscribed, quality/messaging limit,
  last webhook, capabilities.
- Show BSUID-only conversations with the username/display name, not a blank
  phone field.

### 14.3 Instance settings

- Cloud credentials form (write-only token, app/WABA/phone/business).
- Verify/register buttons, registration status, subscription status, webhook
  URL copy field.
- Diagnostics panel: token, app secret, registration, subscription, last
  webhook, last Graph error, rate-limit state.
- 24-hour window toggle/status and BSUID-aware contact identity.
- Templates tab: status, quality, category, rejected reason, create/edit/delete,
  sync.
- Account events panel: quality, messaging limit, security/two-step verification,
  username changes.
- Danger zone: rotate credentials, delete instance.

### 14.4 Conversations/send UI

- Capability-aware buttons: hide/disable unimplemented groups, calls, flows,
  albums, polls, newsletters, status and presence.
- Template picker; if the window is closed, require a template.
- BSUID-only chat: show username/display name, not a missing phone number.
- Typing indicator on mark-read when the instance supports it.
- Show `Failed` status with Meta error code/message from
  `cloud_status_events`.
- Show `Template` badge and `Cloud` channel badge.
- Do not offer “sync history” or “contacts” for Cloud.
- Do not label Groups/Calling/Flows as “not supported by WhatsApp”; label them
  “not implemented in WaMux.”

### 14.5 i18n and tests

- Add English and Portuguese strings.
- Component tests for create flow, credential redaction, capability gating and
  window-closed send path.

---

## 15. File-level work breakdown

### Go

| File/package | Change |
|---|---|
| `pkg/channel/channel.go` | **new**: `Kind`, `Capabilities`, unsupported error |
| `pkg/cloud/client.go` | **new**: Graph client, retries, error parsing |
| `pkg/cloud/types.go` | **new**: Graph/Cloud DTOs |
| `pkg/cloud/send.go` | **new**: payload builders, Cloud sender |
| `pkg/cloud/media.go` | **new**: upload/download, limits |
| `pkg/cloud/templates.go` | **new**: CRUD/send/cache |
| `pkg/cloud/webhook.go` | **new**: parse/route/persist |
| `pkg/cloud/signature.go` | **new**: HMAC verification |
| `pkg/cloud/credentials.go` | **new**: app/credential/registration service |
| `pkg/cloud/contacts.go` | **new**: BSUID/username/phone identity |
| `pkg/cloud/window.go` | **new**: BSUID-aware 24-hour window |
| `pkg/cloud/status.go` | **new**: status mapping |
| `pkg/cloud/account.go` | **new**: quality/account/security/preferences events |
| `pkg/cloud/diagnostics.go` | **new**: token/registration/webhook diagnostics |
| `pkg/cloud/worker.go` | **new**: media download worker |
| `pkg/cloud/cleanup.go` | **new**: retention |
| `pkg/cloud/*_test.go` | **new**: unit/integration tests |
| `pkg/instance/model/instance_model.go` | add `Integration` |
| `pkg/instance/service/instance_service.go` | Cloud branches in create/connect/status/delete/logout |
| `pkg/instance/service/cloud_service.go` | **new**: credentials/verify/registration/subscription/diagnostics |
| `pkg/instance/handler/instance_handler.go` | Cloud handlers, error mapping, status, register |
| `pkg/instance/repository/*` | create with Cloud credentials transaction |
| `pkg/sendMessage/service/send_service.go` | extract/reuse outbound persistence; add channel field |
| `pkg/sendMessage/service/router.go` | **new**: `SendService` router |
| `pkg/sendMessage/service/template_structs.go` | **new**: `TemplateStruct` |
| `pkg/sendMessage/handler/send_handler.go` | `/send/template`, BSUID recipient fields, error mapping |
| `pkg/sendMessage/handler/*` | pass channel capability errors through |
| `pkg/routes/routes.go` | public webhook; Cloud/template/register/diagnostics routes |
| `pkg/migrations/migrations.go` | migrations 0018–0029 |
| `pkg/message/model/message_model.go` | `Channel`, `contact_id`, BSUID columns |
| `pkg/message/repository/message_repository.go` | `Failed` rank, channel/contact/error handling |
| `pkg/internal/event_types/event_types.go` | add `TEMPLATE` |
| `pkg/events/interfaces/*` | optional `EventSink` interface |
| `pkg/config/config.go` | Cloud configuration |
| `pkg/docmodels/docmodels.go` | Cloud request/response Swagger models |
| `cmd/wamux/main.go` | wire cloud client/service/router/worker; feature flag |
| `pkg/server/handler/*` | Cloud health/metrics fields |
| `pkg/audit/*` | Cloud audit actions |

### Manager

| File | Change |
|---|---|
| `wamux-manager/src/types/instance.ts` | integration/cloud types |
| `wamux-manager/src/services/api/instances.ts` | Cloud create/verify/credentials calls |
| `wamux-manager/src/services/api/templates.ts` | **new** template API |
| `wamux-manager/src/components/instances/CreateInstanceModal.tsx` | channel selector + Cloud form |
| `wamux-manager/src/components/instances/InstanceCard.tsx` | Cloud badge/actions |
| `wamux-manager/src/components/instances/CloudCredentialsForm.tsx` | **new** |
| `wamux-manager/src/components/instances/CloudRegisterPhone.tsx` | **new** |
| `wamux-manager/src/components/instances/CloudDiagnostics.tsx` | **new** |
| `wamux-manager/src/components/instances/CloudStatusPanel.tsx` | **new** |
| `wamux-manager/src/components/instances/TemplateManager.tsx` | **new** |
| `wamux-manager/src/pages/InstanceSettings.tsx` | Cloud tab |
| `wamux-manager/src/pages/Instances.tsx` | Cloud connect flow |
| `wamux-manager/src/pages/Messages.tsx` | capability gating, window/template |
| `wamux-manager/src/components/instances/SendMessageModal.tsx` | hide unsupported types/template picker |
| `wamux-manager/src/i18n/*` | strings |

### Docs

- `docs/wiki/guias-api/api-cloud.md`
- `docs/wiki/referencia/environment-variables.md`
- `docs/wiki/deploy-producao/*` (HTTPS callback, Meta app setup)
- `docs/archive/plans/cloud-api-coverage.md` — implemented/deferred/not-planned
  operation matrix and permissions
- Swagger annotations and generated docs
- Update the WABA spike/design links
- `README` feature matrix

---

## 16. Work plan and estimates

Estimates assume one engineer familiar with WaMux, including tests and review
time. Add 20–30% for Meta app review/ops wait time.

### Phase 0 — prerequisites (3–5 days)

- Meta Business Portfolio, WABA, test phone number, system-user token.
- Meta app product setup, app secret, webhook verify token.
- Public HTTPS callback URL/tunnel for staging.
- Permissions: `whatsapp_business_messaging`,
  `whatsapp_business_management`.
- Save sanitized webhook fixtures: text, media, status, template,
  BSUID-only, phone+BSUID, phone-only, user preference, account quality.
- Confirm the Graph version; reference fixture uses `v27.0`.
- Confirm phone registration requirements and the six-digit PIN workflow.
- Decide single-app vs multi-app deployment and whether `/webhook/cloud/:appId`
  is needed.

**Exit:** a `curl` to Graph returns the test phone number; the number is
registered; a signed webhook reaches a stub endpoint; BSUID and phone-only
fixtures are captured.

### Phase 1 — channel skeleton, credentials, BSUID, signed webhook, text (16–20 days)

| ID | Task | Est. |
|---|---|---:|
| P1.1 | `instances.integration`, `messages.channel`, `contact_id`, BSUID models | 2 d |
| P1.2 | migration steps 0018–0029 + SQLite/Postgres tests | 2 d |
| P1.3 | `cloud_apps`/credentials/registration service with `tokencrypt` | 3 d |
| P1.4 | Graph client core: auth, timeouts, retry policy, error envelope | 3 d |
| P1.5 | `SendRouter` + Cloud text send + operation state/persistence/event | 4 d |
| P1.6 | webhook GET/POST signature + `/webhook/cloud/:appId` routing | 2 d |
| P1.7 | BSUID/phone identity resolution + inbound text/status + dedupe | 4 d |
| P1.8 | Cloud create/connect/register/status/disconnect/delete | 3 d |
| P1.9 | handler error mapping, diagnostics, Swagger, audit | 2 d |
| P1.10 | tests, docs, manager minimal create/verify/register/status | 4 d |

**Exit:** a Cloud instance registers, sends and receives text to phone or BSUID;
statuses update; ambiguous send outcomes are surfaced without retry; unsigned
webhooks are rejected; existing Baileys tests still pass.

### Phase 2 — media, templates, window (13–16 days)

| ID | Task | Est. |
|---|---|---:|
| P2.1 | media upload (multipart/URL), limits, local/S3 | 3 d |
| P2.2 | inbound media job queue/worker, SHA/size checks | 3 d |
| P2.3 | template CRUD + local cache + paginated sync | 3 d |
| P2.4 | `/send/template` + component/parameter validation + BSUID auth rules | 3 d |
| P2.5 | BSUID-aware 24-hour window tracking and pre-check | 2 d |
| P2.6 | manager templates/window/media/registration UI | 3 d |
| P2.7 | tests/fixtures/docs | 2 d |

**Exit:** image/video/document/audio round-trip; templates can be created,
synced and sent; free-form outside the window returns a clear error; template
quality/category/component updates appear.

### Phase 3 — interactive, operations, hardening (10–13 days)

| ID | Task | Est. |
|---|---|---:|
| P3.1 | buttons/lists/location/contact/reaction mapping + typing indicator | 3 d |
| P3.2 | adaptive rate limiter, `Retry-After`, full error hints | 3 d |
| P3.3 | account/quality/security/user-preference webhooks | 3 d |
| P3.4 | diagnostics, metrics, health, audit, log redaction | 2 d |
| P3.5 | capability matrix + manager gating; coverage manifest | 2 d |
| P3.6 | load/race/fuzz tests, docs, release notes | 3 d |

**Exit:** production checklist and acceptance criteria pass; load and security
tests pass.

### Phase 4 — Embedded Signup/token rotation (10–15 days, optional)

- Facebook Login for Business, code exchange, shared WABA discovery.
- App review, privacy/terms.
- Token rotation UI and expiry reminders.
- Multi-app onboarding and per-app webhook paths.

Do not start Phase 4 until manual onboarding has real usage.

### Phase 5 — Cloud product expansion (separate projects, optional)

Not part of Phases 1–3:

- Groups API (OBA only; max 8 participants; text/media/text templates only).
- Calling API (WebRTC/SIP, call permissions, calls webhooks).
- Flows and product/catalog interactive messages.

Each is a product decision and estimate of its own. Do not fold them into the
8–10 week estimate.

### Total

- **Phases 1–3:** 8–10 engineer-weeks.
- **Phase 4:** +2–3 weeks.
- **Functional demo only:** 2–3 weeks (not production-ready).
- **Groups/Calling/Flows:** separate projects; no estimate included here.

---

## 17. Test plan

### Unit

- `pkg/cloud/payload_test.go`: every supported type, phone/BSUID recipients,
  precedence, limits, quoted reply, `wamid`, unsupported operations.
- `pkg/cloud/contacts_test.go`: BSUID-only, phone-only, both, parent BSUID,
  username changes, number changes, identity merge without duplicate chats.
- `pkg/cloud/signature_test.go`: valid, invalid, missing, malformed prefix,
  multiple app secrets, `/webhook/cloud/:appId`, body mutation, unknown WABA.
- `pkg/cloud/webhook_test.go`: text, media, statuses, failed errors, template
  status/quality/category/components, account/security/user-preference events,
  unknown phone, duplicate, malformed single change, oversized 4 MiB+ payload,
  nil arrays, panic recovery.
- `pkg/cloud/errors_test.go`: Graph code mapping, `error_data.details`,
  `error_user_title`/`error_user_msg`, `fbtrace_id`, retry decisions,
  `Retry-After` both forms, no POST retry on timeout/5xx.
- `pkg/cloud/window_test.go`: open/closed, BSUID key, timestamp, template
  bypass, Meta `131047` mapping.
- `pkg/cloud/templates_test.go`: component/parameter payloads, cursor
  pagination, status/quality mapping, cache updates, BSUID auth restriction.
- `pkg/cloud/ratelimit_test.go`: adaptive slowdown, 90% usage, 429 halving,
  recovery, jitter, Retry-After.
- `pkg/cloud/account_test.go`: account/quality/security/username/user-preference
  events and manager-visible state.
- `pkg/cloud/diagnostics_test.go`: token, app secret, registration, subscription,
  webhook and template checks.
- `pkg/channel/router_test.go`: routing by integration, unsupported errors,
  no Baileys call for Cloud.
- `pkg/instance/service/cloud_test.go`: create transaction, rotate, verify,
  register, diagnostics, status, delete, no secret/PIN in JSON.
- `pkg/message/repository/status_order_test.go`: add `Failed` transitions and
  BSUID `contact_id` reconciliation.
- `pkg/migrations/*`: apply on SQLite and Postgres; seeded Baileys rows keep
  working.
- Manager Vitest: create Cloud, credential redaction, phone registration,
  BSUID display, diagnostics, capability gating, window-closed send, template
  picker.

### Integration/HTTP

- `httptest` Graph fake for send, upload, download, verify, register, subscribe,
  templates, 429, 5xx, timeout, malformed response.
- Webhook fixture → instance lookup → message row → fake `EventSink` receives
  one `Message`; BSUID-only fixture resolves to one contact/chat.
- Status fixture → row status moves forward only; failed errors and BSUID
  recipient stored.
- Duplicate delivery → one row/event.
- Bad signature → 401, no DB writes.
- Unknown phone number + valid signature → 200, no DB writes.
- One malformed change among valid changes → valid ones processed, 200.
- Media job → storage → message row URL; retry after transient failure.
- Phone registration: success, already registered, invalid PIN, `133010`.
- Send timeout → operation marked `unknown`, no automatic second POST.
- Multi-tenant: two Cloud instances; event routed to the right one; instance A
  token cannot read B templates/status/messages.
- Secret scan: DB rows/API JSON/log buffer contain no access token/app secret/
  registration PIN.

### Fuzz/race

- Fuzz webhook JSON and signature header parsers.
- Fuzz template `components`.
- `go test -race -shuffle=on ./...`.
- Webhook burst (1k events), send concurrency, media workers, no goroutine
  leaks.

### Manual staging

- Meta test number; send text/template to a verified recipient by phone and by
  BSUID.
- Verify webhook in Meta dashboard, including a BSUID-only payload.
- Register the phone number with the six-digit PIN; confirm `133010` guidance.
- Rotate token; confirm old token fails and new one works.
- Trigger a rejected template and confirm UI/status/quality/category.
- Trigger a test-number allowlist error (`131030`) and confirm the hint.
- Confirm no Graph call at boot for disconnected Cloud instances.

---

## 18. Acceptance criteria

A release is acceptable only if all are true:

1. Existing Baileys instances, sends, QR, history and tests are unchanged;
   `integration` defaults to `WHATSAPP-BAILEYS`.
2. Migrations apply cleanly on Postgres and SQLite, including a seeded
   pre-Cloud database with existing messages/tokens.
3. A Cloud instance can be created with encrypted credentials; API responses
   and logs never reveal the Meta token, app secret or registration PIN.
4. A Cloud instance can verify and register its phone number; `133010` produces
   a clear registration action, and the PIN is not persisted.
5. `GET /webhook/cloud` echoes the challenge for a valid verify token and
   rejects others; `/webhook/cloud/:appId` works for multi-app deployments.
6. `POST /webhook/cloud` rejects missing/invalid signatures with 401, recovers
   from panics, accepts a 4 MiB payload, and processes valid signed events.
7. Inbound text is persisted once, routed to the correct instance, resolves
   phone/BSUID identity, opens the 24-hour window and emits one `Message` event.
8. A BSUID-only inbound message produces a stable contact/chat without a phone
   number; a phone+BSUID event does not create a duplicate chat.
9. Outbound text/template accepts `number` or `recipient`, returns a Graph
   `wamid`, stores a `Sent` row and emits one `SendMessage` event.
10. A send timeout/5xx never causes an automatic second POST; the operation is
    marked `unknown` and surfaced to the caller.
11. Status webhooks move `Sent → Delivered → Read` monotonically, store
    `Failed` errors without downgrading `Read`, and retain BSUID recipients.
12. Media upload/download works for image, video, audio and document; size/type
    limits are enforced; expired/failed downloads are retryable.
13. Template create/list/send/sync works with cursor pagination; status,
    quality, category and component updates appear in the manager.
14. Free-form sends outside the window return 422 `cloud_window_expired`;
    template sends are allowed; BSUID auth-template restrictions are enforced.
15. Groups, Calling and Flows are reported as “Cloud-supported, not implemented
    in WaMux” (`501 not_implemented_on_channel`), not “Cloud does not support
    it.” Albums/polls/carousels/events/status/presence return 422
    `unsupported_on_channel`.
16. Cross-tenant access is impossible via API, webhook routing, identity lookup
    or template IDs.
17. Rate-limit handling uses adaptive pacing, `Retry-After` and full jitter;
    POST sends are never auto-retried.
18. Metrics, health, diagnostics, audit and structured logs cover Cloud
    operations, including registration and send-outcome-unknown.
19. Manager can create, verify, register, inspect, rotate and delete a Cloud
    instance, manage templates and respect capabilities.
20. Docs, Swagger, env reference and the Cloud API coverage manifest are
    updated.
21. `gofmt`, `go vet`, `go test -race -shuffle=on ./...`, `golangci-lint`, and
    manager lint/build/test pass.

---

## 19. Rollout and rollback

### Rollout

1. Merge schema and code behind `CLOUD_API_ENABLED=false`.
2. Deploy to staging; enable the flag; create a Meta test instance.
3. Verify GET/POST webhook, phone registration, text by phone/BSUID, statuses
   and media.
4. Enable in production for one internal instance.
5. Canary one real customer; monitor Graph errors, webhook failures, send
   latency, unknown send outcomes, registration and media backlog.
6. Open to all tenants after two clean weeks.

### Rollback

- Set `CLOUD_API_ENABLED=false` to stop new Cloud creation/routes.
- Keep the schema; it is additive and harmless to Baileys.
- Existing Cloud instances: decide whether to keep webhook ingestion
  (`read-only` mode) or stop it. Prefer a mode that keeps ingesting messages
  but disables outbound sends, so no inbound data is lost during a rollback.
- Never roll back by dropping tables/columns. A later release can clean up.

### Versioning

- Pin Graph version per app; upgrading is a config change plus contract/fixture
  test run. Reference version: `v27.0`.
- Re-check BSUID/username behavior and Groups OBA eligibility at each upgrade.
- Record the Meta app review status and permissions in the ops runbook.

---

## 20. Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Meta app review/verification delays | launch slips | Start Phase 0 early; use test number in staging |
| Meta API version changes | breakage | configurable version, fixtures, contract tests |
| BSUID/username rollout | missing phone identities, duplicate chats | identity table, BSUID-first contact key, merge tests |
| Groups API availability | product mismatch | capability says “not implemented in WaMux”; OBA requirements documented |
| Webhook signature with multiple apps | event rejection | `/webhook/cloud/:appId`, app/WABA mapping, bounded fallback |
| Token expiry/rotation | sends fail | expiry field, verify endpoint, diagnostics, rotation UI |
| Phone not registered | sends fail | registration step + `133010` hint + diagnostics |
| 24-hour window mismatch | rejected sends | BSUID-aware pre-check + `131047` mapping + template fallback |
| Media ID expiry/size | missing attachments | job queue, immediate download, size caps, retries |
| Duplicate webhooks/retries | duplicate events | dedupe table + idempotent upserts + status rank |
| Duplicate sends on timeout | customer spam | **no POST auto-retry**, operation `unknown`, `biz_opaque_callback_data`, Idempotency-Key |
| Rate limits | 429 storms | adaptive limiter, 90% usage slowdown, jitter, `Retry-After`, metrics |
| Partial webhook failure | retry storms/duplicates | skip bad change, 200 for valid remainder, 500 only for infrastructure failure |
| Groups/Calling/Flows absent | product mismatch | accurate capability notes; separate Phase 5 |
| Secret leakage | severe | encryption, redaction, tests, no token/PIN logs |
| Multi-node send limits | over-limit | conservative limits; shared/DB limiter or document single-node |
| DB growth | storage | retention for events/status/account/media jobs |
| Pricing/opt-in compliance | legal/cost | docs: Meta pricing, opt-in/opt-out; store `user_preferences` |

---

## 21. Documentation deliverables

- Cloud API guide: onboarding, Meta app setup, callback URL, verify token,
  phone registration, BSUID/username behavior, permissions, sending, templates,
  window, capability deferrals, troubleshooting.
- Environment variables reference.
- Swagger/OpenAPI for all new/changed routes and fields.
- Migration/release notes.
- Ops runbook: rotate token, rotate app secret, replay failed media, disable
  Cloud channel, check metrics.
- Security note: secrets encryption, signature verification, tenant isolation.
- Update the comparison/spike docs to point at this plan.

---

## 22. Open decisions

Recommended defaults are in bold; resolve before Phase 1 coding is complete.

1. **One Meta app per deployment** initially; support multiple apps with
   `/webhook/cloud/:appId`. The schema supports multiple; the UI starts with
   one.
2. Manual onboarding first; Embedded Signup later. **Yes.**
3. BSUID support: **mandatory in Phase 1**, not optional.
4. Send retry policy: **POST sends are never auto-retried**; operation state
   `unknown` and `biz_opaque_callback_data` are required.
5. Unsupported feature status: **422 `unsupported_on_channel`** for truly
   unavailable features; “Cloud-supported but not implemented” uses the
   capability notes and a separate error message.
6. Groups, Calling and Flows: **deferred to separate projects**; do not claim
   Cloud lacks them.
7. `/send/link`: **send as Cloud text with `preview_url`**, document the
   difference.
8. Sticker/product/carousel: **not in Phases 1–3**; revisit after real demand.
9. Multi-node rate limiting: **conservative per-node limiter first**, document
   the limit; add a Postgres/shared limiter if a deployment needs horizontal
   Cloud sending.
10. Opt-out storage: **store Meta `user_preferences`**, but the tenant owns the
    final marketing policy.
11. Template edit semantics: pass through to Graph; local cache is not the
    source of truth for approval.
12. Media event timing: **publish `Message` after the file is stored**, with a
    `mediaStatus` field; add a retry endpoint for failures.
13. Feature flag default: **off** until staging passes.

---

## 23. Definition of done

- [ ] Plan approved and open decisions resolved.
- [ ] `integration` model + migrations 0018–0029 merged.
- [ ] Encrypted Cloud credentials + registration status + audit merged.
- [ ] BSUID/username/phone identity model and backfill merged.
- [ ] Graph client with fake-server tests and no-POST-retry policy merged.
- [ ] Signed webhook (4 MiB, partial failures, panic recovery) merged.
- [ ] Inbound text/status/BSUID identity merged.
- [ ] Cloud text send + operation tracking + persistence/events merged.
- [ ] Cloud lifecycle (create/connect/register/status/diagnostics/disconnect/delete) merged.
- [ ] Media + templates + BSUID-aware window merged.
- [ ] Account/quality/security/user-preference webhooks merged.
- [ ] Interactive subset + typing indicator + capability gating merged.
- [ ] Manager create/register/settings/templates/window/diagnostics merged.
- [ ] Metrics/health/audit merged.
- [ ] Docs/Swagger/env/coverage manifest merged.
- [ ] Acceptance tests and full verification pass.
- [ ] Staging Meta test number verified by phone and BSUID.
- [ ] Rollout/rollback runbook reviewed.

---

## 24. Suggested commit sequence

Keep commits reviewable and each one test-backed:

1. `docs(cloud): revise WABA Cloud API implementation plan`
2. `feat(cloud): add integration field, BSUID identity and Cloud schema`
3. `feat(cloud): encrypt and manage Meta app/instance credentials`
4. `feat(cloud): add phone registration and diagnostics`
5. `feat(cloud): add Graph API client with safe retry policy`
6. `feat(cloud): verify and route signed webhooks`
7. `feat(cloud): receive text/status events and resolve BSUID identity`
8. `feat(send): route sends by instance integration`
9. `feat(cloud): send text by phone or BSUID with operation tracking`
10. `feat(cloud): add Cloud instance lifecycle`
11. `feat(cloud): add media upload/download`
12. `feat(cloud): add template management`
13. `feat(cloud): enforce the BSUID-aware customer service window`
14. `feat(cloud): add account/quality/security webhooks`
15. `feat(cloud): add interactive message mapping and typing indicator`
16. `feat(manager): add Cloud onboarding, registration and diagnostics`
17. `docs(cloud): add API, ops, coverage and security documentation`

Each commit should include `gofmt`, `go vet`, focused tests and, where UI is
touched, manager lint/build/test. Run the full race/shuffle suite before the
release candidate.

---

## Sources

Accessed 2026-11-22. Official Meta docs change frequently; re-check the Graph
version, group eligibility, BSUID requirements and webhook fields before
implementation. Meta’s registration examples currently use `v27.0`; the
`waba-cli` manifest references `v25.0` from August 2026. Treat `v27.0` as the
reference and pin the version after a contract test.

### Official Meta docs

- [WhatsApp Business Platform overview](https://developers.facebook.com/documentation/business-messaging/whatsapp/overview)
  — Cloud API products, including messaging, Calling and Groups.
- [Cloud API get started](https://developers.facebook.com/documentation/business-messaging/whatsapp/get-started)
  — app/WABA/test-number setup and first message.
- [Business-scoped user IDs (BSUID)](https://developers.facebook.com/documentation/business-messaging/whatsapp/business-scoped-user-ids/)
  — `user_id`, `from_user_id`, `parent_user_id`, `recipient` sends and username
  rollout.
- [Groups API](https://developers.facebook.com/documentation/business-messaging/whatsapp/groups)
  — OBA requirement, 8-participant limit, supported message types, group
  webhooks.
- [Cloud API Calling](https://developers.facebook.com/documentation/business-messaging/whatsapp/calling)
  — WebRTC/SIP, call permissions and `calls` webhooks.
- [WhatsApp Flows](https://developers.facebook.com/documentation/business-messaging/whatsapp/flows)
  — interactive Flows and `nfm_reply`; deferred.
- [WhatsApp webhooks](https://developers.facebook.com/documentation/business-messaging/whatsapp/webhooks/overview)
  — fields, retries and payload shapes, including template/account/security
  events.
- [Phone number registration](https://developers.facebook.com/documentation/business-messaging/whatsapp/business-phone-numbers/registration)
  — `register`/`deregister`, six-digit PIN and `133010`.
- [WhatsApp Cloud API message API](https://developers.facebook.com/documentation/business-messaging/whatsapp/reference/whatsapp-business-phone-number/message-api)
  — send payloads, `recipient`, `biz_opaque_callback_data`, status behavior.
- [WhatsApp Cloud API pricing](https://developers.facebook.com/documentation/business-messaging/whatsapp/pricing)
  — Meta charges for conversations/messages; WaMux does not bill.
- [Graph API changelog](https://developers.facebook.com/docs/graph-api/changelog)
  — version lifecycle and breaking changes; confirm the pinned version.

### Reference implementations

These are third-party and not Meta-endorsed. Use them for patterns, fixtures
and coverage checks; do not make them the core dependency.

- [`wapikit/wapi.go`](https://github.com/wapikit/wapi.go) — Apache-2.0,
  `v0.8.2`. Particularly useful:
  - [`manager/webhook_manager.go`](https://github.com/wapikit/wapi.go/blob/main/manager/webhook_manager.go)
    — signature verification, partial-change skipping, webhook field coverage.
  - [`pkg/client/client.go`](https://github.com/wapikit/wapi.go/blob/main/pkg/client/client.go)
    — app secret vs verify token separation, custom HTTP client/base URL.
  - [`pkg/components/template_message.go`](https://github.com/wapikit/wapi.go/blob/main/pkg/components/template_message.go)
    — template component/parameter types.
  - [`pkg/messaging/client.go`](https://github.com/wapikit/wapi.go/blob/main/pkg/messaging/client.go)
    — phone registration/deregistration and typing-indicator patterns.
  - [`pkg/client/errors.go`](https://github.com/wapikit/wapi.go/blob/main/pkg/client/errors.go)
    — typed Graph errors.
- [`piusalfred/whatsapp`](https://github.com/piusalfred/whatsapp) — MIT,
  `v0.2.9`, marked alpha. Particularly useful:
  - [`webhooks/listener.go`](https://github.com/piusalfred/whatsapp/blob/main/webhooks/listener.go)
    — 4 MiB body cap, per-request config reader, signature validation, panic
    recovery, partial/fallback handling.
  - [`webhooks/field.go`](https://github.com/piusalfred/whatsapp/blob/main/webhooks/field.go)
    — exhaustive webhook field/event coverage.
  - [`pkg/http/core_client.go`](https://github.com/piusalfred/whatsapp/blob/main/pkg/http/core_client.go)
    — body/header limits, timeouts, interceptors and connection reuse.
  - [`groups/groups.go`](https://github.com/piusalfred/whatsapp/blob/main/groups/groups.go)
    — Cloud Groups API coverage and OBA constraints.
- [`jjuanrivvera/waba-cli`](https://github.com/jjuanrivvera/waba-cli) — MIT,
  CLI not an SDK. Particularly useful:
  - [`internal/api/retry.go`](https://github.com/jjuanrivvera/waba-cli/blob/main/internal/api/retry.go)
    — only idempotent methods are retried; POST sends are deliberately never
    auto-retried; `Retry-After` handles delta-seconds and HTTP-date forms.
  - [`internal/api/ratelimit.go`](https://github.com/jjuanrivvera/waba-cli/blob/main/internal/api/ratelimit.go)
    — adaptive limiter, 90% usage slowdown, 429 halving and gradual recovery.
  - [`internal/api/errors.go`](https://github.com/jjuanrivvera/waba-cli/blob/main/internal/api/errors.go)
    — full Graph error envelope, `fbtrace_id` and actionable hints.
  - [`api-manifest.json`](https://github.com/jjuanrivvera/waba-cli/blob/main/api-manifest.json)
    — 105-operation coverage manifest and Graph version reference.
  - [`commands/groups.go`](https://github.com/jjuanrivvera/waba-cli/blob/main/commands/groups.go)
    — group eligibility, participant limit and supported message types.
  - [`commands/doctor.go`](https://github.com/jjuanrivvera/waba-cli/blob/main/commands/doctor.go)
    — diagnostics layout for token, permissions and account health.

### What each source changed in this plan

| Source | Change |
|---|---|
| Meta BSUID docs | Added `cloud_contacts`, `contact_id`, `recipient` sends, BSUID webhook fields and `131062` |
| Meta Groups/Calling/Flows docs | Corrected capability matrix; deferred implementation without claiming Cloud lacks the products |
| Meta phone registration docs | Added `register`/`deregister`, PIN handling and `133010` |
| `wapi.go` | Webhook partial-failure handling, template parameter types, phone registration, typing indicator, typed errors |
| `piusalfred/whatsapp` | 4 MiB webhook cap, per-request/multi-app config, panic recovery, webhook field coverage, HTTP body/header limits |
| `waba-cli` | No POST auto-retry, adaptive rate limiter, full error hints, pagination safety, diagnostics and coverage manifest |
| Evolution API / Reforged | Payload-shape reference only; not a security, retry or tenancy model |
