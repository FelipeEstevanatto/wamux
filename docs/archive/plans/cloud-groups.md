# Cloud Groups API — separate program

Status: **implemented opt-in**. Flag: `CLOUD_GROUPS=false` by default.

## Scope

- OBA (Official Business Account) only; max 8 participants and 10,000 groups per
  business number.
- Create/delete groups, invite links, join requests, participant add/remove,
  settings and group webhooks.
- Text/media/approved-template messages only; no interactive, auth, commerce or
  calls.

## Boundary

Package `pkg/cloud/groups`, routes under `/cloud/groups`, group event storage and
the main signed WABA webhook hook. It must not expose routes when the flag is off.
The group send path uses the core phone limiter and Cloud send observer but does
not create a direct-contact message row.

## Estimate

2–3 weeks after OBA eligibility and Meta permission review.
