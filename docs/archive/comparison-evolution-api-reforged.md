# WaMux vs Evolution API Reforged — full comparison

**Date:** 2026-11-22
**WaMux:** `1.0.1` (Go 1.27, whatsmeow, fork of `evolution-foundation/evolution-go` 0.7.2)
**Reforged:** `3.3.0` (Node 24, TypeScript, Baileys 7.0.0-rc14, fork of `evolution-foundation/evolution-api`)
**Compared repo:** <https://github.com/Operational-Intelligence-Inc/evolution-api-reforged>

---

## 0. TL;DR

- These are **two different implementations of the same product category**, not two
  branches of one project. WaMux is a Go/whatsmeow rewrite of Evolution Go;
  Reforged is a hardened Node/Baileys distribution of Evolution API v2.
- **Reforged wins on integration breadth and ecosystem compatibility**: official
  Cloud API, 8 chatbot integrations, Kafka/SQS/Pusher, Redis, MySQL, and drop-in
  Evolution API v2 compatibility.
- **WaMux wins on runtime, security, tests, data correctness, manager and docs**:
  one Go binary, no telemetry, 841 tests + 8 fuzzers + 72 benchmarks, stronger
  auth/token handling, richer history/chat APIs, and a maintainable in-repo UI.
- Their most valuable *fork-specific* work is **Baileys reliability engineering**:
  a durable event queue, per-instance health verdict, socket watchdog, a
  stable-connection reconnect reset, conflict limits, and history-row fallback.
- The best adoptions for WaMux are **small and high-leverage**: history batch
  fallback, stable-window reconnect reset, 440/non-retryable handling, configurable
  10 GB history, per-instance health/queue visibility, graceful event drain,
  instance-scoped WebSocket tickets, and a pre-release/promote release pipeline.
- The big one — a durable inbound event queue — is worth a **design spike**, not a
  blind port. Their implementation depends on Redis; WaMux can choose Postgres or
  NATS JetStream and should first measure whether deploys/crashes actually lose
  messages.
- Do **not** copy wholesale: Cloud API (unless strategic), MySQL, telemetry,
  default Sentry, their prebuilt manager submodule, voice calls, or their
  Redis-coupled event queue.

---

## 1. Scope and method

Reviewed both repositories end to end at the level needed for a product/technical
comparison:

- READMEs, CHANGELOGs, `.env.example`, Dockerfiles, CI workflows.
- All route groups (`pkg/routes/routes.go`, `src/api/routes/*` and integration routes).
- Integration directories (channels, chatbots, events, storage).
- Reliability code: reconnect policy, health/watchdog, event queue, history
  persistence, status handling.
- Tests, benchmarks, fuzzers, security workflows, release workflows.
- Fork-specific commits and CHANGELOG entries (`3.0.0` → `3.3.0` and unreleased).

Line counts (approximate, excludes generated/vendor code):

| | WaMux | Reforged |
|---|---:|---:|
| Source | 67.9k Go + 20.0k TS (manager) | 36.0k TS |
| Tests | 179 files, 841 tests, 8 fuzzers, 72 benchmarks | 8 files, 121 tests |
| Docs | 43 Markdown wiki pages + generated Swagger | 10 Markdown files, relies on upstream docs |
| Version | 1.0.1 | 3.3.0 |

---

## 2. Scorecard

| Category | Winner | Why |
|---|---|---|
| Runtime efficiency | **WaMux** | Single Go binary; no Node runtime, no Baileys JS stack, lower memory/CPU, cgroup-aware GC tuning. |
| Protocol reliability | **Reforged** (edge) | Durable event queue, health verdict, watchdog, stable-window reconnect, conflict limits. WaMux has good backoff and event-driven recovery but no durable replay. |
| Data correctness | **WaMux** | Keyset history pagination, LID/PN canonicalization, chat state/unread, albums, idempotent sends, edit/revoke semantics, media integrity, retention, counter rollups. |
| Security | **WaMux** | Always-on SSRF for caller URLs, token encryption at rest, per-instance HMAC/S3, passkeys, upload guards, rate limits, WS tickets, signed downloads, non-root image. Reforged has good targeted fixes but remains telemetry-on, root container, and has an ineffective metrics IP allowlist. |
| API breadth | **Reforged** | Cloud API, 8 chatbots, Kafka/SQS/Pusher, templates, catalog, business profile. |
| API depth / correctness | **WaMux** | More precise history filters/keyset, communities, newsletters, poll results, group join requests, presence subscribe, mark played, albums, idempotency. |
| Observability | **WaMux** (overall) / **Reforged** (per-instance) | WaMux has `/server/health`, rich `/metrics`, inventory, logs, proxy checks. Reforged has a superior per-instance health verdict and queue lag. |
| Multi-tenancy | **WaMux** | Token hashes + AES-GCM, per-instance S3/HMAC, per-instance send guards, ownership locks. Reforged fixed several tenant scoping bugs but stores tokens plainly. |
| Multi-node | **WaMux** | Postgres advisory locks + visible leases + heartbeat. Reforged is single-process. |
| Manager | **WaMux** | In-repo React 19, messages UI, events feed, API tester, i18n, tests. Reforged uses the upstream prebuilt manager as a submodule. |
| Docs | **WaMux** | 43 wiki pages, API reference, deployment/security guides. Reforged relies on upstream docs. |
| Tests / CI | **WaMux** | 841 tests, race + shuffle, fuzzers, benchmarks, CodeQL, govulncheck, gitleaks, Trivy image scan, image smoke test. Reforged has 121 focused tests and a useful but lighter CI. |
| Integrations / ecosystem | **Reforged** | Drop-in Evolution API v2 API; huge ecosystem of clients/integrations. |
| Release discipline | **Reforged** | Pre-release on merge + manual promote of `latest`; fails on an existing version. WaMux pushes `latest` immediately. |
| Deployment hardening | **WaMux** | Non-root user, healthcheck, migrations at boot, multiple compose variants. Reforged runs as root, no healthcheck, migrations via shell entrypoint. |
| Privacy | **WaMux** | No telemetry, no license server, no heartbeat. Reforged keeps upstream telemetry on by default. |

