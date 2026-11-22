# WaMux vs Evolution API Reforged — full comparison (Revision 2)

**Date:** 2026-11-22
**WaMux:** `1.0.1` + 28 post-release commits, HEAD `9fb919b`
**Reforged:** `3.3.0`, HEAD `8c223ce` (unchanged since Revision 1)
**Compared repo:** <https://github.com/Operational-Intelligence-Inc/evolution-api-reforged>
**Supersedes:** Revision 1 of this document (written before the hardening work)

---

## 0. TL;DR

- These remain **two different implementations of the same product category**, not
  two branches: WaMux is Go/whatsmeow, Reforged is Node/Baileys.
- **Reforged still wins on integration breadth and ecosystem compatibility**:
  Cloud API, eight chatbot integrations, Kafka/SQS/Pusher, Redis, MySQL, and
  drop-in Evolution API v2 compatibility.
- **Reforged’s clearest remaining reliability advantage is its durable inbound
  event queue** (Redis Streams, replay, per-item deadline, queue health). WaMux
  now drains in-flight handlers on shutdown but still has no durable replay.
- **WaMux closed most of the socket/reconnect/history gap** in this cycle:
  stable-window reconnect, 440 and non-retryable handling, history row fallback,
  configurable 10 GB history, health verdict, watchdog, Played status, idle
  eviction, and pre-release/promote releases are now implemented.
- **WaMux is now ahead on security depth** (tenant-endpoint SSRF, token opt-out,
  audit log, instance-scoped WS tickets, private S3 default, passkeys) and on
  engineering evidence (881 tests, 75 benchmarks, fuzzers, race/CodeQL/Trivy).
- **Do not chase feature parity.** The remaining work worth doing is the durable
  Postgres/NATS event inbox, a generic HTTP chatbot adapter, and selected manager
  screens. Cloud API/MySQL remain strategic decisions, not parity tasks.

---

## 1. What changed in WaMux since Revision 1

| Area | Before (Revision 1) | Now |
|---|---|---|
| Reconnect reset | Cleared on any `Connected`; 440 loops defeated backoff | Cleared only after 60 s of continuous connection |
| 440 / logout / ban | Reconnected or looped | Blocked with a clear reason; session kept; conflict cap |
| History batch failure | One bad row dropped the batch | Chunk retry, then row-by-row; counts logged |
| History depth | Fixed 3650 d / 2 GB | Configurable, default 3650 d / 10 GB |
| Shutdown | Persistence pool drained only | Sockets stop, in-flight handlers drain, then pools |
| WS auth | Global key/ticket only; docs claimed instance token | Instance-scoped signed tickets |
| Webhooks/Typebot/S3 SSRF | Unguarded | DNS-pinned guard; private targets opt-in |
| S3 default | Public-read bucket policy | Presigned by default; public read opt-in |
| Event payload token | Always included | `WEBHOOK_INCLUDE_TOKEN=false` opt-out |
| Admin audit | None | Local `audit_logs` + `GET /server/audit` |
| Per-instance health | `connected`/`reconnecting` only | Verdict, reasons, last events/acks, reconnect counts |
| Watchdog | None | `off`/`report`/`on`, capped, no logout |
| Statuses | Sent/Delivered/Read | Adds `Played` above `Read` |
| Idle instances | Runtime client kept forever | Optional idle eviction, session kept |
| Search | Per conversation only | Global per-instance search with keyset pages |
| Retention | Messages/media/receipts | Also send operations, chat reads/states, Typebot, poll votes |
| S3 cleanup | None | Expired global-bucket objects deleted only when unreferenced |
| Manager | No S3/HMAC UI | Storage + Security sections, health card |
| Release | `latest` moved on every main merge | Pre-release + manual promote by digest |
| Token backfill | Re-encrypted every instance every boot | Skips already-encrypted rows |
| Audit latency | Synchronous DB insert per read | Async queue + 1-minute token-read dedupe |
| Logger footprint | ~450 KB per instance | ~100 KB per instance; closed on delete |

---

## 2. Scorecard

