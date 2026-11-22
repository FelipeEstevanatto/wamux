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

## 24-hour window

A user message opens a 24-hour customer-service window. Free-form messages
outside it return `422 cloud_window_expired`; an approved template is required.
This is Meta policy, not a WaMux preference.

## BSUID and usernames

Meta now sends BSUIDs (`user_id`, `from_user_id`, `recipient_user_id`) and may
omit phone numbers. WaMux stores a contact identity table and accepts BSUID
recipients. Some authentication templates still require a phone number; Meta
returns `131062` when a BSUID is not allowed.

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

- Groups, Calling, Flows and product/catalog interactions are Cloud-supported
  but not implemented in this release.
- Albums, polls, carousels, status/stories, communities, newsletters and full
  presence do not have a Cloud equivalent.
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
| `80007`/`130429` | Back off; the adaptive limiter will slow down |
