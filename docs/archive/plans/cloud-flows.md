# Cloud Flows API — separate program

Status: **implemented opt-in**. Flag: `CLOUD_FLOWS=false` by default.

## Scope

- Flow JSON create/update/publish, flow assets, endpoint callbacks, `nfm_reply`
  inbound messages, status webhooks and encryption/signing.

## Boundary

Package `pkg/cloud/flows`, routes under `/cloud/flows`, `cloud_flow_events`,
Flow JSON lifecycle/assets, an interactive Flow send entry point and signed
endpoint callbacks. Core typed webhooks may also store a Flow status field; the
Flow program owns its typed contract.

## Estimate

2–4 weeks plus Meta review.