---

## 3. What Reforged does best

### 3.1 Drop-in Evolution API v2 compatibility

This is their single biggest advantage. Because Reforged is a fork of the Node
Evolution API, it keeps the same routes, payloads, webhook event names, manager
API and ecosystem. Existing Evolution API clients, CRM connectors, n8n nodes and
managers can switch to it with little or no change. WaMux's API is its own shape
(Evolution Go lineage), so it cannot be a drop-in replacement.

Evidence: `src/api/routes/*`, `src/api/integrations/*`, `README.md`.

### 3.2 Integration breadth

- **Channels:** Baileys, official WhatsApp Cloud API, and a stateless
  webhook-driven "Evolution" channel.
- **Chatbots:** Chatwoot, Typebot, OpenAI, Dify, Flowise, N8N, EvoAI,
  EvolutionBot.
- **Event sinks:** webhook, WebSocket, RabbitMQ, NATS, Kafka, SQS, Pusher.
- **Storage:** S3/MinIO.
- **Infrastructure:** Redis cache, PostgreSQL/MySQL/PgBouncer.

Evidence: `src/api/integrations/channel/`, `src/api/integrations/chatbot/`,
`src/api/integrations/event/`, `prisma/`.

### 3.3 Baileys reliability engineering (the fork's real contribution)

This is the part worth studying, regardless of stack:

1. **Per-instance connection health** (`connection-health.ts`, 3.3.0).
   In-memory facts: last frame, last keep-alive answer, reconnect opens/drops/440s
   in a 10-minute window, last inbound message, last outbound ack, unacked sends,
   event-queue running age. A `verdict` of `ok | reconnecting | looping | stalled
   | closed` plus human-readable `reasons`. Exposed in
   `GET /instance/connectionState`.
2. **Socket watchdog** (`connection-watchdog.ts`, 3.3.0).
   When the state reads `open` but the websocket is closed or keep-alive is
   silent, rebuild that one instance's socket. Modes `off | report | on`
   (default `report`), 30 s confirmation, 10 s checks, 1 s→5 min backoff, 5
   consecutive rebuilds max. Never logs out, never clears the session.
3. **Durable per-instance event queue** (`baileys-event-queue.ts`, 3.1.0).
   Redis Streams: XADD before processing, XDEL after success, replay on the first
   socket, per-item deadline, max entry size, max length, in-memory fallback.
   This prevents losing messages across a restart/deploy.
4. **Reconnect policy** (`reconnect.policy.ts`, 3.0.0/3.1.0).
   Exponential 1 s→5 min, no attempt cap for a linked session, stable-connection
   reset after 60 s, non-retryable codes `401/403/402/406`, QR limit, and a
   `RECONNECT_CONFLICT_LIMIT` for consecutive 440s (default 10, session kept).
5. **History batch resilience** (3.2.0).
   Chunked retry then row-by-row insert so one bad row does not drop a whole
   history batch; skipped/failed counts logged; oversize batches split into parts
   (3.3.0) under the queue entry cap.
6. **Early sent-status buffer** (`pending-message-updates.ts`, 3.3.0).
   Receipts that arrive before the sent message row is stored are held in a
   bounded TTL buffer and replayed. Status only moves forward
   (`PENDING → ERROR → SERVER_ACK → DELIVERY_ACK → READ → PLAYED`).
7. **Full-history pairing** (3.2.0).
   `fullSyncDaysLimit: 3650`, `fullSyncSizeMbLimit: 10240`, Windows Desktop
   identity when `syncFullHistory` is on, plus per-batch INFO logging.

### 3.4 Release discipline

Their unreleased workflow separates **publishing** from **promotion**:

