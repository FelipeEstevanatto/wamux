# WABA / WhatsApp Cloud API — implementation plan

**Status:** proposed; no implementation started.
**Target:** WaMux `1.1.0` (or the next minor release).
**Companion design:** [`../spikes/waba-cloud-api.md`](../spikes/waba-cloud-api.md).
**Baseline:** WaMux `1.0.1` + 29 commits, latest migration `0017`.
**Reference implementation:** Evolution API `2.3.7` and Reforged `3.3.0` (functional reference only; not a security or reliability template).

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
2. Signed `GET/POST /webhook/cloud`.
3. Inbound text and status events.
4. Outbound text with idempotency and message persistence.
5. Cloud status, disconnect, delete and capability reporting.

Then media, templates, the 24-hour window, interactive messages and manager
onboarding.

**Estimate:** 6–8 engineer-weeks for production Phases 1–3. A demo/text-only
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

### Non-goals

- Feature parity with Baileys or with Reforged.
- Groups, communities, newsletters, status/stories, albums, polls, carousels,
  product catalogs, calls, presence/typing parity or chat-history sync.
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

### 3.3 Cloud API has no chat history or contact-list API

WaMux can store messages received by webhook, but it cannot import a phone's
history or contact list. The manager must not show a “sync history” action for
Cloud instances.

### 3.4 Cloud API does not accept a client-generated message ID

The Graph API returns a `wamid`. WaMux's `id` field cannot be used as the
WhatsApp message ID. For Cloud, `id` is only a WaMux/idempotency hint and may be
sent as `biz_opaque_callback_data`. The response ID is the Graph `wamid`.

### 3.5 A token is not necessarily permanent

System-user tokens can be long-lived, but user tokens expire. Store
`token_expires_at` when known, surface verification failures, and provide a
rotation endpoint. Do not build automatic refresh unless a refresh token and a
testable flow are available.

---

## 4. Scope matrix

| Capability | Phase 1 | Phase 2 | Phase 3 | Notes |
|---|---:|---:|---:|---|
| Cloud instance + credentials | ✅ | | | encrypted, write-only |
| Signed webhook GET/POST | ✅ | | | app-level verify token |
| Inbound text | ✅ | | | persisted + event |
| Inbound statuses | ✅ | | | sent/delivered/read/failed |
| Outbound text | ✅ | | | `wamid`, status mapping |
| Instance status/disconnect/delete | ✅ | | | no QR/socket |
| Cloud media upload | | ✅ | | local/multipart and URL |
| Cloud media download | | ✅ | | queued, S3/local |
| Templates CRUD/send | | ✅ | | Graph + local cache |
| 24-hour window | | ✅ | | pre-check + Meta error |
| Buttons/lists | | | ✅ | subset of WaMux buttons |
| Location/contact/reaction | | | ✅ | Cloud payload mapping |
| Sticker | | | ✅ | if Meta account supports it |
| Rate-limit/error mapping | ✅ basic | ✅ | ✅ full | 429, retry, typed errors |
| Capability discovery | ✅ | ✅ | ✅ | API + manager |
| Manager onboarding | | | ✅ | create/settings/templates |
| Embedded Signup | | | | Phase 4 |
| Groups/albums/polls/newsletters/status | ❌ | ❌ | ❌ | explicit unsupported |
| Chat history/contact sync | ❌ | ❌ | ❌ | Cloud has no API |
| Presence/typing parity | ❌ | ❌ | ❌ | not available |

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
    Text        bool `json:"text"`
    Media       bool `json:"media"`
    Templates   bool `json:"templates"`
    Interactive bool `json:"interactive"`
    Location    bool `json:"location"`
    Contacts    bool `json:"contacts"`
    Reactions   bool `json:"reactions"`
    Groups      bool `json:"groups"`
    Albums      bool `json:"albums"`
    Polls       bool `json:"polls"`
    Newsletters bool `json:"newsletters"`
    Status      bool `json:"status"`
    Presence    bool `json:"presence"`
    History     bool `json:"history"`
}

var ErrUnsupported = errors.New("unsupported_on_channel")

type UnsupportedError struct {
    Channel Kind
    Op      string
}

func (e *UnsupportedError) Error() string { ... }
func (e *UnsupportedError) Unwrap() error { return ErrUnsupported }
```

Neutral DTOs (minimal shape; add fields as operations are implemented):

```go
type TextMessage struct {
    To         string
    Body       string
    PreviewURL bool
    ReplyTo    string
}

