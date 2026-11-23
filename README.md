<h1 align="center">WaMux</h1>

<p align="center">
  High-performance, self-hosted WhatsApp API built in Go on top of
  <a href="https://github.com/tulir/whatsmeow">whatsmeow</a>.
</p>

<p align="center">
  <a href="https://github.com/FelipeEstevanatto/wamux/releases/latest"><img src="https://img.shields.io/github/v/release/FelipeEstevanatto/wamux?include_prereleases&label=version&color=00ffa7" alt="Latest version" /></a>
  <a href="https://opensource.org/licenses/Apache-2.0"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License: Apache 2.0" /></a>
  <a href="https://github.com/FelipeEstevanatto/wamux/pkgs/container/wamux"><img src="https://img.shields.io/badge/Container-ghcr.io-blue" alt="Container image" /></a>
</p>

---

WaMux runs a multi-device WhatsApp session with
[whatsmeow](https://github.com/tulir/whatsmeow) and exposes it as a **REST API**,
a **webhook / WebSocket** event stream and a bundled **React admin panel**.

It ships with **no license activation, no heartbeat and no telemetry**: it starts
fully operational and never contacts a third party on its own.

| | |
|---|---|
| **Documentation** | [`docs/wiki/`](./docs/wiki) — guides, API reference, deployment |
| **Container image** | `ghcr.io/felipeestevanatto/wamux` |
| **License** | Apache-2.0 — [`LICENSE`](./LICENSE) · [`NOTICE`](./NOTICE) · [`TRADEMARKS.md`](./TRADEMARKS.md) |
| **Security** | [`SECURITY.md`](./SECURITY.md) · what changed vs. upstream: [`FORK_NOTES.md`](./FORK_NOTES.md) |

## Quick start

Requires Docker with Compose 2.24 or newer. Nothing to clone, build or configure.

```bash
mkdir wamux && cd wamux
curl -fsSL -O https://raw.githubusercontent.com/FelipeEstevanatto/wamux/main/docker/examples/docker-compose.yml
docker compose up -d
docker compose exec wamux cat /app/data/global-api-key
```

Open <http://localhost:8081/manager> and log in with that key. It is generated on
first boot and kept in the `wamux_data` volume, so back that volume up along with the
database.

| | |
|---|---|
| API | <http://localhost:8081> |
| Swagger | <http://localhost:8081/swagger/index.html> |
| Manager | <http://localhost:8081/manager> |

Every setting is optional. Put overrides in a `.env` next to the compose file, such as
`CLIENT_NAME`, `WAMUX_PORT`, `WAMUX_VERSION` (`latest`, a version such as `1.0.0`, or `dev`) or
`GLOBAL_API_KEY`. See [`.env.example`](./docker/examples/.env.example). Change
`POSTGRES_PASSWORD` before the first boot, if at all: the database is not published
outside Docker.

[`docker-compose.full.yml`](./docker/examples/docker-compose.full.yml) adds RabbitMQ,
NATS and MinIO, and [`docker-compose.swarm.yml`](./docker/examples/docker-compose.swarm.yml)
runs behind Traefik on Swarm.

Or with a plain `docker run` (bring your own Postgres):

```bash
docker run -d --name wamux -p 8081:8080 \
  -e POSTGRES_AUTH_DB='postgresql://user:pass@host:5432/wamux_auth?sslmode=disable' \
  -e POSTGRES_USERS_DB='postgresql://user:pass@host:5432/wamux_users?sslmode=disable' \
  -v wamux_data:/app/data \
  ghcr.io/felipeestevanatto/wamux:latest
```

## Build from source

```bash
git clone https://github.com/FelipeEstevanatto/wamux.git
cd wamux
docker build -t ghcr.io/felipeestevanatto/wamux:local .
WAMUX_VERSION=local docker compose -f docker/examples/docker-compose.yml up -d
```

> Building the image requires **BuildKit** (Docker Engine 23+ or Docker Desktop,
> the default on current releases). It uses Go build-cache mounts, so a source
> change rebuilds in seconds.

### Releasing

`VERSION` is the source of truth. A merge to `main` publishes the immutable
`<version>` and `sha-<short>` tags and creates a GitHub **pre-release**; it does
**not** move `latest`. After the pre-release has been checked, run
**Actions → Promote release** with its version. That points `latest` and
`<major>.<minor>` at the same image digest, with no rebuild, and marks the
release as latest. The publish job fails instead of overwriting an existing tag.

## Local development (no Docker for the app)

```bash
git clone https://github.com/FelipeEstevanatto/wamux.git
cd wamux
make setup                    # installs deps + prepares .env
# edit .env: POSTGRES_* pointing at a reachable Postgres
make dev
```

`make help` lists every target. To build the manager alone:
`make manager-install && make manager-build`.

## Configuration

Create a `.env` (see [`docker/examples/.env.example`](./docker/examples/.env.example)
for the full commented list, and
[`docs/wiki/referencia/environment-variables.md`](./docs/wiki/referencia/environment-variables.md)
for the reference):

```env
SERVER_PORT=8080
CLIENT_NAME=wamux                                 # names this server; see the table below
OS_NAME=WaMux

GLOBAL_API_KEY=your-secure-api-key-here          # admin key; generated when empty

POSTGRES_AUTH_DB=postgresql://user:pass@localhost:5432/wamux_auth?sslmode=disable
POSTGRES_USERS_DB=postgresql://user:pass@localhost:5432/wamux_users?sslmode=disable
DATABASE_SAVE_MESSAGES=true                       # persist messages (feeds history + counts)
```

A few of the most-used settings:

| Variable | Description | Default |
|---|---|---|
| `GLOBAL_API_KEY` | Admin key (`/instance/all`, `/server/stats`, Manager) | generated on first boot |
| `CLIENT_NAME` | Names this server. Instances belong to it, so changing it hides them until it is changed back. Servers sharing a database need different names | `wamux` |
| `DATABASE_SAVE_MESSAGES` | Persist messages (enables history readback + dashboard counts) | `true` |
| `DATABASE_SAVE_HISTORY_SYNC` | Also store history sync batches, up to `MESSAGE_RETENTION_DAYS` old | `true` |
| `MESSAGE_RETENTION_DAYS` | Prune stored messages after N days (`0` = keep forever) | `365` |
| `CONNECT_ON_STARTUP` | Reconnect paired instances on boot | `true` (explicit API disconnects stay down) |
| `SWAGGER_ENABLED` | Serve `/swagger` publicly | `true` |
| `METRICS_AUTH` | `/metrics` access: `admin`, `loopback` or `none` | `admin` |
| `AVATAR_RATE_LIMIT_PER_MINUTE` | Profile-picture queries per instance (`0` disables) | `30` |
| `AVATAR_RATE_LIMIT_BURST` | Burst for the avatar limiter (`0` disables) | `5` |
| `SSRF_PROTECTION` | Opt-in: refuse outbound fetches to loopback/private/link-local hosts | `false` |
| `WEBHOOK_HMAC_KEY` | Global key for webhook HMAC signing | — |
| `RATE_LIMIT_PER_MINUTE` | Requests per credential / per IP (`0` disables) | `600` |
| `WEBHOOK_BASE64_MAX_BYTES` | Largest file embedded as base64 in an event; larger files stream to storage and the event carries `mediaUrl` (`0` never embeds) | `5242880` |
| `MAX_UPLOAD_BYTES` | Largest file `/send/media` accepts; files over 64 MiB stream through disk and must be a document or an MP4 video | `2147483648` |
| `MEDIA_RETENTION_DAYS` | Days to keep stored media (`0` keeps it forever) | `MESSAGE_RETENTION_DAYS` |
| `HTTP_ACCESS_LOG` | Request log: `errors`, `all` or `off` | `errors` |
| `TRUSTED_PROXIES` | Who may set `X-Forwarded-For`: IPs/CIDRs, or `none` | private addresses, so a local reverse proxy needs nothing |
| `NATS_JETSTREAM` | Publish through an existing NATS stream (at-least-once + dedupe) | `false` |
| `NATS_STREAM` | Stream name used when JetStream is enabled | `WAMUX_EVENTS` |

RabbitMQ, NATS and MinIO settings are in the same `.env.example`.

> **`/swagger` is public by default.** It is not gated by `GLOBAL_API_KEY` — set
> `SWAGGER_ENABLED=false` on an internet-facing deployment.
>
> **`/metrics` requires the global API key by default** (`METRICS_AUTH=admin`).
> Set `METRICS_AUTH=loopback` for a local scrape agent or `none` for the old
> public behaviour.

## Authentication

Two credentials, both sent in the `apikey` header:

- **Global API key** (`GLOBAL_API_KEY`) — admin routes: `/instance/all`,
  `/instance/create`, `/instance/delete/:id`, `/server/stats`,
  `/instance/overview/:id`, `/instance/proxy/:id`, `/instance/limits/:id`.
- **Instance token** (returned by `/instance/create`, shown in the Manager) —
  everything scoped to one instance: `/send/*`, `/message/*`, `/chat/*`,
  `/group/*`, `/user/*`, `/typebot/*`, `/instance/connect`, `/instance/qr`.

## Features

- **Instances** — create, connect, pair (QR / pairing code), proxy, per-instance
  overview (profile picture, platform, contact/chat/message counts).
- **Official WhatsApp Cloud API (WABA)** — opt-in second channel with encrypted
  Meta credentials, BSUID identity continuity, signed/typed webhooks, phone
  registration, per-phone rate limiting, metrics, templates, interactive/product
  payloads, Business Management queries, Embedded Signup and opt-in Groups,
  Calling (control plane) and Flows programs. Enable with
  `CLOUD_API_ENABLED=true`; see
  [`docs/wiki/guias-api/api-cloud.md`](./docs/wiki/guias-api/api-cloud.md).
- **Messaging** — text, media, location, contact, polls, stickers, buttons,
  lists, carousels, events and product cards; reactions, edits, deletes, presence.
- **Events** — per-instance webhooks (with HMAC signing), RabbitMQ, NATS and a
  WebSocket stream. See [`docs/wiki/recursos-avancados/events-system.md`](./docs/wiki/recursos-avancados/events-system.md).
- **Media** — inbound media to MinIO/S3 (globally or per instance), optional
  local copies for in-thread previews.
- **History** — message persistence with readback (`GET /chat/history`,
  `GET /chat/chats`, `GET /chat/media/:messageId`).
- **Typebot** — optional chatbot integration (inert until configured).
- **Manager** — React admin panel with a dashboard (source in
  [`wamux-manager/`](./wamux-manager)).

The full endpoint list is in the
[API reference](./docs/wiki/referencia/api-reference.md) and the live Swagger UI
(`/swagger`). A few notable endpoints:

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `GET` | `/instance/overview/:instanceId` | global | Profile picture, push name, device platform, contact/chat/message counts |
| `GET` | `/instance/inventory` | global | Monitoring-safe fleet view: no tokens, hashed owner |
| `GET` | `/server/stats` | global | Runtime/host metrics, message aggregates, running version |
| `GET` | `/server/health` · `/server/ok` | — | Readiness / liveness probes |
| `GET` | `/metrics` | — | Prometheus metrics |
| `GET` | `/chat/history` · `/chat/chats` · `/chat/media/:messageId` | instance | Stored messages / conversations / attachments |
| `GET` | `/chat/history/capabilities` | instance | History pagination contract (`keyset`, max page size) |
| `POST` | `/chat/lidMapping` | instance | Batch LID ↔ phone-number resolution (up to 200) |
| `POST` | `/instance/hmac` | instance | Configure webhook HMAC signing |
| `POST` | `/instance/s3` | instance | Per-instance S3 media storage |
| `POST` | `/send/*` | instance | Send text, media, albums, buttons, lists, carousels, events, products |
| `POST` | `/send/template` | instance | Send an approved Cloud API template |
| `GET` | `/send/operations` · `/send/operations/:id` | instance | Idempotent send state (pending / sent / failed) |
| `GET/POST` | `/webhook/cloud` · `/webhook/cloud/:appId` | — | Meta Cloud API callback (signature verified) |
| `GET/PUT` | `/cloud/credentials` | instance | Read (safe summary) or update encrypted Meta credentials |
| `POST` | `/cloud/verify` · `/cloud/register` · `/cloud/read` | instance | Verify/subscribe, register the phone, mark read |
| `GET` | `/cloud/diagnostics` · `/cloud/templates` | instance | Cloud health and cached templates |
| `GET` | `/cloud/contacts/:contactId` · `/cloud/identity-events` | instance | BSUID identity continuity and reconciliation |
| `GET` | `/cloud/account-events` | instance | Typed account/template webhook history |
| `GET/POST` | `/cloud/business/*` | instance | Phone/WABA assets, analytics and catalogs |
| `POST` | `/cloud/embedded/start` | instance | Start Embedded Signup (Meta OAuth) |
| `POST` | `/send/catalog` · `/send/contact-request` | instance | Cloud catalog and contact-request messages |

Full list: `GET /swagger/doc.json`.

### Webhook HMAC signing

Every HTTP webhook delivery can carry an `x-hmac-signature` header: the lowercase
hex HMAC-SHA256 of the exact request body. Configure a per-instance key with
`POST /instance/hmac` (`GET`/`DELETE` to inspect/clear) or a global
`WEBHOOK_HMAC_KEY`; the per-instance key wins. Keys are encrypted at rest with
`WEBHOOK_HMAC_ENCRYPTION_KEY` (falling back to `GLOBAL_ENCRYPTION_KEY`, then to a
key derived from `GLOBAL_API_KEY`). With no key configured, deliveries are sent
unsigned. Failed deliveries can be routed to a RabbitMQ dead-letter queue
(`WEBHOOK_ERROR_QUEUE_NAME`).

### Per-instance S3 storage

Each instance can store inbound media in its own S3-compatible bucket, overriding
the global MinIO config: `POST /instance/s3` (`GET`, `DELETE`, `POST /instance/s3/test`).
`mediaDelivery` selects `base64`, `s3` or `both`. The secret is encrypted at rest.

### Message history readback

With `DATABASE_SAVE_MESSAGES=true`, conversations can be read back over the API
(`GET /chat/history?chat=<phone-or-JID>&limit=&before=`) instead of only through
webhooks. Limits default to 50 and cap at 500. Filters: `after`, `until`
(inclusive timestamps), `type` and `from_me`; `keyset=true` adds the exact
cursor. `/chat/history/search` accepts the same filters with `q`; omit `chat` to search
across every conversation of the instance.

### Idempotent sends

`POST /send/text` and JSON `POST /send/media` accept an `Idempotency-Key`. The
first call reserves a native WhatsApp id and runs normally; a retry with the same
semantic JSON replays the stored response without sending again. A different
body with the same key is `409`; an in-flight duplicate is `202`. Pending rows
are reconciled against the stored message when `GET /send/operations/:id` is
read. See [`docs/operations/send-idempotency.md`](./docs/operations/send-idempotency.md).

### Fleet inventory

`GET /instance/inventory` (global key) returns connection state, auto-reconnect
and a SHA-256 owner fingerprint for each instance. It never returns the instance
token or the raw owner JID, so it is safe to feed a monitoring system.

### SSRF protection

Endpoints that fetch a caller-supplied URL (media, link previews, button/
carousel headers, stickers, group photos) **always** refuse loopback, private
(RFC1918), link-local (including `169.254.169.254`), CGNAT and multicast
addresses. DNS is pinned and every redirect is revalidated.

`SSRF_PROTECTION` still exists for explicitly configured internal services that
intentionally fetch private hosts; it does not loosen the caller-URL guard.
Send internal media by upload/base64, or expose it through a public endpoint.

## Manager

The React panel is served at `/manager`; its source lives in this repo at
[`wamux-manager/`](./wamux-manager) (React 19, Vite, Tailwind). It covers instance
management, QR/pairing, messaging, the messages screen, webhooks, proxy settings,
a system dashboard, an API tester and the **Sobre / About** page.

## Documentation

The full manual lives in [`docs/wiki/`](./docs/wiki):

| Start here | Concepts | API | Operations |
|---|---|---|---|
| [Introduction](./docs/wiki/fundamentos/introduction.md) | [Architecture](./docs/wiki/conceitos-core/architecture.md) | [API overview](./docs/wiki/guias-api/api-overview.md) | [Docker deploy](./docs/wiki/deploy-producao/docker-deployment.md) |
| [Installation](./docs/wiki/fundamentos/installation.md) | [Instances](./docs/wiki/conceitos-core/instances.md) | [API reference](./docs/wiki/referencia/api-reference.md) | [Security](./docs/wiki/deploy-producao/security.md) |
| [Quickstart](./docs/wiki/fundamentos/quickstart.md) | [Database](./docs/wiki/conceitos-core/database.md) | [Environment variables](./docs/wiki/referencia/environment-variables.md) | [Debugging](./docs/wiki/desenvolvimento/debugging.md) |

## Project layout

```
wamux/
├── cmd/wamux/             # Application entry point
├── pkg/                   # Go packages (routes, services, whatsmeow, ...)
├── wamux-manager/         # Manager SPA source (React/Vite)
├── manager/dist/          # Built manager assets served at /manager
├── docker/examples/       # Ready-to-run compose files
├── docs/wiki/             # Documentation
├── docs/                  # Generated Swagger
├── .github/workflows/     # GHCR publish + security scans
├── Dockerfile
├── Makefile
└── VERSION                # Single source of truth for the version
```

## Contributing

Issues and pull requests are welcome in this repository.

- Do **not** open a public issue for security problems — see [`SECURITY.md`](./SECURITY.md).
- Development setup and conventions: [`docs/wiki/desenvolvimento/development-guide.md`](./docs/wiki/desenvolvimento/development-guide.md) and [`docs/wiki/desenvolvimento/contributing.md`](./docs/wiki/desenvolvimento/contributing.md).

## License & attribution

Apache License 2.0, with the additional conditions in [`LICENSE`](./LICENSE)
(see also [`NOTICE`](./NOTICE) and [`TRADEMARKS.md`](./TRADEMARKS.md)). The code
was originally forked from
[Evolution Go](https://github.com/evolution-foundation/evolution-go) by Evolution
Foundation; its copyright and trademark notices are retained. WaMux is not
affiliated with, endorsed by, or an official release of Evolution Foundation, and
`LICENSE` condition **1.b** requires any system that uses it to surface a clear,
administrator-visible notice (the Manager's **Sobre / About** page satisfies it).