- A merge into `prod` publishes `<version>` and `sha-<short>` and creates a
  GitHub **pre-release**; `latest` and `<major>.<minor>` do not move.
- A manual **Promote release** workflow retags the same digest to `latest` and
  the minor tag, then marks the GitHub Release as latest.
- It refuses a version with no release, one already promoted, or one older than
  the current latest.

Evidence: `.github/workflows/publish_prod_image.yml`,
`.github/workflows/promote_release.yml`, `CHANGELOG.md` "Unreleased".

### 3.5 Operational writing

Their CHANGELOG entries read like incident reports: symptom, root cause,
mechanism, limits, configuration, and live-test evidence. This is excellent for
operators and future maintainers. WaMux's CHANGELOG is also detailed, but the
Reforged entries are a good model for "why this exists" documentation.

### 3.6 Targeted security hardening

- Fail-closed global API key (missing/blank/known example values refuse boot).
- Route-matched instance authorization; `instanceName` cannot be overridden by
  body/query.
- Chatbot routes mounted only when their feature flag is on.
- Chatwoot webhook HMAC token derived from the global key.
- WebSocket sockets bound to the handshake principal.
- Typebot receives the instance token, not the global key.
- Tenant-scoped S3/Chat/Message/Contact lookups.
- Parameterized Chatwoot import SQL.
- Signal session keys redacted from console output.
- Unique message key index + dedupe migration.
- SSRF-filtered media fetch with DNS pinning.

### 3.7 Caveats in Reforged

Not everything is stronger:

- **Telemetry is on by default** (`TELEMETRY_ENABLED=true`) and posts route paths
  to an external endpoint. For a self-hosted fork this is a privacy regression.
- **Runs as root**, no Docker `HEALTHCHECK`, migrations via a shell entrypoint.
- **Metrics IP allowlist is a no-op**: `allowedIPs.filter(...) === 0` compares an
  array to `0` and never rejects; it also trusts raw `X-Forwarded-For`.
- **MySQL is not at parity** with the fork's 2026 changes; those migrations are
  PostgreSQL/PgBouncer-first.
- **Meta and Evolution channel webhooks** are mounted without API-key guards;
  Meta's GET verify token is the only check visible in the router.
- **Publish workflow does not run tests**; a direct `prod` merge bypasses the
  test job (tests run on `main`/`develop` pushes and PRs).
- **Health is process-local** and resets on restart; queue replay can re-trigger
  webhooks/chatbots (documented).
- **No standalone `/health` endpoint**; readiness must be derived from `/`,
  `/instance/connectionState` or `/metrics`.

---

## 4. What WaMux does best

### 4.1 Runtime and deployment

- Go single binary on whatsmeow; no Node/Baileys process, no JS dependency tree.
- cgroup-derived `GOMEMLIMIT`/`GOGC`, idle-heap reclaimer, bounded pools for
  persistence, media and background work.
- Non-root Docker image (`USER wamux`), `HEALTHCHECK`, versioned migrations at
  boot, multiple compose variants (basic, full with RabbitMQ/NATS/MinIO, Swarm
  behind Traefik).
- Optional SQLite auth DB for development; Postgres for production.

### 4.2 Engineering rigor

- **179 test files / 841 tests / 8 fuzzers / 72 benchmarks**.
- CI runs `go vet`, `go test -race -shuffle=on`, govulncheck, golangci-lint,
  gofmt, docs check, CodeQL, dependency review, 30 s fuzz targets, manager
  lint/build/tests, gitleaks.
- Image workflow builds, boots against Postgres, runs a smoke test, and scans
  with Trivy for HIGH/CRITICAL.
- Benchmarks cover the real HTTP pipeline, group-send crypto, identity cache,
  persistence, webhook signing/pooling, idempotency, rate limiting, auth,
  storage and idle memory.

### 4.3 Security

- **SSRF always on for caller-supplied URLs**: DNS pinning, redirect
  revalidation, blocks loopback/RFC1918/link-local/CGNAT/multicast/reserved.
- **Instance tokens encrypted at rest**: HMAC-SHA256 lookup hash + AES-256-GCM
  ciphertext; plaintext only in memory.
- **Webhook HMAC** per instance or global, encrypted keys, replay CLI.
- **Per-instance S3** with encrypted secret.
- **Passkey pairing** support for accounts that require WebAuthn.
- **Upload guard** (default 2 GiB), per-instance send guard (rate + concurrency),
  per-credential/IP rate limiting, admin bucket, brute-force protection.
- **WebSocket tickets** (30 s HMAC-signed) instead of putting the global key in
  the URL; **signed media download links** (5 min).
- **Media integrity**: inbound size/SHA-256 verification before storage.
- **No license server, heartbeat or telemetry.**

### 4.4 Data correctness and product depth

- Keyset history pagination (`before` + `before_id`), exact pages, filters
  (`after`, `until`, `type`, `from_me`), search, capabilities contract.