type MediaMessage struct {
    To       string
    Kind     string // image/video/audio/document/sticker
    URL      string
    Data     []byte
    Filename string
    MIME     string
    Caption  string
    ReplyTo  string
}

type Result struct {
    ID        string
    Timestamp time.Time
    Channel   Kind
    Raw       map[string]any // optional, never serialized directly
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
  - `Info.Chat` = recipient JID
  - `Info.Sender` = instance JID (or the Cloud phone number)
  - `Info.Timestamp` = send time
  - `Message` stays nil for non-media, or a minimal map for media metadata
  - add `Channel` and `Cloud` fields to the response envelope (additive)

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
| Connect | start client/QR | verify token/phone, subscribe app; no client |
| Status | socket state | cached verification + webhook state |
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
                  message row             status event             template update
                        │                       │                       │
                  media job queue         cloud_status_events      cloud_templates
                        │                       │                       │
                        └───────────────────────┴───────────▶ EventSink
                                                              (Message/Receipt)
```

Outbound:

```
/send/* ──▶ SendRouter ──▶ CloudSender ──▶ Graph /messages
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
CREATE INDEX idx_messages_instance_channel ON messages (instance_id, channel);
```

Use lowercase `baileys` / `cloud` for the row-level channel. The instance
integration remains the canonical `WHATSAPP-BAILEYS` / `WHATSAPP-BUSINESS`
value.

### 6.3 `cloud_apps`

```sql
CREATE TABLE IF NOT EXISTS cloud_apps (
    app_id            TEXT PRIMARY KEY,
    app_secret_enc    TEXT NOT NULL,
    verify_token_hash TEXT NOT NULL,
    graph_version     TEXT NOT NULL DEFAULT 'v23.0',
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
    chat_jid        TEXT NOT NULL,
    last_inbound_at TIMESTAMPTZ NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    last_message_id TEXT,
    PRIMARY KEY (instance_id, chat_jid)
);

CREATE INDEX IF NOT EXISTS idx_cloud_windows_expiry
    ON cloud_chat_windows (expires_at);
```

Use the Meta message timestamp, not the webhook receipt time, so a delayed
webhook does not extend the window incorrectly. Keep rows; a later inbound
upsert refreshes the window. Cleanup is optional.

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
| `0020_cloud_api_credentials` | per-instance WABA/phone/token |
| `0021_cloud_templates` | template cache/lifecycle |
| `0022_cloud_chat_windows` | 24-hour window state |
| `0023_cloud_webhook_events` | dedupe/replay claims |
| `0024_cloud_media_jobs` | durable media download queue |
| `0025_messages_channel` | message channel column + index |
| `0026_cloud_status_events` | status/error history |

Postgres steps must use `IF NOT EXISTS` where supported. SQLite cannot add a
column with `IF NOT EXISTS`; AutoMigrate runs first, so SQLite steps should only
backfill and create indexes/tables that AutoMigrate did not create. Test both
drivers.

### 6.12 Retention

- `cloud_webhook_events`: 30 days.
- `cloud_status_events`: 90 days.
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
| `CLOUD_API_GRAPH_VERSION` | `v23.0` | Default for app rows; configurable |
| `CLOUD_API_APP_ID` | empty | Initial Meta app id |
| `CLOUD_API_APP_SECRET` | empty | Initial app secret (encrypted at boot/seed) |
| `CLOUD_API_VERIFY_TOKEN` | empty | Initial webhook verify token |
| `CLOUD_API_HTTP_TIMEOUT_SECONDS` | `30` | Graph request timeout |
| `CLOUD_API_MEDIA_TIMEOUT_SECONDS` | `120` | Media transfer timeout |
| `CLOUD_API_MAX_WEBHOOK_BYTES` | `2097152` | Webhook body cap |
| `CLOUD_API_MAX_MEDIA_BYTES` | `104857600` | Inbound media cap |
| `CLOUD_API_MEDIA_WORKERS` | `2` | Media download worker count |
| `CLOUD_API_WINDOW_HOURS` | `24` | Window length; Meta policy default |
| `CLOUD_API_ENFORCE_WINDOW` | `true` | Default for new credentials |
| `CLOUD_API_RETRY_MAX` | `3` | Graph retry attempts |

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
  "graphVersion": "v23.0"
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
  "displayPhoneNumber": "+5511999999999",
  "verifiedName": "Acme Support",
  "qualityRating": "GREEN",
  "checkedAt": "2026-11-22T14:00:00Z"
}
```

`POST /instance/connect` for Cloud performs verification and
`POST /{waba-id}/subscribed_apps`; it returns the normal instance object plus
`cloud`. If subscription fails, keep credentials, set `connected` only after
verification, and return a clear error with `subscription: false`.

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
    "windowHours": 24,
    "capabilities": {
      "text": true,
      "media": true,
      "templates": true,
      "interactive": true,
      "groups": false,
      "albums": false,
      "polls": false,
      "newsletters": false,
      "status": false,
      "presence": false,
      "history": false
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
  "text": "Hello from WaMux",
  "quoted": {"messageId": "wamid..."}
}
```

- `number` is normalized to digits; Cloud `to` is E.164 without `+`.
- `quoted.messageId` maps to `context.message_id`; it must be a `wamid`.
- `mentionAll` / `mentionedJid` are unsupported on Cloud: return
  `422 unsupported_on_channel` rather than silently dropping mentions.
- `delay` is honored as a bounded local sleep; Cloud has no typing indicator.
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
      "wamid": "wamid.HBgM..."
    }
  }
}
```

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
- If the local cache says the template is rejected/disabled, return
  `422 cloud_template_error` before calling Graph.
- Template sends bypass the 24-hour window.

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
`DISABLED`, etc. Surface `rejected_reason` and `quality_score`.

### 8.9 Webhook

`GET /webhook/cloud?hub.mode=subscribe&hub.verify_token=...&hub.challenge=...`

- Look up `verify_token_hash`; if matched, return `hub.challenge` as plain text.
- Otherwise `403`.
- No auth middleware; this is Meta's callback.

`POST /webhook/cloud`

- Require `X-Hub-Signature-256: sha256=<hex>`.
- Verify HMAC-SHA256 over the raw body using the app secret selected by the
  event's WABA/phone number. If the WABA is unknown, try all configured app
  secrets (bounded); otherwise `401`.
- Body cap `CLOUD_API_MAX_WEBHOOK_BYTES`; oversized `413`.
- Valid signed event: return 200 with `{"status":"ok"}`.
- Invalid/missing signature: `401`.
- Unknown phone number but valid signature: 200 and ignore (avoid Meta retry
  storms).
- Processing failure: 500 after releasing the dedupe claim.

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
| Unsupported on channel | 422 | `unsupported_on_channel` |
| Window closed | 422 | `cloud_window_expired` |
| Template rejected/not found | 422 | `cloud_template_error` |
| Undeliverable | 422 | `cloud_undeliverable` |
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
    window.go          24-hour window logic
    status.go          status mapping
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
    SubscribeApp(ctx context.Context, wabaID string) error
    ListTemplates(ctx context.Context, wabaID string) ([]Template, error)
    CreateTemplate(ctx context.Context, wabaID string, t Template) (Template, error)
    UpdateTemplate(ctx context.Context, templateID string, t Template) (Template, error)
    DeleteTemplate(ctx context.Context, wabaID, name string) error
}
```

Implementation rules:

- Use a dedicated `http.Client`; never `http.DefaultClient`.
- `Authorization: Bearer <access token>`.
- Base URL and version from config/app row; no hardcoded Graph version in
  payload code.
- Request timeout 30 s (media 120 s), bounded response body (1 MiB JSON,
  streamed media with a byte cap).
- Retry 429, 500, 502, 503, 504 with jitter; honor `Retry-After`. Do not retry
  other 4xx.
- Parse `X-Business-Use-Case-Usage`; back off when call/cpu usage is near the
  limit.
- Capture `X-FB-Trace-ID`/`fbtrace_id` for support; never log the token.
- Set a `WaMux/<version>` user agent.
- Treat a 200 with `error` as an error.
- Use `context.Context` end to end.

### 9.3 Payload builders

```
POST /{version}/{phone-number-id}/messages
{
  "messaging_product": "whatsapp",
  "recipient_type": "individual",
  "to": "<digits>",
  "type": "<type>",
  ...
}
```

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

Unsupported on Cloud:

- `/send/album`
- `/send/poll`
- `/send/carousel` (unless implemented as a template in a later phase)
- `/send/event`
- `/send/product`
- `/send/status/text`, `/send/status/media`
- groups, communities, newsletters, presence, calls, history

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
| `131047` | `cloud_window_expired` | no (template required) |
| `131026` | `cloud_undeliverable` | no |
| `131051` | `unsupported_on_channel` | no |
| `132000` | `cloud_template_error` | no |
| `132001` | `cloud_template_error` | no |
| `132005`/`132007`/`132012` | `cloud_template_error` | no |
| `80007`/`130429`/`131048` | `cloud_rate_limited` | yes, after backoff |
| `368` | `cloud_temporarily_blocked` | after `Retry-After` |
| `131042` | `cloud_payment_issue` | no |
| unknown | `cloud_upstream_error` | 5xx only |

Always include the Meta trace id in structured logs, not in the public error
unless it helps support.

---

## 10. Inbound webhook processing

### 10.1 Signature verification

```
rawBody, err := readLimited(c.Request.Body, CLOUD_API_MAX_WEBHOOK_BYTES)
candidateApps := resolveAppsFromBody(rawBody) // WABA id / phone number id
for each app secret:
    expected := "sha256=" + hex(HMAC-SHA256(secret, rawBody))
    if hmac.Equal([]byte(expected), []byte(header)) { verified = true }
if !verified { 401 }
```

Important:

- Verify the **raw bytes** before unmarshalling.
- Use `hmac.Equal`, not string equality.
- Header is lowercase `x-hub-signature-256` after Gin normalization; parse
  exactly one `sha256=` prefix.
- Do not log the body on failure.
- If the body contains an unknown WABA, try all configured app secrets (bounded
  to 10) before rejecting; this handles events for numbers not yet registered.
- The verify token is **not** accepted as POST authentication.

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
            "contacts": [{"profile": {"name": "Ana"}, "wa_id": "5511888888888"}],
            "messages": [
              {
                "from": "5511888888888",
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
2. Iterate entries/changes; ignore unknown `field` values safely.
3. Resolve credentials by `value.metadata.phone_number_id`.
4. For each message:
   - Build `event_key`; claim in `cloud_webhook_events`.
   - Normalize `wa_id` to `<digits>@s.whatsapp.net`.
   - Map `type` to the internal message type.
   - Persist a `message_model.Message` with `channel="cloud"`,
     `status="Received"`, `is_from_me=false`, text/caption/media metadata.
   - Upsert `cloud_chat_windows` using the Meta timestamp.
   - If media, insert `cloud_media_jobs` (ignore duplicate).
   - Publish a `Message` event through the shared `EventSink`.
   - Update `lastWebhookAt` for the instance.
5. For each status:
   - Claim dedupe key.
   - Insert `cloud_status_events` (unique key) and update `messages.status`
     using the rank rules.
   - Publish a `Receipt` event with `status` mapped to `Sent`/`Delivered`/
     `Read`/`Failed`.
6. For `message_template_status_update`:
   - Locate `cloud_templates` by `message_template_id` (fallback name +
     language).
   - Update status/rejected reason.
   - Publish a new `Template` event (add `TEMPLATE` to the internal event types
     as an additive change).
7. Return 200 only after all non-media work committed.

### 10.4 Event mapping

Use the existing event names so consumers do not need a new API:

- inbound message → `Message`
- status → `Receipt`
- credentials verified → `Connected`
- template status → `Template` (new; document it)
- template quality/account updates → `Template` or log-only

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
`key.remoteJid`, `key.fromMe`, `key.id`, `pushName`, `message`,
`messageType`, `messageTimestamp`, `source`. For media, include the stored
`mediaUrl`/`mediaMimetype`, not the expiring Meta URL.

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
| `interactive` | `interactive_reply` | `button_reply` or `list_reply` |
| `order` | `order` | store raw metadata; no product sync |
| `system` | `system` | store/log only |
| unknown | `unknown` | persist raw type, emit event, never panic |

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
  - Set `Connected=false`, `AutoReconnect=false` until verified.
  - Never write the Meta token to `instances.token`.
- If Baileys: current behavior, `integration` defaults; no other changes.

### 11.2 Connect

- Load credentials; missing → `400 cloud_credentials_missing`.
- Decrypt token; if decryption fails → `500` with a config/rotation message.
- Call `GET /{phone-number-id}?fields=display_phone_number,verified_name,quality_rating,status`.
- Call `POST /{waba-id}/subscribed_apps`.
- Update cached display name/phone, `last_verified_at`, `last_verify_error`.
- Set `Connected=true` on successful verification; store `subscribed` status
  separately.
- No QR, no `whatsmeow.Client`, no ownership lease.
- On failure, keep the instance and credentials; return the error.

### 11.3 Status

- Do not call Graph on every poll.
- Return cached verification, `lastWebhookAt`, subscription flag and
  capabilities.
- If credentials are missing, return `connected=false` plus a clear reason.

### 11.4 Disconnect

- Set `Connected=false`, `AutoReconnect=false`.
- Keep credentials/templates/windows.
- Do not unsubscribe the app; other instances may share it.

### 11.5 Delete

- Delete child rows (`cloud_status_events`, `cloud_media_jobs`,
  `cloud_webhook_events`, `cloud_chat_windows`, `cloud_templates`,
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
| `GET /chat/history`, `GET /chat/chats` | Works from `messages`; no contact sync |
| `GET /chat/contacts` / contact resolution | Phone/`wa_id` fallback only; no address book |
| `POST /chat/read` / `POST /message/read` | Optionally Graph “mark as read”; otherwise `unsupported_on_channel` |
| `GET /message/*` history | Works from DB |
| `POST /message/react` | Supported in Phase 3 |
| `GET /user/*` (check, avatar, privacy, block) | `unsupported_on_channel` |
| `GET /group/*`, `/community/*`, `/newsletter/*` | `unsupported_on_channel` |
| `GET /instance/overview` | Return zero/null profile fields; do not call whatsmeow |
| `GET /instance/qr`, `POST /pair`, `POST /logout` | `unsupported_on_channel` |
| `GET /server/health` | Include Cloud instance counts and webhook/media state |

Implement this as a small `ChannelCapabilities` check in handlers, not as
nil-pointer defensive code. A Cloud instance must never reach
`clientPointer.Get()` in a path that assumes a connected Baileys client.

### 11.10 Typebot/chatbot dispatch

After persisting an inbound Cloud text message, dispatch to the Typebot
processor asynchronously, like Baileys does. The Cloud service can depend on the
existing `TypebotProcessor` interface (not the Typebot service package) to avoid
a cycle. If no processor is configured, skip. Test that a Typebot reply goes
back through `SendRouter` and is sent by the Cloud channel.

---

## 12. Security

- **Secrets:** AES-256-GCM via `pkg/tokencrypt`; app secret and access token
  never appear in API responses, logs, audit detail or errors.
- **Webhook:** HMAC-SHA256 over raw bytes, constant-time compare, body cap,
  signature-before-side-effects.
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
- Show a “Verify connection” action after creation.

### 14.2 Instances list

- Cloud badge and display phone number.
- Replace “Connect/QR” with “Verify” for Cloud.
- Hide QR/pair/logout actions; show rotate credentials/delete.
- Status panel: verified, subscribed, last webhook, capabilities.

### 14.3 Instance settings

- Cloud credentials form (write-only token, app/WABA/phone/business).
- Verify button, subscription status, webhook URL copy field.
- 24-hour window toggle/status.
- Templates tab: list status, create/edit/delete, sync.
- Danger zone: rotate credentials, delete instance.

### 14.4 Conversations/send UI

- Capability-aware buttons: hide/disable groups, albums, polls, newsletters,
  status, presence.
- Template picker; if the window is closed, require a template.
- Show `Failed` status with Meta error code/message from
  `cloud_status_events`.
- Show `Template` badge and `Cloud` channel badge.
- Do not offer “sync history” or “contacts” for Cloud.

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
| `pkg/cloud/credentials.go` | **new**: app/credential service |
| `pkg/cloud/window.go` | **new**: 24-hour window |
| `pkg/cloud/status.go` | **new**: status mapping |
| `pkg/cloud/worker.go` | **new**: media download worker |
| `pkg/cloud/cleanup.go` | **new**: retention |
| `pkg/cloud/*_test.go` | **new**: unit/integration tests |
| `pkg/instance/model/instance_model.go` | add `Integration` |
| `pkg/instance/service/instance_service.go` | Cloud branches in create/connect/status/delete/logout |
| `pkg/instance/service/cloud_service.go` | **new**: credentials/verify/subscription |
| `pkg/instance/handler/instance_handler.go` | Cloud handlers, error mapping, status |
| `pkg/instance/repository/*` | create with Cloud credentials transaction |
| `pkg/sendMessage/service/send_service.go` | extract/reuse outbound persistence; add channel field |
| `pkg/sendMessage/service/router.go` | **new**: `SendService` router |
| `pkg/sendMessage/service/template_structs.go` | **new**: `TemplateStruct` |
| `pkg/sendMessage/handler/send_handler.go` | `/send/template`, error mapping |
| `pkg/sendMessage/handler/*` | pass channel capability errors through |
| `pkg/routes/routes.go` | public webhook; Cloud/template routes |
| `pkg/migrations/migrations.go` | migrations 0018–0026 |
| `pkg/message/model/message_model.go` | `Channel` column |
| `pkg/message/repository/message_repository.go` | `Failed` rank, channel/error handling |
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
- Swagger annotations and generated docs
- Update the WABA spike/design links
- `README` feature matrix

---

## 16. Work plan and estimates

Estimates assume one engineer familiar with WaMux, including tests and review
time. Add 20–30% for Meta app review/ops wait time.

### Phase 0 — prerequisites (2–3 days)

- Meta Business Portfolio, WABA, test phone number, system-user token.
- Meta app product setup, app secret, webhook verify token.
- Public HTTPS callback URL/tunnel for staging.
- Permissions: `whatsapp_business_messaging`,
  `whatsapp_business_management`.
- Save sanitized webhook fixtures (text/status/media/template).
- Confirm the Graph version and current media limits.
- Decide single-app vs multi-app deployment.

**Exit:** a `curl` to Graph returns the test phone number; a webhook reaches a
stub endpoint with a valid signature.

### Phase 1 — channel skeleton, credentials, signed webhook, text (12–15 days)

| ID | Task | Est. |
|---|---|---:|
| P1.1 | `instances.integration`, `messages.channel`, models/repositories | 2 d |
| P1.2 | migration steps + SQLite/Postgres tests | 1 d |
| P1.3 | `cloud_apps`/credentials service with `tokencrypt`; redaction | 2 d |
| P1.4 | Graph client core: auth, timeout, retries, errors | 2 d |
| P1.5 | `SendRouter` + Cloud text send + persistence/event | 3 d |
| P1.6 | webhook GET verification + POST signature + routing | 2 d |
| P1.7 | inbound text/status processing + dedupe | 3 d |
| P1.8 | Cloud create/connect/status/disconnect/delete | 2 d |
| P1.9 | handler error mapping, Swagger, audit | 1 d |
| P1.10 | tests, docs, manager minimal create/status | 3 d |

**Exit:** a Cloud instance sends and receives text; statuses update; unsigned
webhooks are rejected; existing Baileys tests still pass.

### Phase 2 — media, templates, window (12–15 days)

| ID | Task | Est. |
|---|---|---:|
| P2.1 | media upload (multipart/URL), limits, local/S3 | 3 d |
| P2.2 | inbound media job queue/worker, SHA/size checks | 3 d |
| P2.3 | template CRUD + local cache + sync | 3 d |
| P2.4 | `/send/template` + component validation | 2 d |
| P2.5 | 24-hour window tracking and pre-check | 2 d |
| P2.6 | manager templates/window/media UI | 3 d |
| P2.7 | tests/fixtures/docs | 2 d |

**Exit:** image/video/document/audio round-trip; templates can be created,
synced and sent; free-form outside the window returns a clear error; template
status updates appear.

### Phase 3 — interactive, operations, hardening (7–10 days)

| ID | Task | Est. |
|---|---|---:|
| P3.1 | buttons/lists/location/contact/reaction mapping | 3 d |
| P3.2 | Graph error/rate-limit/backoff hardening | 2 d |
| P3.3 | metrics, health, audit, log redaction | 2 d |
| P3.4 | capability matrix + manager gating | 2 d |
| P3.5 | load/race/fuzz tests, docs, release notes | 2 d |

**Exit:** production checklist and acceptance criteria pass; load and security
tests pass.

### Phase 4 — Embedded Signup/token rotation (10–15 days, optional)

- Facebook Login for Business, code exchange, shared WABA discovery.
- App review, privacy/terms.
- Token rotation UI and expiry reminders.
- Multi-app onboarding.

Do not start Phase 4 until manual onboarding has real usage.

### Total

- **Phases 1–3:** 6–8 engineer-weeks.
- **Phase 4:** +2–3 weeks.
- **Functional demo only:** 2–3 weeks (not production-ready).

---

## 17. Test plan

### Unit

- `pkg/cloud/payload_test.go`: every supported type, limits, quoted reply,
  `wamid`, unsupported operations.
- `pkg/cloud/signature_test.go`: valid, invalid, missing, malformed prefix,
  multiple app secrets, body mutation, unknown WABA.
- `pkg/cloud/webhook_test.go`: text, media, statuses, failed errors, template
  status, unknown phone, duplicate, malformed JSON, oversized, nil arrays.
- `pkg/cloud/errors_test.go`: Graph code mapping, retry decisions,
  `Retry-After`.
- `pkg/cloud/window_test.go`: open/closed, timestamp, template bypass, Meta
  `131047` mapping.
- `pkg/cloud/templates_test.go`: component payloads, status mapping, cache
  updates.
- `pkg/channel/router_test.go`: routing by integration, unsupported errors,
  no Baileys call for Cloud.
- `pkg/instance/service/cloud_test.go`: create transaction, rotate, verify,
  status, delete, no secret in JSON.
- `pkg/message/repository/status_order_test.go`: add `Failed` transitions.
- `pkg/migrations/*`: apply on SQLite and Postgres; seeded Baileys rows keep
  working.
- Manager Vitest: create Cloud, credential redaction, capability gating,
  window-closed send, template picker.

### Integration/HTTP

- `httptest` Graph fake for send, upload, download, verify, subscribe,
  templates, 429, 5xx, timeout, malformed response.
- Webhook fixture → instance lookup → message row → fake `EventSink` receives
  one `Message`.
- Status fixture → row status moves forward only; failed errors stored.
- Duplicate delivery → one row/event.
- Bad signature → 401, no DB writes.
- Unknown phone number + valid signature → 200, no DB writes.
- Media job → storage → message row URL; retry after transient failure.
- Multi-tenant: two Cloud instances; event routed to the right one; instance A
  token cannot read B templates/status/messages.
- Secret scan: DB rows/API JSON/log buffer contain no access token/app secret.

### Fuzz/race

- Fuzz webhook JSON and signature header parsers.
- Fuzz template `components`.
- `go test -race -shuffle=on ./...`.
- Webhook burst (1k events), send concurrency, media workers, no goroutine
  leaks.

### Manual staging

- Meta test number; send text/template to a verified recipient.
- Verify webhook in Meta dashboard.
- Rotate token; confirm old token fails and new one works.
- Trigger a rejected template and confirm UI/status.
- Confirm no Graph call at boot for disconnected Cloud instances.

---

## 18. Acceptance criteria

A release is acceptable only if all are true:

1. Existing Baileys instances, sends, QR, history and tests are unchanged;
   `integration` defaults to `WHATSAPP-BAILEYS`.
2. Migrations apply cleanly on Postgres and SQLite, including a seeded
   pre-Cloud database with existing messages/tokens.
3. A Cloud instance can be created with encrypted credentials; API responses
   and logs never reveal the Meta token or app secret.
4. `GET /webhook/cloud` echoes the challenge for a valid verify token and
   rejects others.
5. `POST /webhook/cloud` rejects missing/invalid signatures with 401 and
   processes valid signed events.
6. Inbound text is persisted once, routed to the correct instance, opens the
   24-hour window and emits one `Message` event.
7. Outbound text returns a Graph `wamid`, stores a `Sent` row and emits one
   `SendMessage` event.
8. Status webhooks move `Sent → Delivered → Read` monotonically and store
   `Failed` errors without downgrading `Read`.
9. Media upload/download works for image, video, audio and document; size/type
   limits are enforced; expired/failed downloads are retryable.
10. Template create/list/send/sync works; status updates appear in the manager.
11. Free-form sends outside the window return 422 `cloud_window_expired`;
    template sends are allowed.
12. Groups/albums/polls/carousels/events/products/status/presence return 422
    `unsupported_on_channel`; capability discovery reports them false.
13. Cross-tenant access is impossible via API, webhook routing or template IDs.
14. Rate-limit/5xx responses produce backoff and typed errors; no retry storm.
15. Metrics, health, audit and structured logs cover Cloud operations.
16. Manager can create, verify, inspect, rotate and delete a Cloud instance,
    manage templates and respect capabilities.
17. Docs, Swagger and env reference are updated.
18. `gofmt`, `go vet`, `go test -race -shuffle=on ./...`, `golangci-lint`, and
    manager lint/build/test pass.

---

## 19. Rollout and rollback

### Rollout

1. Merge schema and code behind `CLOUD_API_ENABLED=false`.
2. Deploy to staging; enable the flag; create a Meta test instance.
3. Verify GET/POST webhook, text, statuses and media.
4. Enable in production for one internal instance.
5. Canary one real customer; monitor Graph errors, webhook failures, send
   latency and media backlog.
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
  test run.
- Record the Meta app review status and permissions in the ops runbook.

---

## 20. Risks and mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Meta app review/verification delays | launch slips | Start Phase 0 early; use test number in staging |
| Meta API version changes | breakage | configurable version, fixtures, contract tests |
| Webhook signature with multiple apps | event rejection | app/WABA mapping + bounded multi-secret verify; one app per deployment initially |
| Token expiry/rotation | sends fail | expiry field, verify endpoint, health metric, rotation UI |
| 24-hour window mismatch | rejected sends | pre-check + `131047` mapping + template fallback |
| Media ID expiry/size | missing attachments | job queue, immediate download, size caps, retries |
| Duplicate webhooks/retries | duplicate events | dedupe table + idempotent upserts + status rank |
| Duplicate sends on timeout | customer spam | Idempotency-Key + `biz_opaque_callback_data`; document Graph limitation |
| Rate limits | 429 storms | backoff, jitter, per-phone limiter, metrics |
| No groups/history | product mismatch | capability matrix, manager gating, docs |
| Secret leakage | severe | encryption, redaction, tests, no token logs |
| Multi-node send limits | over-limit | conservative limits; Phase 3 DB limiter or document single-node |
| DB growth | storage | retention for events/status/media jobs |
| Pricing/opt-in compliance | legal/cost | docs: Meta pricing, opt-in/opt-out are tenant responsibilities |

---

## 21. Documentation deliverables

- Cloud API guide: onboarding, Meta app setup, callback URL, verify token,
  permissions, sending, templates, window, unsupported features, troubleshooting.
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

1. **One Meta app per deployment** (recommended) vs multiple apps/tenants.
   The schema supports multiple; the UI starts with one.
2. Manual onboarding first; Embedded Signup later. **Yes.**
3. Unsupported feature status: **422 `unsupported_on_channel`**.
4. `/send/link`: **send as Cloud text with `preview_url`**, document the
   difference.
5. Sticker/product/carousel: **not in Phases 1–3**; revisit after real demand.
6. Multi-node rate limiting: **per-node limiter first**, document the limit;
   add a Postgres limiter only if a deployment needs horizontal Cloud sending.
7. Opt-out storage: **out of scope**; tenant handles STOP/opt-out policy.
8. Template edit semantics: pass through to Graph; local cache is not the
   source of truth for approval.
9. Media event timing: **publish `Message` after the file is stored**, with a
   `mediaStatus` field; add a retry endpoint for failures.
10. Feature flag default: **off** until staging passes.

---

## 23. Definition of done

- [ ] Plan approved and open decisions resolved.
- [ ] `integration` model + migrations merged.
- [ ] Encrypted Cloud credentials + audit merged.
- [ ] Graph client with fake-server tests merged.
- [ ] Signed webhook + inbound text/status merged.
- [ ] Cloud text send + persistence/events merged.
- [ ] Cloud lifecycle (create/connect/status/disconnect/delete) merged.
- [ ] Media + templates + window merged.
- [ ] Interactive subset + capability gating merged.
- [ ] Manager create/settings/templates/window merged.
- [ ] Metrics/health/audit merged.
- [ ] Docs/Swagger/env reference merged.
- [ ] Acceptance tests and full verification pass.
- [ ] Staging Meta test number verified.
- [ ] Rollout/rollback runbook reviewed.

---

## 24. Suggested commit sequence

Keep commits reviewable and each one test-backed:

1. `docs(cloud): add WABA Cloud API implementation plan`
2. `feat(cloud): add integration field and Cloud schema`
3. `feat(cloud): encrypt and manage Meta app/instance credentials`
4. `feat(cloud): add Graph API client`
5. `feat(cloud): verify and route signed webhooks`
6. `feat(cloud): receive text and status events`
7. `feat(send): route sends by instance integration`
8. `feat(cloud): send text through the Cloud channel`
9. `feat(cloud): add Cloud instance lifecycle`
10. `feat(cloud): add media upload/download`
11. `feat(cloud): add template management`
12. `feat(cloud): enforce the customer service window`
13. `feat(cloud): add interactive message mapping`
14. `feat(manager): add Cloud onboarding and settings`
15. `docs(cloud): add API, ops and security documentation`

Each commit should include `gofmt`, `go vet`, focused tests and, where UI is
touched, manager lint/build/test. Run the full race/shuffle suite before the
release candidate.