| Category | Winner | Why |
|---|---|---|
| Runtime efficiency | **WaMux** | One Go binary; no Node/Baileys runtime; live stack uses ~15 MB heap / ~66 MB RSS with one connected instance. |
| Protocol reliability | **Reforged** (narrow) | Still has durable event replay and per-item queue deadlines. WaMux now matches socket health/watchdog/reconnect policy but not restart durability. |
| Data correctness | **WaMux** | Keyset history, global search, LID canonicalization, chat state/unread, albums, idempotent sends, edit/revoke rules, media integrity. |
| Security | **WaMux** | Tenant-endpoint SSRF, token encryption, HMAC, instance WS tickets, passkeys, private S3 default, audit, upload/send guards. |
| API breadth | **Reforged** | Cloud API, eight chatbots, Kafka/SQS/Pusher, templates. |
| API depth | **WaMux** | Communities, newsletters, poll results, group join requests, albums, idempotency, richer history. |
| Observability | **WaMux** (overall) | Health endpoint, rich metrics, per-instance verdict, inventory, audit. Reforged still has queue lag; WaMux has no durable queue to report. |
| Multi-tenancy | **WaMux** | Encrypted tokens, per-instance S3/HMAC, send guards, signed WS tickets. |
| Multi-node | **WaMux** | Postgres advisory locks + leases + heartbeat. Reforged is single-process. |
| Manager | **WaMux** (maintainability) | In-repo React 19 with messages/events/API tester, now S3/HMAC/health. Reforged has upstream manager breadth but as a submodule. |
| Docs | **WaMux** | Wiki, API reference, audit, spikes, release process. Reforged relies on upstream docs. |
| Tests / CI | **WaMux** | 190 test files, **881 tests**, 8 fuzzers, 75 benchmarks, race/shuffle, CodeQL, govulncheck, gitleaks, Trivy, image smoke. Reforged has 8 focused suites (~121 tests). |
| Integrations / ecosystem | **Reforged** | Drop-in Evolution API v2 and the surrounding ecosystem. |
| Release discipline | **Tie** | Both now publish pre-releases and promote `latest` by hand. |
| Deployment hardening | **WaMux** | Non-root, healthcheck, migrations at boot, multiple compose layouts. Reforged runs as root, no healthcheck. |
| Privacy | **WaMux** | No telemetry, license server, or heartbeat. Reforged keeps upstream telemetry on by default. |
| Measured performance | **WaMux** | Live benchmark evidence below; Reforged was not benchmarked here. |

---

## 3. What Reforged does best

### 3.1 Drop-in Evolution API v2 compatibility

It keeps the Node Evolution API routes, payloads, webhook event names and
manager API. Existing clients, CRM connectors, n8n nodes and managers can switch
with little change. WaMux’s API is its own (Evolution Go lineage), so it cannot
be a drop-in replacement.

### 3.2 Integration breadth

- **Channels:** Baileys, official WhatsApp Cloud API, stateless webhook channel.
- **Chatbots:** Chatwoot, Typebot, OpenAI (+ Whisper), Dify, Flowise, N8N,
  EvoAI, EvolutionBot.
- **Event sinks:** webhook, WebSocket, RabbitMQ, NATS, Kafka, SQS, Pusher.
- **Infrastructure:** Redis cache, PostgreSQL/MySQL/PgBouncer.

WaMux now matches the core Baileys path and webhook/RabbitMQ/NATS/WebSocket, but
not the rest.

### 3.3 Durable inbound event queue

This is still their strongest reliability idea and the one WaMux has not
implemented:

- Redis Streams per instance; XADD before processing, XDEL after success.
- Replay of the backlog on the next socket/restart.
- Per-item deadline; a hung handler is logged and the queue moves on.
- Max entry size and stream length; oversize history batches split into parts.
- Queue health in `GET /instance/connectionState` (`pending`, `oldestAgeMs`,
  `runningAgeMs`, `processed`, `persistent`).

WaMux now has graceful drain and per-instance health, but a `kill -9`/OOM between
WhatsApp’s ack and the handler completing can still lose an event.

### 3.4 Baileys-specific edge handling

Some of this is now matched (stable reconnect, 440 cap, non-retryable codes,
history row fallback, early status, Played, idle eviction), but their experience
with Baileys remains deeper: connection generation fencing, serialized socket
creation, early ack buffering, and the event queue’s replay semantics.

### 3.5 Operational documentation

Their CHANGELOG entries read like incident reports: symptom, root cause,
mechanism, limits, and live-test evidence. That remains a good model for “why
this exists” documentation.

