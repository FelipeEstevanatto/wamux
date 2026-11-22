# Strategic spikes and deferred work

Design-only investigations requested by the Reforged comparison. None of these
are production code.

| Document | Recommendation |
|---|---|
| [`durable-events.md`](./durable-events.md) | Prototype a Postgres inbox behind a flag; keep NATS/Redis as optional backends |
| [`generic-chatbot.md`](./generic-chatbot.md) | One HTTP chatbot adapter; do not port six providers |
| [`kafka.md`](./kafka.md) | Defer until a deployment asks; build in a separate module/build tag |
| [`waba-cloud-api.md`](./waba-cloud-api.md) | 6–8 week Cloud API plan; start with signed webhook + text/media/templates |
| [`cloud-api-mysql.md`](./cloud-api-mysql.md) | Do not pursue either without a concrete paid requirement |

## Deferred manager work (Workstream E)

The backend endpoints exist, but these screens were deliberately deferred rather
than half-built. Each needs a product decision on layout and permissions:

- **Typebot**: bot CRUD, session list and pause/resume. The backend is stable; a
  screen needs a multi-step form and an operational view.
- **Labels, communities and newsletters**: list/create/membership flows. These
  are multi-screen management areas, not settings forms.
- **Poll results**: a results table per poll, with voter privacy rules.
- **Fleet inventory/limits**: belongs on the dashboard, not the per-instance
  settings page; needs a design pass for pagination and filtering.
- **Per-instance webhook event matrix**: the current settings page edits the
  flat list; a matrix is a larger UI change.

They are deferred, not dropped. The existing API/Swagger is the source of truth
until those screens are designed.
