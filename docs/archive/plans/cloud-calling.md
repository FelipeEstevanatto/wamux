# Cloud Calling API — separate program

Status: **control plane implemented opt-in**; media plane remains external. Flag: `CLOUD_CALLING=false` by default.

## Scope

- WebRTC/SIP signaling, call permissions/settings and `calls` webhooks.
- Media path with TURN/SFU; do not put it in the messaging send service.
- Call event persistence and operational runbook.

## Boundary

Package `pkg/cloud/calling`, routes under `/cloud/calling`, `cloud_call_events`,
call start/get/action control, permissions/settings and the main signed WABA
`calls` webhook hook. `POST /cloud/calling/calls/:id/media` returns an explicit
501: WebRTC/SIP media is not terminated by this control plane.

## Estimate

3–4 weeks plus infrastructure and Meta review.
