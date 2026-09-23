<p align="center">
  <a href="https://evolutionfoundation.com.br">
    <img src="./public/hover-evolution.png" alt="Evolution Foundation" />
  </a>
</p>

<h1 align="center">Evolution Go</h1>

<p align="center">
  High-performance WhatsApp API built in Go — part of the Evolution Foundation ecosystem.
</p>

> ### ⚠️ Unofficial community fork
>
> This repository is a **community fork** of Evolution Go. It is **not
> affiliated with, endorsed by, or an official release of Evolution
> Foundation**. Upstream: <https://github.com/evolution-foundation/evolution-go>.
>
> - Container image: `ghcr.io/felipeestevanatto/evolution-go-community`
> - What this fork changed: [`FORK_NOTES.md`](./FORK_NOTES.md) and [`CHANGELOG.md`](./CHANGELOG.md)
> - Attribution & brand terms: [`NOTICE`](./NOTICE) and [`TRADEMARKS.md`](./TRADEMARKS.md)

<p align="center">
  <a href="https://github.com/FelipeEstevanatto/evo-gofork/releases/latest"><img src="https://img.shields.io/github/v/release/FelipeEstevanatto/evo-gofork?include_prereleases&label=version&color=00ffa7" alt="Latest version" /></a>
  <a href="https://opensource.org/licenses/Apache-2.0"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License: Apache 2.0" /></a>
  <a href="https://docs.evolutionfoundation.com.br"><img src="https://img.shields.io/badge/Docs-upstream-00ffa7" alt="Documentation (upstream)" /></a>
  <a href="https://github.com/FelipeEstevanatto/evo-gofork/pkgs/container/evolution-go-community"><img src="https://img.shields.io/badge/Container-ghcr.io-blue" alt="Container image (this fork)" /></a>
</p>

<p align="center">
  <a href="https://evolutionfoundation.com.br">Website</a> &middot;
  <a href="https://docs.evolutionfoundation.com.br">Documentation</a> &middot;
  <a href="https://evolutionfoundation.com.br/community">Community</a> &middot;
  <a href="mailto:suporte@evofoundation.com.br">Support</a>
</p>


---

## About

