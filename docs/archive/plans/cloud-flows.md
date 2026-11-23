# Cloud Flows API — separate program

Status: **not implemented**. Flag: `CLOUD_FLOWS=false`.

## Scope

- Flow JSON create/update/publish, flow assets, endpoint callbacks, `nfm_reply`
  inbound messages, status webhooks and encryption/signing.

## Boundary

Package `pkg/cloud/flows`, own routes and webhook field handlers. Core typed
webhooks may store a Flow field as a known account event, but the Flow program
owns its typed contract.

## Estimate

2–4 weeks plus Meta review.