- LID ↔ phone canonicalization both directions, chat merge, unread counts,
  archived/pinned/muted/marked-unread state, read position.
- **Albums** (2–30 items, per-item captions, rollback on partial failure).
- **Idempotent sends** with canonical JSON fingerprints, native WhatsApp id
  reservation, `202` for in-flight, `409` for a different body, status endpoint.
- **Edit/revoke semantics**: author check, chat check, 15-minute window,
  tombstones, status monotonicity.
- **Communities, newsletters, poll results, group join requests, labels,
  presence subscribe, mark played** — areas where Evolution API has no or weaker
  endpoints.
- Message counter rollup + trigger, chat-list recursive CTE, retention cleanup,
  media retention.
- Multi-node ownership: Postgres advisory locks + visible leases + heartbeat.

### 4.5 Manager and docs

- In-repo React 19 manager (Vite, TanStack Query, Tailwind, Zustand, i18n):
  dashboard, instances, messages with unread dividers/search/media/albums,
  live events, API tester, About/legal.
- 43 wiki pages: fundamentals, core concepts, API guides, advanced features,
  reference, deployment/security, development.
- Generated Swagger UI.

### 4.6 API surface advantages

- Albums, carousel, product messages, status text/media, communities,
  newsletters, poll results, group join-request management, labels/unlabels,
  presence subscribe, mark played, message status, idempotent sends, per-instance
  S3/HMAC, fleet inventory, proxy test, account limits.

---

## 5. Feature matrix

Legend: ✅ full, ⚠️ partial/different, ❌ absent, ➖ not applicable.

| Capability | WaMux | Reforged | Notes |
|---|---|---|---|
| WhatsApp Web transport | ✅ whatsmeow | ✅ Baileys | Different libraries; WaMux is Go. |
| Official Cloud API | ❌ | ✅ | Reforged has Meta webhook + Graph sending + templates. |
| Stateless webhook channel | ❌ | ✅ | Reforged "Evolution" channel. |
| PostgreSQL | ✅ | ✅ | Both production-grade. |
| MySQL | ❌ | ⚠️ | Reforged upstream supports it; fork migrations are PG-first. |
| PgBouncer | ❌ | ✅ | Reforged provider option. |
| SQLite | ⚠️ | ❌ | WaMux auth DB only. |
| Redis cache | ❌ | ✅ | Reforged optional; WaMux uses in-process caches. |
| Durable inbound event queue | ❌ | ✅ | Reforged Redis Streams; WaMux in-memory. |
| Webhook sink | ✅ | ✅ | WaMux per-endpoint isolation, HMAC, DLQ; Reforged retries/JWT. |
| WebSocket | ✅ | ✅ | WaMux tickets but global-key scope; Reforged instance-bound. |
| RabbitMQ | ✅ | ✅ | Both. |
| NATS | ✅ | ✅ | WaMux has JetStream + dedupe. |
| Kafka | ❌ | ✅ | Reforged per-instance/global. |
| SQS | ❌ | ✅ | Reforged. |
| Pusher | ❌ | ✅ | Reforged. |
| S3/MinIO | ✅ | ✅ | Both. |
| Per-instance S3 | ✅ | ❌ | WaMux advantage. |
| Typebot | ✅ | ✅ | WaMux deliberately minimal. |
| Chatwoot | ❌ | ✅ | Reforged. |
| OpenAI chatbot + Whisper | ❌ | ✅ | Reforged. |
| Dify / Flowise / N8N / EvoAI / EvolutionBot | ❌ | ✅ | Reforged. |
| Text/media/audio/PTV | ✅ | ✅ | Equivalent. |
| Albums | ✅ | ❌ | WaMux advantage. |
| Buttons/lists | ✅ | ✅ | Both. |
| Carousel | ✅ | ❌ | WaMux advantage. |
| Polls + results | ✅ | ⚠️ | Reforged sends polls; WaMux also returns results. |
| Product messages | ✅ | ✅ | Both send; Reforged also fetches catalog/collections. |
| Templates | ❌ | ✅ | Cloud API only. |
| Status/stories | ✅ | ✅ | Both. |
| Groups | ✅ | ✅ | WaMux has join-request management; Reforged has revoke/accept invite. |
| Communities | ✅ | ❌ | WaMux advantage. |
| Newsletters | ✅ | ❌ | WaMux advantage. |
| Labels | ✅ | ⚠️ | WaMux more granular (chat/message/edit). |
| Calls | ⚠️ reject only | ⚠️ offer/Wavoip | Both partial; neither is a full VoIP stack. |
| History readback | ✅ keyset/filters | ✅ findMessages | WaMux more precise. |
| History sync depth | 3650 d / 2 GB | 3650 d / 10 GB | Reforged deeper cap. |
| Chat state/unread | ✅ | ✅ | Both. |
| LID mapping | ✅ | ✅ | Both. |
| Idempotent sends | ✅ | ❌ | WaMux advantage. |
| Edit/revoke correctness | ✅ strict | ✅ author check | Both hardened. |
| HMAC webhooks | ✅ | ⚠️ JWT | Different mechanisms. |
| SSRF protection | ✅ caller URLs | ✅ media URLs | Both incomplete for configured endpoints. |
| Token encryption at rest | ✅ | ❌ | WaMux advantage. |
| Passkey pairing | ✅ | ❌ | WaMux advantage. |
| Rate limiting | ✅ | ✅ | Both. |
| Instance-scoped WebSocket | ⚠️ | ✅ | WaMux only global key/ticket; docs mismatch. |
| `/server/health` | ✅ | ❌ | WaMux advantage. |
| Metrics | ✅ rich | ⚠️ basic | WaMux richer; Reforged's IP allowlist is broken. |
| Per-instance health verdict | ⚠️ basic | ✅ | Reforged advantage. |
| Queue lag/health | ⚠️ pool depth | ✅ per-instance | Reforged advantage. |
| Socket watchdog | ❌ | ✅ | Reforged advantage; WaMux handles keepalive events. |
| Reconnect backoff | ✅ | ✅ | Different schedules; Reforged has stable/conflict policy. |
| History row fallback | ❌ | ✅ | Reforged advantage. |
| Idle client eviction | ❌ | ✅ | Reforged `DEL_INSTANCE`. |
| Multi-node single-writer | ✅ | ❌ | WaMux advantage. |
| Release promotion | ❌ | ✅ | Reforged advantage. |
| Non-root image + healthcheck | ✅ | ❌ | WaMux advantage. |
| Manager | ✅ in-repo | ✅ upstream prebuilt | WaMux maintainable; Reforged broader integration UI. |
| Tests/CI | ✅ strong | ⚠️ focused | WaMux much larger. |
| Telemetry | none | on by default | WaMux advantage. |