**Evolution Go** is a high-performance WhatsApp API built in Go. Part of the Evolution Foundation ecosystem, it provides a robust, lightweight solution for WhatsApp integration using the [whatsmeow](https://github.com/tulir/whatsmeow) library.

## Part of the Evolution Foundation ecosystem

Evolution Go is one of the messaging engines maintained by Evolution Foundation. It is used as a WhatsApp provider by the [Evo CRM Community](https://github.com/evolution-foundation/evo-crm-community) and other projects in the ecosystem.

---

## Features

- **High performance** — built with Go for minimal resource usage
- **RESTful API** — clean, well-documented REST endpoints with Swagger
- **Real-time events** — WebSocket, Webhook, AMQP/RabbitMQ and NATS support
- **Media support** — images, videos, audio, documents with MinIO/S3 storage
- **Message storage** — optional PostgreSQL persistence
- **QR code pairing** — built-in QR code generation for device linking
- **No phone-home** — this fork has had the license gate/heartbeat and telemetry removed (see `FORK_NOTES.md`)
- **Docker ready** — production-ready Docker configuration

---

## Quick Start

### Docker (recommended)

Pull this fork's image from GHCR (no build needed):

```bash
# docker-compose.yml with: image: ghcr.io/felipeestevanatto/evolution-go-community:0.8.0
docker compose pull
docker compose up -d
```

Or build from source:

```bash
git clone https://github.com/FelipeEstevanatto/evo-gofork.git
cd evolution-go
git checkout fork/community-stable
make docker-build
make docker-run
```

### Local development

```bash
git clone https://github.com/FelipeEstevanatto/evo-gofork.git
cd evolution-go
git checkout fork/community-stable

# Setup, configure and run
make setup
cp .env.example .env
make dev
```

> Run `make help` to see all available commands. See [COMMANDS.md](./COMMANDS.md) for detailed workflows.

---

## Configuration

Create a `.env` file:

```env
# Server
SERVER_PORT=8080
CLIENT_NAME=evolution

# Security
GLOBAL_API_KEY=your-secure-api-key-here

# Database
POSTGRES_AUTH_DB=postgresql://postgres:password@localhost:5432/evogo_auth?sslmode=disable
POSTGRES_USERS_DB=postgresql://postgres:password@localhost:5432/evogo_users?sslmode=disable
DATABASE_SAVE_MESSAGES=false

# Logging
WADEBUG=DEBUG
LOGTYPE=console

# Optional
# AMQP_URL=amqp://guest:guest@localhost:5672/
# NATS_URL=nats://localhost:4222
# WEBHOOK_URL=https://your-webhook-url.com/webhook
# MINIO_ENABLED=true
# MINIO_ENDPOINT=localhost:9000
# MINIO_ACCESS_KEY=minioadmin
# MINIO_SECRET_KEY=minioadmin
```

| Variable | Description | Default |
|---|---|---|
| `SERVER_PORT` | Server port | `8080` |
| `CLIENT_NAME` | Client identifier | `evolution` |
| `GLOBAL_API_KEY` | API authentication key | **Required** |
| `DATABASE_SAVE_MESSAGES` | Enable message storage | `false` |
| `WADEBUG` | WhatsApp debug level | `INFO` |

---

## Activation

None required. This fork starts fully operational — there is no license
registration, no activation gate, and no heartbeat to any external server.
Just authenticate with your `GLOBAL_API_KEY`.

---

## API Documentation

Swagger UI available at:

```
http://localhost:8080/swagger/index.html
```

### Key Endpoints

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/instance/create` | Create WhatsApp instance |
| `GET` | `/instance/{name}/qrcode` | Get QR code for pairing |
| `POST` | `/message/sendText` | Send text message |
| `POST` | `/message/sendMedia` | Send media message |
| `GET` | `/instance/{name}/status` | Get instance status |
| `DELETE` | `/instance/{name}` | Delete instance |

---

## Project Structure

```
evolution-go/
├── cmd/evolution-go/     # Application entry point
├── pkg/
│   ├── safemap/         # Mutex-guarded maps shared across goroutines
│   ├── instance/        # Instance management
│   ├── message/         # Message handling
│   ├── sendMessage/     # Message sending
│   ├── routes/          # HTTP routes
│   ├── middleware/      # Auth & validation middleware
│   ├── config/          # Configuration
│   ├── events/          # Event producers (AMQP, NATS, Webhook, WS)
│   └── storage/         # Media storage (MinIO)
├── docs/                # Swagger documentation
├── Dockerfile
├── Makefile
└── VERSION
```

---

## Tech Stack

| Component | Technology |
|---|---|
| Language | Go 1.24+ |
| HTTP framework | Gin |
| WhatsApp | [whatsmeow](https://github.com/tulir/whatsmeow) |
| Database | PostgreSQL |
| ORM | GORM |
| Message queue | RabbitMQ, NATS |
| Object storage | MinIO/S3 |
| Documentation | Swagger/OpenAPI |
| Container | Docker |

---

## Documentation

| Resource | Link |
|---|---|
| This fork (source) | [FelipeEstevanatto/evo-gofork](https://github.com/FelipeEstevanatto/evo-gofork) |
| This fork (container) | [ghcr.io/felipeestevanatto/evolution-go-community](https://github.com/FelipeEstevanatto/evo-gofork/pkgs/container/evolution-go-community) |
| What this fork changed | [FORK_NOTES.md](./FORK_NOTES.md) · [CHANGELOG.md](./CHANGELOG.md) |
| Upstream project | [evolution-foundation/evolution-go](https://github.com/evolution-foundation/evolution-go) |
| Upstream docs | [docs.evolutionfoundation.com.br](https://docs.evolutionfoundation.com.br) |
| Attribution & brand terms | [NOTICE](./NOTICE) · [TRADEMARKS.md](./TRADEMARKS.md) |
| Contributing | [CONTRIBUTING.md](./CONTRIBUTING.md) |
| Security | [SECURITY.md](./SECURITY.md) |

---

## Usage notification (deployers)

Evolution Go's license (`LICENSE`, additional condition **1.b**) requires any
system that uses it to show a **clear, administrator-visible notification that
Evolution Go is being utilized**, reachable from the system's documentation or
settings page.

This fork satisfies it in the manager: an admin can open **Sobre / About**
(`/manager/about`), which states that the system uses Evolution Go and links the
license and attribution.

**If you embed this image (or the API) in another product**, you inherit that
obligation for *your* users: surface an equivalent admin-visible notice. The
image ships `LICENSE`, `NOTICE`, `TRADEMARKS.md` and `FORK_NOTES.md` in `/app`.

---

## Hosting

This fork does not operate any hosting service. (The upstream project has a
HostGator partnership; see the [upstream repository](https://github.com/evolution-foundation/evolution-go).)

---

## Telemetry

**This fork has no telemetry.** Upstream's license gate, heartbeat and
telemetry were removed (see `FORK_NOTES.md` §1): nothing here contacts Evolution
Foundation or any other third party on its own.

---

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines on how to submit issues, propose features, and open pull requests.

Join our [community](https://evolutionfoundation.com.br/community) to discuss ideas and collaborate.

---

## Security

For security issues, **do not open a public issue**. Email **suporte@evofoundation.com.br** or use GitHub's private vulnerability reporting. See [SECURITY.md](./SECURITY.md) for details.

---

## Acknowledgments

- [whatsmeow](https://github.com/tulir/whatsmeow) by [Tulir Asokan](https://github.com/tulir) — WhatsApp protocol library
- [Evolution API](https://github.com/evolution-foundation/evolution-api) — Node.js sister project

---

## License

Evolution Go is licensed under the Apache License 2.0, with additional
brand-protection conditions (LOGO/copyright preservation and the Usage
Notification requirement). See [LICENSE](./LICENSE) for full details.

**This is an unofficial community fork.** It is not affiliated with, endorsed
by, or an official release of Evolution Foundation. If you redistribute it or
embed it in another product, review the additional conditions — and note the
tension between `LICENSE` condition 1.a (do not remove the LOGO/copyright from
the console) and `TRADEMARKS.md` §4.2 (a *modified* UI must remove the brand
assets and use a distinct name). For any use not expressly permitted, contact
**suporte@evofoundation.com.br** (`TRADEMARKS.md` §5).

For licensing inquiries, contact **suporte@evofoundation.com.br**.

## Trademarks

"Evolution Foundation", "Evolution" and "Evolution Go" are trademarks of Evolution Foundation. See [TRADEMARKS.md](./TRADEMARKS.md) for the brand assets policy.

Third-party attributions are documented in [NOTICE](./NOTICE).

---

<p align="center">
  Made by <a href="https://evolutionfoundation.com.br">Evolution Foundation</a> · © 2026
</p>
