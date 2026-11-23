# Cloud API coverage manifest

This manifest is the contract between the Cloud adapter, the operator CLI and
the manager. Every Graph operation we call has a row here; every unimplemented
operation has an explicit status (`deferred` or `separate`) and a reason.

The machine-readable copy is [`cloud-api-coverage.json`](cloud-api-coverage.json).
`wamuxctl cloud coverage --json` prints the same operation set for scripts.

| Area | Status | Notes |
|---|---|---|
| Messages: text, media, template, location, contact, reaction, interactive | implemented | `POST /{phone-number-id}/messages` |
| Interactive: CTA URL, product, product list, catalog, contact request | implemented | Window rules apply to free-form messages |
| Templates: list, sync, create, update, delete, webhooks | implemented | Components are passed through; carousel/LTO use raw components |
| Phone/WABA assets, register/deregister, request/verify code | implemented | Instance token scopes the WABA |
| Analytics | implemented | Short-lived in-process cache; no raw analytics persisted |
| Catalogs/products | implemented | Live Graph calls; product pagination is cursor-based |
| Embedded Signup | implemented | State is server-side and short-lived; token never enters a URL |
| Groups | separate | `pkg/cloud/groups` design, OBA-only |
| Calling | separate | WebRTC/SIP infrastructure is out of the core send path |
| Flows | separate | Flow JSON and endpoint callbacks are a separate package |

Unknown webhook fields are stored in `cloud_account_events` with
`event_type=unknown`; they must never make Meta retry with a 5xx.
