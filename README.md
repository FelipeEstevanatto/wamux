<p align="center">
  <a href="https://evolutionfoundation.com.br">
    <img src="./public/hover-evolution.png" alt="Evolution Foundation" />
  </a>
</p>

<h1 align="center">Evolution Go — community fork</h1>

<p align="center">
  High-performance WhatsApp API built in Go, maintained as a community fork of
  Evolution Go.
</p>

> ### ⚠️ Unofficial community fork
>
> This repository (`FelipeEstevanatto/evo-gofork`) is a **community fork** of
> Evolution Go. It is **not affiliated with, endorsed by, or an official release
> of Evolution Foundation**. Upstream:
> <https://github.com/evolution-foundation/evolution-go>.
>
> - **Container image:** `ghcr.io/felipeestevanatto/evo-gofork`
> - **What this fork changes:** [`FORK_NOTES.md`](./FORK_NOTES.md) · [`CHANGELOG.md`](./CHANGELOG.md)
> - **Attribution & brand terms:** [`NOTICE`](./NOTICE) · [`TRADEMARKS.md`](./TRADEMARKS.md)

<p align="center">
  <a href="https://github.com/FelipeEstevanatto/evo-gofork/releases/latest"><img src="https://img.shields.io/github/v/release/FelipeEstevanatto/evo-gofork?include_prereleases&label=version&color=00ffa7" alt="Latest version" /></a>
  <a href="https://opensource.org/licenses/Apache-2.0"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License: Apache 2.0" /></a>
  <a href="https://docs.evolutionfoundation.com.br"><img src="https://img.shields.io/badge/Docs-upstream-00ffa7" alt="Documentation (upstream)" /></a>
  <a href="https://github.com/FelipeEstevanatto/evo-gofork/pkgs/container/evo-gofork"><img src="https://img.shields.io/badge/Container-ghcr.io-blue" alt="Container image (this fork)" /></a>
</p>

---

## About