### 3.6 Caveats in Reforged

- Telemetry enabled by default; self-hosted operators must turn it off.
- Runs as root, no Docker healthcheck, migrations via shell entrypoint.
- MySQL is not at parity with the fork-specific PostgreSQL changes.
- Meta/Evolution channel webhooks are mounted without API-key guards.
- Publish workflow does not run tests; a direct `prod` merge bypasses the test
  job.
- Metrics IP allowlist is a no-op (`filter(...) === 0` compares an array to 0).
- No audit log; event payloads always carry the instance `apikey`.
- No per-instance S3 and no passkey pairing.
- No built-in release promotion before their unreleased change (now added).

---

## 4. What WaMux does best

### 4.1 Runtime, memory, and measured performance

- Single Go binary on whatsmeow; no Node/Baileys process.
- Bounded persistence/media/background pools; cgroup-aware `GOMEMLIMIT`.
- Live local stack: **~15 MB heap, ~66 MB RSS, 83 goroutines** with one
  connected instance; Docker memory ~23 MiB.
- Under a 101-instance load after fixes: **~38 MB heap / ~104 MB RSS** vs the
  pre-hardening baseline **~76 MB / ~137 MB**.
- `GET /instance/all`: **1,217–1,277 req/s** after async/deduped audit
  (baseline 1,075–1,164); 1,000 requests create **1** audit row.

### 4.2 Reliability now close to parity

- Stable-window reconnect reset (60 s), 440 conflict cap, non-retryable codes,
  session kept for manual resume.
- History batch chunk retry + row fallback; batch counts logged.
- Configurable 10 GB history request.
- Graceful event drain on shutdown.
- Per-instance health verdict and watchdog.
- Played status, idle eviction, app-state recovery, NCT salt bootstrap.

The remaining durability gap is deliberate and isolated: no inbound event
journal.

### 4.3 Security depth

- **SSRF:** caller URLs always guarded; webhook/Typebot/per-instance S3 guarded
  with `INTEGRATIONS_ALLOW_PRIVATE` opt-in. DNS pinning and redirect
  revalidation.
- **Tokens:** HMAC lookup hash + AES-GCM ciphertext, no plaintext at rest.
- **Webhooks:** per-instance/global HMAC, encrypted keys, DLQ, replay CLI.
- **S3:** presigned by default; public read opt-in; per-instance S3 with
  encrypted secret.
- **WebSocket:** 30 s HMAC ticket, instance-scoped.
- **Pairing:** passkey/WebAuthn helper.
- **Abuse:** per-credential/IP rate limits, per-instance send guard, upload
  guard, 5-minute signed downloads.
- **Audit:** local admin action log; no secrets.
- **Privacy:** no telemetry, license activation, or heartbeat.

### 4.4 Data correctness and API depth

- Keyset history pagination, filters (`after`, `until`, `type`, `from_me`),
  global per-instance search, capabilities contract.
- LID ↔ phone canonicalization, chat merge, unread counts, archived/pinned/
  muted/marked-unread state, read position.
- Albums, carousel, product messages, exact media type preservation.
- Idempotent sends with native WhatsApp ID reservation and operation status.
- Edit/revoke author, chat and 15-minute rules; tombstones; status monotonicity.
- Communities, newsletters, poll results, group join requests, labels, presence
  subscribe, mark played.
- Message counter rollup, chat-list CTE, retention, media SHA-256 verification.

### 4.5 Observability and operations

- `/server/health`, `/server/ok`, `/server/stats`, `/server/audit`, `/metrics`.
- Per-instance verdict (`ok`, `reconnecting`, `looping`, `stalled`, `closed`)
  with reasons, last event/inbound/ack, reconnect counts, event age.
- Watchdog `report` default; `on` rebuilds without logout.
- Inventory is a monitoring-safe fleet view.
- Multi-node ownership/lease primitives.
- Cleanup now covers messages, local media, send operations, chat reads/states,
  Typebot sessions, poll votes, orphan receipts, and unreferenced global S3
  objects.

### 4.6 Manager/docs/tests

- In-repo React 19 manager: dashboard, instances, messages, events, and the API tester.
- Storage, HMAC, and health settings.
- 36 wiki pages plus the API reference, audit, spike and archive docs.
- **881 tests**, 8 fuzzers, 75 benchmarks; race/shuffle, CodeQL, govulncheck,
  gitleaks, Trivy, image smoke.

