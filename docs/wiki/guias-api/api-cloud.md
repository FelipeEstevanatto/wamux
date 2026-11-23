# WhatsApp Cloud API (WABA)

WaMux supports the official Meta WhatsApp Cloud API as a second channel. The
existing Baileys/WhatsApp Web channel remains the default; Cloud is selected
per instance with `integration: "WHATSAPP-BUSINESS"`.

## Prerequisites

- A Meta Business Portfolio and WhatsApp Business Account (WABA).
- A business phone number registered for Cloud API with its six-digit PIN.
- A Meta app with the WhatsApp product.
- An access token with `whatsapp_business_messaging` and
  `whatsapp_business_management`.
- A public HTTPS webhook URL. Meta cannot call `localhost`.

Configure the deployment:

```env
CLOUD_API_ENABLED=true
DATA_ENCRYPTION_KEY=<long-random-secret>
CLOUD_API_APP_ID=<meta-app-id>
CLOUD_API_APP_SECRET=<meta-app-secret>
CLOUD_API_VERIFY_TOKEN=<webhook-verify-token>
CLOUD_API_GRAPH_VERSION=v27.0
```

For multiple Meta apps use `/webhook/cloud/<app-id>` as the callback URL.

## Create and connect

```bash
curl -X POST "$WAMUX/instance/create" \
  -H "apikey: $GLOBAL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "support-cloud",
    "token": "instance-token",
    "integration": "WHATSAPP-BUSINESS",
    "cloud": {
      "appId": "<meta-app-id>",
      "wabaId": "<waba-id>",
      "phoneNumberId": "<phone-number-id>",
      "businessId": "<business-id>",
      "accessToken": "<meta-access-token>"
    }
  }'
```

Then:

```bash
curl -X POST "$WAMUX/instance/connect" \
  -H "apikey: instance-token" -H "Content-Type: application/json" -d '{}'

curl -X POST "$WAMUX/instance/cloud/register" \
  -H "apikey: instance-token" -H "Content-Type: application/json" \
  -d '{"pin":"123456"}'
```

The PIN is sent to Meta and never stored by WaMux. Registration is limited by
Meta to 10 attempts per number per 72 hours.

## Webhook

Configure this callback in the Meta app:

```text
https://your-wamux.example.com/webhook/cloud
```

Subscribe to the `messages` field (and template/account fields you need).
WaMux verifies `X-Hub-Signature-256` with the Meta app secret; the verify token
is only used for Meta's GET challenge.

## Sending

Existing send routes work for phone-number recipients:

```bash
curl -X POST "$WAMUX/send/text" \
  -H "apikey: instance-token" -H "Content-Type: application/json" \
  -d '{"number":"5511999999999","text":"Hello"}'
```

BSUID recipients use the new `recipient` field:

```json
{
  "recipient": "US.13491208655302741918",
  "recipientType": "individual",
  "text": "Hello"
}
```

The returned `data.Info.ID` is the Graph `wamid`.

Templates:

```bash
curl -X POST "$WAMUX/send/template" \
  -H "apikey: instance-token" -H "Content-Type: application/json" \
  -d '{
    "number": "5511999999999",
    "name": "order_update",
    "language": "en_US",
    "components": [
      {"type":"body","parameters":[{"type":"text","text":"12345"}]}
    ]
  }'
```

## Identity continuity and contacts

Meta is migrating from phone-only identities to BSUIDs. WaMux keeps a
`cloud_contact_identities` history and merges an old BSUID/new BSUID transition
into one canonical contact in a single transaction. Messages keep their original
`message_id` and are only re-pointed at the canonical `contact_id`; the 24-hour
window keeps the latest expiry.

```text
GET  /cloud/contacts/:contactId
GET  /cloud/identity-events
POST /cloud/contacts/:contactId/reconcile
```

The reconcile endpoint accepts `waId`, `oldBsuid`, `newBsuid` and `parentBsuid`
for operator repair. `user_id_update` webhooks are applied idempotently.

