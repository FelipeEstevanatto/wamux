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

## Quick start (prebuilt image)

Requires Docker with the Compose plugin. No clone, no build.

```bash
mkdir wamux && cd wamux

curl -fsSL -o docker-compose.yml \
  https://raw.githubusercontent.com/FelipeEstevanatto/wamux/main/docker/examples/docker-compose.ghcr.yml
curl -fsSL -o .env \
  https://raw.githubusercontent.com/FelipeEstevanatto/wamux/main/docker/examples/.env.example

# Set a strong admin key (required), then start
sed -i "s/^GLOBAL_API_KEY=.*/GLOBAL_API_KEY=$(openssl rand -hex 24)/" .env
docker compose pull && docker compose up -d
```

| | |
|---|---|
| API | <http://localhost:8081> |
| Swagger | <http://localhost:8081/swagger/index.html> |
| Manager | <http://localhost:8081/manager> (log in with `GLOBAL_API_KEY`) |
| Dashboard | <http://localhost:8081/dashboard> |

`WAMUX_VERSION` in `.env` selects the image tag: `latest` (newest release), a
version such as `0.9.1`, or `dev` (develop branch).

Or with a plain `docker run` (bring your own Postgres):

```bash
docker run -d --name wamux -p 8081:8080 \
  -e GLOBAL_API_KEY=your-secure-api-key-here \
  -e POSTGRES_AUTH_DB='postgresql://user:pass@host:5432/wamux_auth?sslmode=disable' \
  -e POSTGRES_USERS_DB='postgresql://user:pass@host:5432/wamux_users?sslmode=disable' \
  -v wamux_data:/app/data \
  ghcr.io/felipeestevanatto/wamux:latest
```

## Build from source

```bash
git clone https://github.com/FelipeEstevanatto/wamux.git
cd wamux
cp .env.example .env          # set GLOBAL_API_KEY
docker compose up -d --build  # also builds the manager SPA (an oven/bun stage)
```

## Local development (no Docker for the app)

```bash
git clone https://github.com/FelipeEstevanatto/wamux.git
cd wamux
make setup                    # installs deps + prepares .env
# edit .env: GLOBAL_API_KEY and POSTGRES_* pointing at a reachable Postgres
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
CLIENT_NAME=wamux
OS_NAME=WaMux

GLOBAL_API_KEY=your-secure-api-key-here          # required admin key

POSTGRES_AUTH_DB=postgresql://user:pass@localhost:5432/wamux_auth?sslmode=disable
POSTGRES_USERS_DB=postgresql://user:pass@localhost:5432/wamux_users?sslmode=disable
DATABASE_SAVE_MESSAGES=true                       # persist messages (feeds history + counts)
```

A few of the most-used settings:

| Variable | Description | Default |
|---|---|---|
| `GLOBAL_API_KEY` | Admin key (`/instance/all`, `/server/stats`, Manager) | **required** |
| `DATABASE_SAVE_MESSAGES` | Persist messages (enables history readback + dashboard counts) | `false` (compose: `true`) |
| `MESSAGE_RETENTION_DAYS` | Prune stored messages after N days (`0` = keep forever) | `365` |
| `CONNECT_ON_STARTUP` | Reconnect paired instances on boot | `false` |
| `SWAGGER_ENABLED` | Serve `/swagger` publicly | `true` |
| `SSRF_PROTECTION` | Opt-in: refuse outbound fetches to loopback/private/link-local hosts | `false` |
| `WEBHOOK_HMAC_KEY` | Global key for webhook HMAC signing | — |
| `RATE_LIMIT_PER_MINUTE` | Requests per credential / per IP (`0` disables) | `600` |

RabbitMQ, NATS and MinIO settings are in the same `.env.example`.

> **`/swagger` is public by default.** It is not gated by `GLOBAL_API_KEY` — set
> `SWAGGER_ENABLED=false` on an internet-facing deployment.

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
- **Messaging** — text, media, location, contact, polls, stickers, buttons,
  lists, carousels, events and product cards; reactions, edits, deletes, presence.
- **Events** — per-instance webhooks (with HMAC signing), RabbitMQ, NATS and a
  WebSocket stream. See [`docs/wiki/recursos-avancados/events-system.md`](./docs/wiki/recursos-avancados/events-system.md).
- **Media** — inbound media to MinIO/S3 (globally or per instance), optional
  local copies for in-thread previews.
- **History** — message persistence with readback (`GET /chat/history`,
  `GET /chat/chats`, `GET /chat/media/:messageId`).
- **Typebot** — optional chatbot integration (inert until configured).
- **Manager** — React admin panel (source in [`wamux-manager/`](./wamux-manager))
  plus a self-hosted `/dashboard`.

The full endpoint list is in the
[API reference](./docs/wiki/referencia/api-reference.md) and the live Swagger UI
(`/swagger`). A few notable endpoints:

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `GET` | `/instance/overview/:instanceId` | global | Profile picture, push name, device platform, contact/chat/message counts |
| `GET` | `/server/stats` | global | Runtime/host metrics, message aggregates, running version |
| `GET` | `/server/health` · `/server/ok` | — | Readiness / liveness probes |
| `GET` | `/metrics` | — | Prometheus metrics |
| `GET` | `/chat/history` · `/chat/chats` · `/chat/media/:messageId` | instance | Stored messages / conversations / attachments |
| `POST` | `/instance/hmac` | instance | Configure webhook HMAC signing |
| `POST` | `/instance/s3` | instance | Per-instance S3 media storage |
| `POST` | `/send/*` | instance | Send text, media, buttons, lists, carousels, events, products |

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
webhooks. Limits default to 50 and cap at 500.

### SSRF protection

Endpoints that fetch a caller-supplied URL can refuse loopback, private
(RFC1918), link-local (including `169.254.169.254`), CGNAT and multicast
addresses. It is opt-in (`SSRF_PROTECTION=true`) so senders that fetch media
from an internal host keep working by default.

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