---

## 6. Their fork-specific improvements — adopt / don't adopt

| # | Improvement | Why it matters | Effort | Verdict |
|---|---|---|---|---|
| 1 | Stable-connection reset (60 s) before clearing reconnect backoff | A 440 loop briefly reaches `open`; resetting on any `Connected` defeats the backoff. WaMux calls `reconnectSucceeded` immediately (`whatsmeow.go:2187`). | 0.5–1 d | **Adopt now** |
| 2 | Non-retryable close codes + 440 conflict limit + keep session | Stops endless loops after logout/ban/session replacement; keeps the session for a manual reconnect. | 1–2 d | **Adopt now** |
| 3 | History batch chunk/row fallback + skipped counts | WaMux `InsertHistoryMessages` returns on the first failed chunk (`message_repository.go:541`); one bad row drops the whole batch. | 0.5–1 d | **Adopt now** |
| 4 | 10 GB configurable history + INFO batch logs | WaMux asks for 3650 days but caps at 2 GB (`whatsmeow.go:1266`); heavy accounts may truncate. | 0.5 d | **Adopt now** |
| 5 | Graceful event drain on shutdown | WaMux drains the persistence pool but not in-flight event handlers; SIGTERM can drop work. | 1–2 d | **Adopt now** |
| 6 | Per-instance health verdict + queue lag | Ops cannot tell "open but broken" from healthy; `InstanceHealth.Reconnecting` is never populated. | 2–4 d | **Adopt next** |
| 7 | Watchdog `report`/`on` | Defense in depth for silent stalls. WaMux already reacts to `KeepAliveTimeout`/`StreamError`, so lower urgency. | 2–3 d | **Adopt next (report first)** |
| 8 | Instance-scoped WebSocket tickets | Fixes the docs/implementation mismatch and prevents a tenant token from seeing all instances. | 2–3 d | **Adopt next** |
| 9 | Idle client eviction (`DEL_INSTANCE`) | Frees memory for large fleets after long disconnects; DB row and session stay. | 1–2 d | **Consider** |
| 10 | Release pre-release + manual promote | Prevents a bad main merge from becoming `latest`; avoids overwriting an existing version. | 0.5 d | **Adopt now** |
| 11 | Fail-closed / weak key warning | WaMux generates a key when empty (good); warn on known/weak values. | 0.5 d | **Adopt** |
| 12 | Chatbot route gating | Only mount integrations that are enabled. WaMux only has Typebot, so low impact. | 0.5 d | **Adopt with new integrations** |
| 13 | Early sent-status buffer | WaMux's status-only upsert already handles early receipts; no need to port. | — | **Don't port** |
| 14 | Played status in the lifecycle | WaMux handles Read/Delivered only; `ReceiptTypePlayed` is ignored. | 1 d | **Adopt (optional)** |
| 15 | Durable inbound event queue | Prevents message loss across restart/deploy; also gives deadlines and replay. High complexity, Redis coupling in their design. | 2–4 wk | **Design spike** |
| 16 | Generic HTTP chatbot adapter (EvolutionBot/N8N style) | One adapter covers N8N/Dify/Flowise/custom; cheaper than porting 6 integrations. | 1–2 wk | **Consider** |
| 17 | Kafka producer | Most-requested missing sink; WaMux's producer interface is clean. | 3–5 d | **Consider on demand** |
| 18 | Redis cache | Useful for multi-node shared state, rate limits, idempotency; adds a dependency. | 1–2 wk | **Consider later** |
| 19 | Cloud API provider | Strategic, but a different transport/message model and a large surface. | 4–8 wk | **Strategic decision** |
| 20 | MySQL support | Low value for a Postgres-first project; migrations/triggers are PG-specific. | 2–4 wk | **Skip for now** |
| 21 | Voice calls / Wavoip | Unreliable unofficial stack; WaMux only rejects calls. | — | **Skip** |
| 22 | Sentry / telemetry | Sentry optional; telemetry contradicts WaMux's no-telemetry stance. | 1 d / — | **Optional Sentry, no telemetry** |
| 23 | Upstream manager submodule | WaMux's in-repo manager is more maintainable. | — | **Skip** |
| 24 | Business catalog/profile endpoints | Useful only for product/business use cases; whatsmeow support must be verified. | 2–4 d | **Low priority** |