**Evolution Go** is a high-performance WhatsApp API built in Go on top of
[whatsmeow](https://github.com/tulir/whatsmeow). This fork tracks upstream `0.7.2`
and adds, on top of it:

- a **versioned manager source** (`evolution-go-manager/`) with refreshed
  dependencies, so the panel can actually be changed and built;
- API/manager features (per-instance overview with profile picture, contact /
  chat / message counts and device platform, proxy settings, ephemeral timers,
  Typebot, `/server/stats`, self-hosted `/dashboard`);
- a **security audit** pass (SQL injection, vulnerable dependencies, orphaned
  credentials, constant-time key comparison) — `govulncheck` reports **0**;
- a **whatsmeow API audit** (media retry, `WaitForConnection`, bounded retry
  receipts, `BuildReaction`, reconnect on `StreamError`/`KeepAliveTimeout`, more
  webhook events);
- **no license gate, no heartbeat and no telemetry** — see
  [`FORK_NOTES.md`](./FORK_NOTES.md) §1.

It is an independent distribution: report issues here, not upstream.

---

## Quick start

### Docker — pull the published image (recommended)

No clone, no build. The image is public on GHCR:

```bash
mkdir evogo && cd evogo

# Compose that runs the published image
curl -fsSL -o docker-compose.yml \
  https://raw.githubusercontent.com/FelipeEstevanatto/evo-gofork/develop/docker/examples/docker-compose.ghcr.yml

# Environment (set GLOBAL_API_KEY!)
curl -fsSL -o .env \
  https://raw.githubusercontent.com/FelipeEstevanatto/evo-gofork/develop/docker/examples/.env.example

# Image tag: `dev` (default), `0.8.1` (release) or `latest`
sed -i "s/^EVOGO_VERSION=.*/EVOGO_VERSION=dev/" .env

docker compose pull && docker compose up -d
```

Or with a plain `docker run` (bring your own Postgres):

```bash
docker run -d --name evolution_go -p 8081:8080 \
  -e GLOBAL_API_KEY=your-secure-api-key-here \
  -e POSTGRES_AUTH_DB='postgresql://user:pass@host:5432/evogo_auth?sslmode=disable' \
  -e POSTGRES_USERS_DB='postgresql://user:pass@host:5432/evogo_users?sslmode=disable' \
  -v evolution_go_data:/app/data \
  -e LOG_DIRECTORY=/app/data/logs \
  ghcr.io/felipeestevanatto/evo-gofork:dev
```

Then open:

| | |
|---|---|
| API | <http://localhost:8081> |
| Swagger | <http://localhost:8081/swagger/index.html> |
| Manager | <http://localhost:8081/manager> (log in with `GLOBAL_API_KEY`) |
| Dashboard | <http://localhost:8081/dashboard> (same key) |

### Docker — build from source

```bash
git clone https://github.com/FelipeEstevanatto/evo-gofork.git
cd evo-gofork
git checkout develop

cp .env.example .env        # set GLOBAL_API_KEY
docker compose up -d --build
```

`docker compose up --build` also builds the manager SPA (a `oven/bun` stage),
so frontend edits are picked up by the same command.

### Local development

```bash
git clone https://github.com/FelipeEstevanatto/evo-gofork.git
cd evo-gofork
git checkout develop

make setup
cp .env.example .env        # set GLOBAL_API_KEY
make dev
```

> `make help` lists every target. The manager alone: `make manager-install` then
> `make manager-build` (pnpm, bun or npm).

---

## Configuration

Create a `.env` file (see `docker/examples/.env.example` for the full list):

```env
# Server
SERVER_PORT=8080
CLIENT_NAME=evolution
OS_NAME=Evolution GO

# Security (required)
GLOBAL_API_KEY=your-secure-api-key-here

# Database
POSTGRES_AUTH_DB=postgresql://user:pass@localhost:5432/evogo_auth?sslmode=disable
POSTGRES_USERS_DB=postgresql://user:pass@localhost:5432/evogo_users?sslmode=disable
DATABASE_SAVE_MESSAGES=true

# Logging / runtime
DEBUG_ENABLED=0
LOG_TYPE=console
CONNECT_ON_STARTUP=true
SWAGGER_ENABLED=true        # set false to hide /swagger

# Optional
# WEBHOOK_URL=https://your-webhook-url.com/webhook
# PASSKEY_PUBLIC_URL=https://your-api.example.com
# AMQP_URL=amqp://user:pass@rabbitmq:5672/
# NATS_URL=nats://nats:4222
# MINIO_ENABLED=true
```

| Variable | Description | Default |
|---|---|---|
| `SERVER_PORT` | Server port (inside the container) | `8080` |
| `CLIENT_NAME` | Client identifier sent to WhatsApp | `evolution` |
| `GLOBAL_API_KEY` | Admin key (`/instance/all`, `/server/stats`, …) | **Required** |
| `POSTGRES_AUTH_DB` / `POSTGRES_USERS_DB` | Auth and users databases (auto-created) | — |
| `DATABASE_SAVE_MESSAGES` | Persist messages (feeds the counts and `/server/stats`) | `true` in compose |
| `CONNECT_ON_STARTUP` | Reconnect instances that were `connected=true` on boot | `false` |
| `SWAGGER_ENABLED` | Serve `/swagger` publicly | `true` |
| `DEBUG_ENABLED` | `1` enables debug logging | `0` |
| `LOG_TYPE` | `console` or `file` | `console` |

---

## Authentication

Two credentials, both sent in the `apikey` header:

- **Global API key** (`GLOBAL_API_KEY`) — admin routes: `/instance/all`,
  `/instance/create`, `/instance/delete/:id`, `/server/stats`,
  `/instance/overview/:id`, `/instance/proxy/:id`, `/instance/limits/:id`.
- **Instance token** (returned by `/instance/create`, shown in the manager) —
  everything scoped to one instance: `/send/*`, `/message/*`, `/chat/*`,
  `/group/*`, `/user/*`, `/typebot/*`, `/instance/connect`, `/instance/qr`.

**No activation is required.** This fork starts fully operational — no license
registration, no gate, no heartbeat to any external server.

---

## API

Swagger UI: <http://localhost:8081/swagger/index.html> (regenerate with
`make swagger`). The manager's **API Tester** reads the live spec, so it is
always current.

A few of the endpoints this fork adds or that are easy to get wrong:

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `GET` | `/instance/overview/:instanceId` | global | Profile picture, push name, device platform, contact/chat/message counts |
| `GET` | `/server/stats` | global | Runtime/host metrics, message aggregates, running version |
| `GET` | `/dashboard` | — | Self-hosted dashboard page |
| `POST` | `/chat/ephemeral` | instance | Set/clear the disappearing-messages timer for a chat |
| `POST` | `/send/text` · `/send/media` · `/send/button` · `/send/list` · `/send/carousel` · `/send/event` · `/send/product` | instance | Send messages |
| `POST` | `/instance/name/:instanceId` | global | Rename an instance (id/token unchanged) |
| `GET` | `/instance/proxy/:instanceId` | global | Get/set/test the instance proxy |
| `GET` | `/instance/limits/:instanceId` | global | Reach-out timelock and new-chat quota (error 463) |
| `GET` | `/instance/all` | global | List instances |
| `GET` | `/instance/qr` | instance | QR / pairing code |