---

## 5. Feature matrix

Legend: ✅ full, ⚠️ partial/different, ❌ absent, ➖ not applicable.

| Capability | WaMux | Reforged | Notes |
|---|---|---|---|
| WhatsApp Web transport | ✅ whatsmeow | ✅ Baileys | Different libraries. |
| Official Cloud API | ❌ | ✅ | Reforged Meta webhook + Graph + templates. |
| Stateless webhook channel | ❌ | ✅ | Reforged “Evolution” channel. |
| PostgreSQL | ✅ | ✅ | Both production-grade. |
| MySQL / PgBouncer | ❌ | ⚠️ | MySQL exists upstream; fork migrations are PG-first. |
| SQLite | ⚠️ auth DB | ❌ | WaMux auth only. |
| Redis cache | ❌ | ✅ | Reforged optional. |
| **Durable inbound queue** | ❌ | ✅ | Reforged Redis Streams + replay; WaMux only in-flight drain. |
| Per-instance queue health | ❌ | ✅ | No queue to report in WaMux yet. |
| Webhook sink | ✅ | ✅ | WaMux per-endpoint isolation, HMAC, DLQ. |
| WebSocket | ✅ | ✅ | WaMux instance-scoped tickets; Reforged principal-bound. |
| RabbitMQ | ✅ | ✅ | Both. |
| NATS | ✅ | ✅ | WaMux JetStream + dedupe. |
| Kafka / SQS / Pusher | ❌ | ✅ | Reforged per-instance/global. |
| S3/MinIO | ✅ | ✅ | Both. |
| Per-instance S3 | ✅ | ❌ | WaMux advantage. |
| Typebot | ✅ | ✅ | WaMux deliberately minimal. |
| Chatwoot / OpenAI / Dify / Flowise / N8N / EvoAI / EvolutionBot | ❌ | ✅ | Reforged. |
| Text/media/audio/PTV | ✅ | ✅ | Equivalent. |
| Albums | ✅ | ❌ | WaMux advantage. |
| Buttons/lists | ✅ | ✅ | Both. |
| Carousel | ✅ | ❌ | WaMux advantage. |
| Polls + results | ✅ | ⚠️ | Reforged sends; WaMux also returns results. |
| Product messages | ✅ | ✅ | Reforged also fetches catalog/collections. |
| Templates | ❌ | ✅ | Cloud API only. |
| Status/stories | ✅ | ✅ | Both. |
| Groups | ✅ | ✅ | WaMux join requests; Reforged revoke/accept invite. |
| Communities | ✅ | ❌ | WaMux advantage. |
| Newsletters | ✅ | ❌ | WaMux advantage. |
| Labels | ✅ | ⚠️ | WaMux more granular. |
| Calls | ⚠️ reject | ⚠️ offer/Wavoip | Neither is a full VoIP stack. |
| History readback | ✅ keyset/filters | ✅ findMessages | WaMux more precise. |
| Global message search | ✅ | ⚠️ | WaMux keyed by instance; Reforged flexible JSON search. |
| History sync depth | ✅ 3650 d / 10 GB | ✅ 3650 d / 10 GB | Both configurable now. |
| Chat state/unread | ✅ | ✅ | Both. |
| LID mapping | ✅ | ✅ | Both. |
| Idempotent sends | ✅ | ❌ | WaMux advantage. |
| Played status | ✅ | ✅ | Both now. |
| Edit/revoke correctness | ✅ strict | ✅ author check | Both hardened. |
| HMAC webhooks | ✅ | ⚠️ JWT | Different mechanisms. |
| Event token opt-out | ✅ | ❌ | WaMux advantage. |
| SSRF (caller URLs) | ✅ | ✅ | Both. |
| SSRF (tenant-configured endpoints) | ✅ | ❌ | WaMux guards webhook/Typebot/S3. |
| Token encryption at rest | ✅ | ❌ | WaMux advantage. |
| Passkey pairing | ✅ | ❌ | WaMux advantage. |
| Audit log | ✅ | ❌ | WaMux advantage. |
| Instance-scoped WebSocket | ✅ | ✅ | Both. |
| `/server/health` | ✅ | ❌ | WaMux advantage. |
| Metrics | ✅ rich | ⚠️ basic | WaMux richer; Reforged IP allowlist bug. |
| Per-instance health verdict | ✅ | ✅ | Reforged adds queue/unacked; WaMux adds audit/inventory. |
| Watchdog | ✅ report/on | ✅ report/on | Both. |
| Reconnect backoff | ✅ stable window + cap | ✅ stable window + cap | Both. |
| History row fallback | ✅ | ✅ | Both. |
| Idle client eviction | ✅ | ✅ | Both. |
| Multi-node single-writer | ✅ | ❌ | WaMux advantage. |
| Release promotion | ✅ | ✅ | Both. |
| Non-root + healthcheck | ✅ | ❌ | WaMux advantage. |
| Manager | ✅ in-repo | ✅ upstream prebuilt | WaMux maintainable; Reforged broader integration UI. |
| Tests/CI | ✅ 881 tests | ⚠️ ~121 tests | WaMux much larger. |
| Telemetry | none | on by default | WaMux advantage. |