---

## 7. Gaps in WaMux surfaced by the comparison

These are not necessarily things Reforged solved; they are weaknesses the
comparison makes obvious.

### 7.1 Reliability

1. **History batch atomicity** — `InsertHistoryMessages` has no row fallback.
2. **Reconnect reset** — `reconnectSucceeded` on any `Connected`, so a transient
   open resets the loop counter.
3. **440 / `StreamReplaced` handling** — the handler returns without updating the
   stored state or recording a reason; verify a subsequent `Disconnected` always
   follows. If not, the instance can look connected after a 440.
4. **No conflict/non-retryable policy** — no equivalent of
   `RECONNECT_CONFLICT_LIMIT` or `CODES_TO_NOT_RECONNECT`.
5. **No graceful event drain** — shutdown stops HTTP and drains the persistence
   pool, but in-flight event handlers are not awaited.
6. **No durable queue** — a crash between the WhatsApp ack and the DB write loses
   the event.
7. **2 GB history cap** — may truncate heavy accounts.
8. **`InstanceHealth.Reconnecting` unused** — the health view cannot show
   "reconnecting" or a verdict.

### 7.2 Security

1. **SSRF is not applied to tenant-configured endpoints** — webhook URLs,
   Typebot URLs and per-instance S3 endpoints use plain HTTP/MinIO clients. A
   tenant token can point a webhook at `169.254.169.254` or an internal service
   and receive events (including `instanceToken`). Apply the SSRF dialer with an
   explicit private-network allowlist/opt-in.
2. **S3 public-read policy by default** — `setBucketPolicy` grants public
   `s3:GetObject` unless `SkipBucketPolicy` is set. Default to presigned URLs;
   make public-read opt-in.
3. **Event payloads include `instanceToken`** — same compatibility behavior as
   Evolution API's `apikey`. Add an opt-out (`WEBHOOK_INCLUDE_TOKEN=false`) and
   document the risk.
4. **WebSocket instance scoping** — `/ws` accepts only the global key or a global
   ticket, while the docs show `?token=<instance token>`. Either implement
   instance-scoped tickets or fix the docs.
5. **No audit log** for admin actions (instance create/delete, proxy/S3/HMAC
   changes, token access).
6. **No request IDs/tracing** across API → webhook/queue integrations.

### 7.3 Product / integrations

1. No Cloud API, Chatwoot, OpenAI, Dify, Flowise, N8N, EvoAI, EvolutionBot.
2. No Kafka, SQS or Pusher sinks.
3. No Redis or MySQL.
4. No templates, catalog fetch, business profile, voice-call stack.
5. Manager UI missing for Typebot, S3, HMAC, labels, communities, newsletters,
   poll results, inventory/limits.
6. No global cross-chat search; search is per conversation.
7. No outbound media upload to S3 (local/stream only).
8. No retention for `send_operations`, `chat_reads`, `chat_states`, Typebot
   sessions or poll votes.
9. No async send outbox; sends are synchronous.

### 7.4 Observability

1. No per-instance verdict/reasons, last frame/ack, or event-queue age.
2. No HTTP latency histograms, per-route/status counts, webhook success/failure
   counters, event-type counters, DB pool metrics.
3. No dependency health checks (DB, RabbitMQ, NATS, S3) in `/server/health`.
4. No tracing/Sentry/error reporting.

### 7.5 Docs and consistency

1. `docs/wiki/referencia/faq.md` says "both use whatsmeow" — Evolution API uses
   Baileys. Fix.
2. `docs/wiki/recursos-avancados/events-system.md` says webhook retries use a
   fixed 30 s interval; code uses exponential 2–60 s.