Full list: `GET /swagger/doc.json`.

---

## Manager

The React panel is served at `/manager` and its **source lives in this repo** at
`evolution-go-manager/` (React 19, Vite, Tailwind; the UI primitives are vendored
under `evolution-go-manager/src/components/ui/`). `docker compose up --build`
builds it automatically; locally use `make manager-build` (it syncs into
`manager/dist`).

It shows each instance's profile picture, counts and paired-device platform,
offers proxy settings and a token copy button, a system-wide dashboard, an API
tester and the **Sobre / About** page required by the license (see below).

---

## Usage notification (deployers)

Evolution Go's license (`LICENSE`, additional condition **1.b**) requires any
system that uses it to show a **clear, administrator-visible notification that
Evolution Go is being utilized**, reachable from the documentation or a settings
page. This fork satisfies it with the manager's **Sobre / About** page
(`/manager/about`).

**If you embed this image (or the API) in another product**, you inherit that
obligation for *your* users — surface an equivalent admin-visible notice. The
image ships `LICENSE`, `NOTICE`, `TRADEMARKS.md` and `FORK_NOTES.md` in `/app`.

---

## Stack

| Component | Technology |
|---|---|
| Language | Go 1.26 |
| HTTP framework | Gin |
| WhatsApp | [whatsmeow](https://github.com/tulir/whatsmeow) |
| Database | PostgreSQL 18 |
| ORM | GORM |
| Event brokers | RabbitMQ, NATS (optional) |
| Media storage | MinIO/S3 (optional) |
| Manager | React 19 + Vite + Tailwind |
| Docs | Swagger/OpenAPI |
| Container | Docker (`alpine:3.24`, ffmpeg, poppler-utils) |

### Project structure

```
evo-gofork/
├── cmd/evolution-go/       # Application entry point
├── pkg/                    # Go packages (routes, services, whatsmeow, ...)
├── evolution-go-manager/   # Manager SPA source (React/Vite)
├── manager/dist/           # Built manager assets served at /manager
├── docker/examples/        # Compose examples (incl. pull-only GHCR)
├── docs/                   # Generated Swagger
├── .github/workflows/      # GHCR publish
├── Dockerfile
├── Makefile
└── VERSION                 # Single source of truth for the version
```

---

## Telemetry

**None.** Upstream's license gate, heartbeat and telemetry were removed (see
`FORK_NOTES.md` §1): nothing here contacts Evolution Foundation or any other
third party on its own.

---

## Contributing & security

Issues and pull requests are welcome **in this repository**. For security
problems, do not open a public issue — see [`SECURITY.md`](./SECURITY.md).

## Acknowledgments

- [whatsmeow](https://github.com/tulir/whatsmeow) by [Tulir Asokan](https://github.com/tulir) — WhatsApp protocol library
- [Evolution Go](https://github.com/evolution-foundation/evolution-go) by Evolution Foundation — the upstream project this fork is based on
- [Evolution API](https://github.com/evolution-foundation/evolution-api) — Node.js sister project

## License

Apache License 2.0, with Evolution Go's additional conditions (LOGO/copyright
preservation and the Usage Notification requirement). See [`LICENSE`](./LICENSE).

**This is an unofficial community fork** — not affiliated with, endorsed by, or
an official release of Evolution Foundation. If you redistribute it or embed it
in another product, review the additional conditions, and note the tension
between `LICENSE` condition 1.a (do not remove the LOGO/copyright from the
console) and `TRADEMARKS.md` §4.2 (a *modified* UI must remove the brand assets
and use a distinct name). For any use not expressly permitted, contact
**suporte@evofoundation.com.br** (`TRADEMARKS.md` §5).

## Trademarks

"Evolution Foundation", "Evolution" and "Evolution Go" are trademarks of
Evolution Foundation. See [`TRADEMARKS.md`](./TRADEMARKS.md). Third-party
attributions are in [`NOTICE`](./NOTICE).
