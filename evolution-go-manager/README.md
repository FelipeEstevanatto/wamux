<h1 align="center">Evo-GoFork Manager</h1>

<div align="center">

[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react)](https://react.dev/)
[![TypeScript](https://img.shields.io/badge/TypeScript-7-3178C6?logo=typescript)](https://www.typescriptlang.org/)
[![Vite](https://img.shields.io/badge/Vite-8-646CFF?logo=vite)](https://vitejs.dev/)

</div>

> **Unofficial community fork.** This is the web panel for **Evo-GoFork**, an
> unofficial community fork of [Evolution Go](https://github.com/evolution-foundation/evolution-go).
> It is not affiliated with, endorsed by, or an official release of Evolution
> Foundation. See the repository's `NOTICE` and `TRADEMARKS.md`.

## About

Web interface for managing WhatsApp instances through the Evo-GoFork API
(instance management, QR code / pairing, messaging, webhooks, a system
dashboard and an API tester).

The source lives here and is built by the backend image (`oven/bun` stage) or
locally with `make manager-build`, which syncs the build into `manager/dist`.

## Features

- **Instance management** — create, connect, disconnect, delete
- **QR code / pairing** — QR and pairing-code authentication
- **Per-instance overview** — profile picture, contacts, chats, messages, device
- **Proxy settings** — set / test / reconnect / remove per instance
- **Messaging** — text, media, buttons, lists, carousels, events, products
- **Webhooks** — per-instance webhook configuration and event selection
- **Dashboard** — instances, messages, contacts and host metrics
- **API tester** — reads the live Swagger spec
- **About** — the admin-visible "uses Evolution Go" notice (license 1.b)

## Quick start

```bash
# Install (pnpm, bun or npm)
pnpm install        # or: bun install / npm install

# Development (Vite dev server)
pnpm dev            # http://localhost:5174

# Production build
pnpm build          # -> dist/
```

`make manager-build` from the repository root builds and copies `dist/` into
`manager/dist/` (preserving the fork-only `dashboard.html`).

## Authentication

1. Enter the **API URL** (e.g. `http://localhost:8080`)
2. Enter your **GLOBAL_API_KEY** from the `.env`
3. Credentials are stored in the browser's `localStorage`

No license activation is involved: this fork removed the gate entirely.

## Project structure

```
src/
├── pages/               # Home, Login, Dashboard, Instances, InstanceSettings,
│                        # About, ApiTester, Messages, Events, Settings
├── components/
│   ├── base/            # Layout, Header, Sidebar, GithubIcon, ErrorBoundary
│   └── instances/       # Instance cards, QR code, create/send/test modals
├── constants/branding.ts# Product name, fork/upstream links, disclaimers
├── services/api/        # Axios client + instances/server APIs
├── store/               # Zustand stores (auth, instances + overviews)
├── hooks/               # useAuth, useDarkMode, useServerStats
└── types/               # TypeScript interfaces
```

## Technology stack

| Component | Technology |
|-----------|-----------|
| Framework | React 19 |
| Language | TypeScript 7 |
| Build | Vite 8 |
| Styling | Tailwind CSS 4 |
| UI components | `@evoapi/design-system` |
| State | Zustand |
| HTTP | Axios |
| Forms | React Hook Form + Zod |
| Routing | React Router 7 |
| Icons | Lucide React |
| Notifications | Sonner |

## Integration with the API

- **REST** — every request carries the `apikey` header (global key for admin
  routes, the instance token for instance-scoped ones)
- **WebSocket** — real-time events at `/ws?token=<apiKey>&instanceId=<id>`

## License & attribution

Apache License 2.0 with Evolution Go's additional conditions. The upstream
project is [Evolution Go](https://github.com/evolution-foundation/evolution-go)
by Evolution Foundation; its copyright line and trademark notices are kept
intact.

© 2026 Evolution Foundation