---

## 6. Adoption status from Revision 1

### 6.1 Closed in this cycle

| Reforged improvement | WaMux status |
|---|---|
| Stable-window reconnect reset | adopted |
| 440 conflict cap + non-retryable codes + keep session | adopted |
| History batch chunk/row fallback | adopted |
| Configurable 10 GB history | adopted |
| Graceful event drain | adopted (lighter than durable queue) |
| Weak/example API key warning | adopted |
| Per-instance connection health | adopted |
| Watchdog report/on | adopted |
| Played status | adopted |
| Idle client eviction | adopted |
| Instance-scoped WebSocket tickets | adopted |
| SSRF for tenant endpoints | adopted, broader than Reforged |
| S3 private default | adopted |
| Event token opt-out | adopted; Reforged does not have it |
| Audit log | adopted; Reforged does not have it |
| Pre-release + manual promote | adopted |
| Global search, retention, S3 cleanup | adopted; Reforged lacks these |
| Release/docs corrections | adopted |

### 6.2 Still worth taking from Reforged

1. **Durable inbound event handling.** A Postgres inbox is the best fit for
   WaMux; NATS JetStream optional; Redis Streams only if a deployment already
   standardizes on Redis. The design and success criteria are in
   [`spikes/durable-events.md`](./spikes/durable-events.md).
2. **Queue health semantics.** If the inbox is built, expose `pending`,
   `oldestAgeMs`, `runningAgeMs`, `processed` and `persistent` per instance, as
   Reforged does.
3. **Unacked-send diagnostics.** Reforged reports unacked person sends; WaMux
   tracks last ack but not a count/oldest age. This is a small health addition.
4. **Slow-handler logging.** Reforged logs the event types and handler when a
   queue item exceeds its deadline. WaMux reports event age but does not name a
   stuck handler.
5. **A generic HTTP chatbot adapter.** One adapter with triggers, debounce,
   ignoreJids, stop keywords and sessions covers N8N/Dify/Flowise/custom.
   Porting six providers is the wrong move; see
   [`spikes/generic-chatbot.md`](./spikes/generic-chatbot.md).
6. **Kafka**, only if a deployment asks. Keep it in a separate module/build tag.
7. **Cloud API and MySQL** remain strategic decisions, not parity tasks; see
   [`spikes/cloud-api-mysql.md`](./spikes/cloud-api-mysql.md).
8. **Manager breadth**: Typebot, labels, communities, newsletters, poll results
   and fleet inventory/limits. The backend exists; the screens are deferred with
   reasons in [`spikes/README.md`](./spikes/README.md).

### 6.3 Do not copy

- Telemetry-on-by-default, root image, no healthcheck.
- MySQL “support” without dialect parity.
- Redis-coupled inbound queue as a hard dependency.
- Always including the instance token in event payloads.
- The upstream manager as a submodule.
- Voice/Wavoip.
- Their 1 s→5 min reconnect schedule (ours stays conservative).
- Feature-parity race with Cloud API + eight chatbots + six sinks.

---

## 7. New gaps exposed by the hardening cycle

The implementation and live deployment benchmark surfaced four issues that were
fixed; they are recorded here because they are now part of the comparison
baseline:

1. **Audit writes were synchronous** on `/instance/all`; now async + deduped.
2. **Per-instance loggers reserved ~450 KB each**; now ~100 KB with a lazy
   buffer, closed on delete, tombstoned against late recreation.
3. **Token backfill re-encrypted every instance every boot**; now skips rows
   with `TokenEnc`.
4. **Inventory emitted empty health for every instance**, doubling the response;
   now omitted for `no_health_data`.

Remaining known gaps in WaMux:

- No durable inbound event queue/replay.
- No per-instance S3 object cleanup (global bucket only).
- Manager screens for Typebot/labels/communities/newsletters/poll/inventory.
- Audit retention is manual SQL.
- Watchdog service loop is policy-tested but not exercised with a fake socket.
- Release workflows were YAML-validated, not run from this environment.
- SSRF default is a deployment-visible change for internal webhooks/Typebot/S3.
- No tracing/request IDs; no Sentry/OTel by design.
- No cluster-wide rate limiting/WebSocket fanout; ownership is single-writer, not
  full request routing/failover.

---

## 8. Live deployment evidence

Local Compose stack, baseline image built from `da268e8` vs current, separate
Postgres, 101 instances, rate limit disabled, `CONNECT_ON_STARTUP=false`.

| Endpoint (1,000 req, c=20) | Baseline | Current |
|---|---:|---:|
| `/instance/all` | 1,075–1,164 req/s | **1,217–1,277 req/s** |
| `/instance/inventory` | 1,185 req/s | **1,166 req/s** |
| `/instance/status` | 14,171 req/s | **13,493 req/s** |
| `/server/health` | 24,482 req/s | **24,919 req/s** |
| `/metrics` | 3,208 req/s | **3,028 req/s** |
| `/server/ok` | 29,849 req/s | **26,099 req/s** |

Memory under 101 instances after GC: **~38 MB heap / ~104 MB RSS** vs baseline
**~76 MB / ~137 MB**. After the token-backfill fix, the local stack with one real
instance is **~15 MB heap / ~66 MB RSS / 83 goroutines**, Docker memory ~23 MiB.

These are single-host numbers with variance; they are not a direct comparison to
Reforged, which was not deployed or benchmarked here.

---

## 9. Bottom line (Revision 2)

- **Reforged remains the choice for ecosystem compatibility and integration
  breadth**, and its durable event queue is still the one reliability feature
  WaMux should not pretend to have.
- **WaMux is now the stronger self-hosted WhatsApp gateway on engineering
  fundamentals**: security, data correctness, observability, manager, docs,
  tests, multi-node primitives, and measured memory/performance.
- The reliability gap has narrowed to **restart durability**; socket lifecycle,
  reconnect policy, history resilience, health and watchdog are now matched.
- The next highest-value work is not another integration: it is the **Postgres
  inbound-event inbox spike**, followed by queue health/unacked diagnostics.
- After that, a **generic HTTP chatbot adapter** is the best integration bet;
  Kafka only on demand; Cloud API and MySQL only with a funded requirement.

---

## 10. Sources

- Reforged (unchanged at `8c223ce`, 3.3.0):
  - `CHANGELOG.md` 3.0.0–3.3.0 and Unreleased
  - `src/api/integrations/channel/whatsapp/reconnect.policy.ts`
  - `connection-health.ts`, `connection-watchdog.ts`, `baileys-event-queue.ts`
  - `pending-message-updates.ts`, `browser.identity.ts`
  - `.github/workflows/publish_prod_image.yml`, `promote_release.yml`
  - `src/config/env.config.ts`, `src/api/routes/index.router.ts`
- WaMux (`9fb919b`):
  - `CHANGELOG.md`, `FORK_NOTES.md`, `README.md`
  - `docs/archive/reforged-hardening-report.md`
  - `docs/archive/spikes/`
  - `pkg/whatsmeow/service/reconnect_policy.go`, `health.go`, `watchdog.go`,
    `idle.go`, `event_tracker.go`
  - `pkg/audit/`, `pkg/logger/`, `pkg/ssrf/`, `pkg/storage/minio/`
  - `pkg/instance/service/instance_service.go`
  - `pkg/message/repository/`, `pkg/message/cleanup/`
  - `wamux-manager/src/pages/InstanceSettings.tsx`
  - `.github/workflows/`