3. The same doc shows `ws://.../ws?token=<instance token>`; the code only accepts
   the global key or a global ticket.
4. Passkey doc says CORS liberates `web.whatsapp.com`; the default
   `CORS_ALLOWED_ORIGINS` is empty (same-origin). The extension may bypass CORS
   via host permissions, but the doc should state the requirement.
5. `WEBHOOK_FILES` default in code is `true`; ensure the docs match.

---

## 8. Prioritized roadmap

### P0 — this week (small, high leverage)

1. **History batch fallback** — chunked insert, then row-by-row; log
   inserted/failed/skipped. Add a test with one poison row.
2. **Stable-connection reset** — do not clear the reconnect counter on a
   transient `Connected`; clear it after 60 s of continuous connection. Add
   table-driven tests for a 440 loop.
3. **440/non-retryable policy** — on `StreamReplaced`, mark disconnected with a
   clear reason and suppress auto-reconnect; add a conflict counter and a
   `RECONNECT_CONFLICT_LIMIT` (default 10) that keeps the session.
4. **History limits config** — `HISTORY_SYNC_DAYS` (3650), `HISTORY_SYNC_MAX_MB`
   (10240), `HISTORY_STORAGE_QUOTA_MB` (10240); log each batch at INFO with
   counts and progress.
5. **Graceful event drain** — stop clients first, wait for in-flight handlers
   with a deadline, then drain persistence/webhook/NATS.
6. **Release promotion** — publish `<version>` + `sha` as a pre-release; add a
   manual promote workflow for `latest`/minor; fail on an existing version.
7. **Weak key warning** — log a warning for known example/very short
   `GLOBAL_API_KEY` values.
8. **Instance-scoped WebSocket tickets** — issue a ticket bound to the instance
   token; validate the instance on `/ws`; update docs.
9. **Played status** — handle `ReceiptTypePlayed`, rank it above `Read`, update
   the manager.

### P1 — next 2–4 weeks

10. **Per-instance health** — last event/ack, reconnect counts, queue age,
    verdict/reasons; expose in `/instance/status`, `/instance/inventory`,
    `/metrics`; fill `Reconnecting`.
11. **Watchdog report mode** — log "would rebuild" for stalled instances; enable
    `on` only after field validation.
12. **Idle client eviction** — optional `INSTANCE_IDLE_EVICT_MINUTES`; free
    memory for long-disconnected instances.
13. **Manager UI gaps** — Typebot, S3, HMAC, labels, communities, newsletters,
    poll results, inventory/limits.
14. **Security hardening** — SSRF for configured endpoints with an explicit
    allowlist; S3 presigned-by-default; opt-out for `instanceToken` in payloads.
15. **Docs fixes** — FAQ, WebSocket token, retry interval, passkey CORS,
    `WEBHOOK_FILES`.
16. **Test coverage** — reconnect policy, history fallback, health/watchdog with
    injected clock.

### P2 — 1–2 months

17. **Generic chatbot adapter** — HTTP webhook provider with triggers, debounce,
    ignoreJids, stop keywords; covers N8N/Dify/Flowise/custom without six ports.
18. **Durable event queue spike** — compare Postgres inbox, NATS JetStream and
    Redis Streams; success criteria: zero lost messages across deploy/crash,
    bounded overhead, replay idempotence.
19. **Kafka producer** — if users ask; use the existing producer interface.
20. **Observability** — HTTP latency histograms, webhook success/failure, event
    counters, DB pool metrics, dependency checks, request IDs.
21. **Audit log** — admin actions and credential changes.

### P3 — strategic

22. **Cloud API provider** — only if the product needs official Meta messaging.
23. **MySQL** — only if demanded; the schema/queries are Postgres-specific.
24. **Templates/catalog/business profile** — follow Cloud API or product demand.
25. **Redis cache** — only if multi-node shared state becomes a requirement.
26. **Voice calls** — skip unless a reliable stack is chosen.

---

## 9. Concrete code notes

