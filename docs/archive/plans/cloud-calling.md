# Cloud Calling API — separate program

Status: **not implemented**. Flag: `CLOUD_CALLING=false`.

## Scope

- WebRTC/SIP signaling, call permissions/settings and `calls` webhooks.
- Media path with TURN/SFU; do not put it in the messaging send service.
- Call event persistence and operational runbook.

## Boundary

Package `pkg/cloud/calling`, own routes and webhook handling. No core send
changes and no calling routes when the flag is off.

## Estimate

3–4 weeks plus infrastructure and Meta review.