Account/template webhooks are available for the manager:

```text
GET /cloud/account-events?field=&limit=&offset=
GET /cloud/account-events/:id
```

Unknown webhook fields return `200`, are stored with `event_type=unknown`, and
never make Meta retry forever.

## 24-hour window

A user message opens a 24-hour customer-service window. Free-form messages
outside it return `422 cloud_window_expired`; an approved template is required.
This is Meta policy, not a WaMux preference.

## BSUID and usernames

Meta now sends BSUIDs (`user_id`, `from_user_id`, `recipient_user_id`) and may
omit phone numbers. WaMux stores a contact identity table and accepts BSUID
recipients. Some authentication templates still require a phone number; Meta
returns `131062` when a BSUID is not allowed.

## Interactive and catalog payloads

Cloud supports the same neutral send routes where the payload exists:

| Route | Cloud payload |
|---|---|
| `POST /send/button` with one URL button | `interactive.type=cta_url` |
| `POST /send/product` | `interactive.type=product` |
| `POST /send/catalog` | `interactive.type=catalog_message` |
| `POST /send/contact-request` | `interactive.type=request_contact_info` |

Product routes need the catalog id and a product retailer id. Product-list
payloads use catalog sections; the Cloud adapter never fabricates a template or
catalog. Free-form interactive/product messages obey the 24-hour window; approved
templates are required outside it.

## Business Management

Instance-scoped routes use the instance's own WABA/phone and never expose the
access token:

```text
GET  /cloud/business/phone-numbers
GET  /cloud/business/phone-numbers/:id
POST /cloud/business/phone-numbers/:id/register
POST /cloud/business/phone-numbers/:id/deregister
POST /cloud/business/phone-numbers/:id/request-code
POST /cloud/business/phone-numbers/:id/verify-code
GET  /cloud/business/waba
GET  /cloud/business/wabas?scope=owned|client
GET  /cloud/business/analytics?start=&end=&granularity=DAY
GET  /cloud/business/catalogs
POST /cloud/business/catalogs
GET  /cloud/business/catalogs/:id/products
POST /cloud/business/catalogs/:id/products
```

Analytics responses are cached for two minutes in-process and are not persisted.
Pagination follows opaque Graph cursors only.

## Embedded Signup

Set `CLOUD_API_EMBEDDED_CONFIG_ID` and `CLOUD_API_EMBEDDED_REDIRECT_URI`, then use
the manager's **Connect with Meta** action or:

```text
POST /cloud/embedded/start
GET  /cloud/embedded/callback?code=...&state=...
GET  /cloud/embedded/assets?state=...
POST /cloud/embedded/select
```

The state is random, instance-scoped and expires after ten minutes. The callback
exchanges the code server-side, discovers WABAs/phone numbers, and stores the
token encrypted only when the operator selects an asset. Tokens never enter a URL
or logs.

## CLI

Build the separate operator binary:

```bash
go build ./cmd/wamuxctl
```

It talks only to the HTTP API and reads `WAMUX_URL`, `WAMUX_API_KEY` and
`WAMUX_INSTANCE_TOKEN` (flags win):

```bash
wamuxctl cloud doctor --instance-token "$INSTANCE_TOKEN"
wamuxctl cloud send text --instance-token "$INSTANCE_TOKEN" --to 5511999999999 --text "Hello"
wamuxctl cloud send template --instance-token "$INSTANCE_TOKEN" --to 5511999999999 --name order_update --lang en_US --dry-run
wamuxctl cloud account-events list --instance-token "$INSTANCE_TOKEN"
wamuxctl cloud business phone-numbers --instance-token "$INSTANCE_TOKEN"
wamuxctl cloud business analytics --instance-token "$INSTANCE_TOKEN" --start 2026-01-01 --end 2026-01-31 --granularity DAY
wamuxctl cloud coverage --json
```

`--dry-run` prints the method, URL, body and a redacted token and makes no HTTP
request. Destructive commands require `--yes`.

## Observability and limits

