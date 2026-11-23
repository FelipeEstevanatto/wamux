# Cloud Groups API — separate program

Status: **not implemented**. Flag: `CLOUD_GROUPS=false`.

## Scope

- OBA (Official Business Account) only; max 8 participants and 10,000 groups per
  business number.
- Create/delete groups, invite links, join requests, participant add/remove,
  settings and group webhooks.
- Text/media/approved-template messages only; no interactive, auth, commerce or
  calls.

## Boundary

Package `pkg/cloud/groups`, own handler/routes and own webhook path. It must not
change `pkg/cloud` send behavior or expose routes when the flag is off.

## Estimate

2–3 weeks after OBA eligibility and Meta permission review.