| Area | File | Note |
|---|---|---|
| History fallback | `pkg/message/repository/message_repository.go:541` | `InsertHistoryMessages` returns on first chunk error. Add chunk → row fallback and counts. |
| History limits | `pkg/whatsmeow/service/whatsmeow.go:1264` | 3650 d / 2048 MB / 2048 MB quota; make configurable, raise cap. |
| History logging | `pkg/whatsmeow/service/message_lifecycle.go:130` | Logs only "Stored N"; add batch type/progress/skipped. |
| Reconnect state | `pkg/whatsmeow/service/whatsmeow.go:1019` | `reconnectState`; add stable-since and conflict count. |
| Reconnect reset | `pkg/whatsmeow/service/whatsmeow.go:2187` | `reconnectSucceeded` on any `Connected`; delay 60 s. |
| 440 handling | `pkg/whatsmeow/service/whatsmeow.go:2421` | `StreamReplaced` returns without state/reason. |
| Receipts | `pkg/whatsmeow/service/whatsmeow.go:3243` | Handles Read/ReadSelf/Delivered; add Played. |
| Status rank | `pkg/message/repository/message_repository.go:237` | Rank Delivered/Read only; add Played/Error if adopted. |
| Status view | `pkg/instance/service/instance_service.go:543` | Returns Connected/LoggedIn only; add health. |
| Runtime stats | `pkg/whatsmeow/service/whatsmeow.go:4620` | `Reconnecting` never populated; add facts. |
| WebSocket auth | `cmd/wamux/main.go:429` | Ticket is admin-only; add instance-scoped tickets. |
| WS docs | `docs/wiki/recursos-avancados/events-system.md:493` | Shows instance token; implementation uses global key. |
| Release | `.github/workflows/publish-release.yml` | Pushes `latest` on every main merge. |
| S3 policy | `pkg/storage/minio/media_storage.go:41` | Public-read policy by default. |
| Webhook client | `pkg/events/webhook/webhook_producer.go:120` | Plain transport; no SSRF guard. |
| Typebot client | `pkg/typebot/service/typebot_service.go:54` | Plain `http.Client`; no SSRF guard. |
| Payload token | `pkg/whatsmeow/service/whatsmeow.go:3713` | Adds `instanceToken` to every event. |
| FAQ | `docs/wiki/referencia/faq.md:14` | Says Evolution API uses whatsmeow; it uses Baileys. |
| Retry docs | `docs/wiki/recursos-avancados/events-system.md:90` | Says fixed 30 s; code is exponential 2–60 s. |

---

## 10. What not to copy

- **Telemetry default on** — contradicts WaMux's privacy stance.
- **Root container / no healthcheck** — WaMux is already better.
- **MySQL "support" without parity** — the fork's own migrations are PG-first.
- **Unauthenticated Meta/Evolution webhooks** — do not repeat.
- **Metrics IP allowlist bug** — do not assume their security code is correct.
- **Prebuilt manager submodule** — WaMux's in-repo manager is maintainable.
- **Voice calls / Wavoip** — unreliable unofficial stack.
- **Baileys patches** — Node-specific; not applicable to whatsmeow.
- **Redis-coupled durable queue** — WaMux can choose Postgres/NATS; don't add
  Redis solely for this.
- **Their 1 s→5 min reconnect schedule** — more aggressive than WaMux's; a ban
  risk for unscanned/logged-out sessions. Keep WaMux's conservative schedule.
- **Full feature parity race** — porting Cloud API + 8 chatbots + 6 sinks is a
  multi-quarter effort and dilutes WaMux's strengths.

---

## 11. Bottom line

If the goal is **maximum integration breadth and drop-in Evolution API v2
compatibility**, Reforged is ahead and WaMux should not try to match it feature
for feature. The rational moves are a generic HTTP chatbot adapter and, on
demand, Kafka.

If the goal is **the best self-hosted WhatsApp gateway** — fast, secure, correct,
observable, maintainable — WaMux is already ahead on the fundamentals. The
highest-value work is not copying their integrations; it is adopting their
**reliability lessons**: history batch resilience, stable-window reconnect,
440/non-retryable handling, graceful event drain, per-instance health, and a
safer release pipeline.

The one strategic bet worth a time-boxed spike is **durable inbound event
handling**. Their Redis queue proves the problem is real, but WaMux should solve
it with its own constraints (Postgres or NATS, no mandatory Redis) and only after
measuring the actual loss window in production.

---

## 12. Sources

- Reforged: <https://github.com/Operational-Intelligence-Inc/evolution-api-reforged>
  - `CHANGELOG.md` (3.0.0–3.3.0, Unreleased)
  - `src/api/integrations/channel/whatsapp/reconnect.policy.ts`
  - `src/api/integrations/channel/whatsapp/connection-health.ts`
  - `src/api/integrations/channel/whatsapp/connection-watchdog.ts`
  - `src/api/integrations/channel/whatsapp/baileys-event-queue.ts`
  - `src/api/integrations/channel/whatsapp/pending-message-updates.ts`
  - `src/api/integrations/channel/whatsapp/browser.identity.ts`
  - `.github/workflows/publish_prod_image.yml`, `promote_release.yml`
  - `src/config/env.config.ts`, `src/api/routes/index.router.ts`
- WaMux:
  - `README.md`, `FORK_NOTES.md`, `CHANGELOG.md`
  - `pkg/routes/routes.go`
  - `pkg/whatsmeow/service/whatsmeow.go`
  - `pkg/whatsmeow/service/message_lifecycle.go`
  - `pkg/message/repository/message_repository.go`
  - `pkg/instance/service/instance_service.go`
  - `pkg/events/`, `pkg/storage/minio/`, `pkg/ssrf/`, `pkg/ownership/`
  - `.github/workflows/`, `benchmarks/`, `docs/wiki/`