`/metrics` adds Cloud series for Graph request count/last latency, send
results, webhook results, signature failures, window expiries, media backlog and
unknown outcomes. Labels never contain phone numbers or contact ids.

`GET /server/health` includes Cloud credential/registration/template counts and
the last webhook age. `GET /cloud/diagnostics` remains the per-instance view.

Sends are paced per phone number. Postgres uses `cloud_rate_buckets` with a row
lock so replicas share a budget; SQLite and tests fall back to a process-local
bucket. A 429 halves only the affected phone's rate; POSTs are never retried
automatically.

## Separate opt-in programs

Groups, Calling and Flows are not enabled by the core cloud flag. Each has its
own flag and route namespace. The manager and CLI only show them when enabled.

### Flows (`CLOUD_FLOWS=true`)

```text
GET/POST /cloud/flows
GET      /cloud/flows/:id
POST     /cloud/flows/:id
POST     /cloud/flows/:id/publish
POST     /cloud/flows/:id/deprecate
DELETE   /cloud/flows/:id
GET      /cloud/flows/:id/assets
POST     /cloud/flows/:id/send
GET      /cloud/flows/events
POST     /webhook/cloud/flows/:instanceId   (signed)
```

### Calling (`CLOUD_CALLING=true`)

```text
POST     /cloud/calling/calls
GET      /cloud/calling/calls/:id
POST     /cloud/calling/calls/:id/actions
POST     /cloud/calling/calls/:id/media       (explicit 501)
GET/POST /cloud/calling/permissions
GET/POST /cloud/calling/settings
GET      /cloud/calling/events
```

The control plane starts, accepts, rejects and terminates Meta calls. It does
**not** terminate WebRTC/SIP media; a deployment must provide TURN/SFU and call
media handling separately. Media actions return `501 not_implemented_on_channel`
rather than silently failing.

### Groups (`CLOUD_GROUPS=true`)

```text
GET/POST /cloud/groups
GET      /cloud/groups/:id
DELETE   /cloud/groups/:id
GET/POST /cloud/groups/:id/participants
DELETE   /cloud/groups/:id/participants
POST     /cloud/groups/:id/invite-link
GET      /cloud/groups/:id/join-requests
POST     /cloud/groups/:id/join-requests/:requestId
POST     /cloud/groups/:id/settings
POST     /cloud/groups/:id/messages
GET      /cloud/groups/events
```

Groups are OBA-only and limited to 8 participants per group. Only text, media
and template messages are accepted; interactive/commerce/calling payloads are
rejected before the Graph call.

## Templates

```text
GET    /template
POST   /template
POST   /template/sync
PUT    /template/:name/:language
DELETE /template/:name/:language
```

Template status, quality, category and rejection reasons are Meta-owned. WaMux
caches them for the manager and does not bypass Meta approval.

## Unsupported / deferred

- Groups, Calling and Flows are separate programs with their own flags and
  operational estimates; they are not core channel features.
- Albums, polls, carousels (unless passed as approved template components),
  status/stories, communities, newsletters and full presence do not have a
  Cloud equivalent.
- Cloud has no general chat-history or contact-list API.

## Safe send semantics

WaMux never automatically retries `POST /messages`, template creation or phone
registration after a timeout or 5xx. A timeout can mean Meta accepted the
message; the API returns `202 cloud_send_outcome_unknown` and an operation ID.
Check the operation instead of re-sending blindly.

## Troubleshooting

| Code | Action |
|---|---|
| `133010` | Register the phone number with its six-digit PIN |
| `131030` | Add the recipient to the Meta test-number allowlist |
| `131047` | Send an approved template; the 24-hour window is closed |
| `131062` | Use a phone number for this message type |
| `132000`/`132001` | Check template parameters/language/approval |
| `190` | Rotate the Meta access token |
| `80007`/`130429` | Back off; the per-phone limiter will slow down |
| `cloud_embedded_state_invalid` | Restart Embedded Signup; the state expired or was reused |
