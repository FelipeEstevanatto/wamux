# Reforged hardening — final report

**Date:** 2026-11-22
**Baseline:** `da268e8` (WaMux 1.0.1)
**Commits:** 26
**Reference analysis:** [`comparison-evolution-api-reforged.md`](./comparison-evolution-api-reforged.md)

## Outcome by workstream

| Item | Status | Commit |
|---|---|---|
| A1 stable-window reconnect reset | done | `4126046` |
| A2 440 / non-retryable policy + conflict cap | done | `4126046` |
| A3 history row fallback + counts | done | `de6c45c` |
| A4 configurable history depth + INFO batches | done | `de6c45c` |
| A5 graceful event drain | done | `f96cca8` |
| A6 weak/example API key warning | done | `4126046` |
| B1 instance-scoped WebSocket tickets | done | `d399ae3` |
| B2 SSRF for tenant-configured endpoints | done | `c9766b5` |
| B3 S3 private by default | done | `c9766b5` |
| B4 event token opt-out | done | `4231e2a` |
| B5 local audit log + endpoint | done | `0efdcd3` |
| C1 per-instance health verdict | done | `006f8d1` |
| C2 watchdog report/on | done | `faf3ec9` |
| C3 Played status | done | `8226dd9` |
| C4 idle client eviction | done | `5bc219a` |
| D1 pre-release + manual promote | done | `f952da8` |
| D2 docs corrections | done | `aec6d1f` |
| E1 manager UI | partial | `04fc22c` |
| E2 global message search | done | `9b6896b` |
| E3 auxiliary-table retention + metrics | done | `371e631` |
| E4 S3 cleanup (global bucket) | done | `fbd9389` |
| F design spikes | done | `1014165` |

Also: benchmarks `edaf26f`, health nil/prune fix `addb436`, Swagger
`125f7a0`/`08b5cf0`, env docs `705cc9d`/`41acb5d`, changelog `f9196e4`.

## What changed (high level)

### Reliability

- Reconnect backoff is no longer cleared by a transient `Connected`; it clears
  after 60 s of continuous connection.
- `StreamReplaced` (440), `LoggedOut`, temporary bans and non-retryable stream
  errors mark the instance disconnected, suppress automatic reconnect, keep the
  session and persist a clear reason. `RECONNECT_CONFLICT_LIMIT` caps consecutive
  440s and decides whether a process restart may retry.
- History inserts retry a failed chunk, then write row by row; the batch log
  reports inserted/skipped-existing/duplicates/failed and skip reasons.
- History depth is configurable (`HISTORY_SYNC_DAYS`, `HISTORY_SYNC_MAX_MB`,
  `HISTORY_STORAGE_QUOTA_MB`; defaults 3650 d / 10 GB / 10 GB).
- Shutdown disconnects sockets and waits for in-flight event handlers with a
  10 s deadline before draining persistence.
- A weak/example `GLOBAL_API_KEY` logs a warning (the generated-key path is
  unchanged).

### Security

- `/ws/ticket` accepts an instance token; the signed ticket binds the socket to
  that instance. The global key/ticket still opens any/all.
- Webhook, Typebot and per-instance S3 endpoints use the DNS-pinned SSRF guard.
  Private targets require `INTEGRATIONS_ALLOW_PRIVATE=true`.
- Global S3 is presigned by default; public read requires `S3_PUBLIC_READ=true`
  (or a per-instance public URL).
- `WEBHOOK_INCLUDE_TOKEN=false` omits the instance token from every event sink.
- `audit_logs` records instance create/delete/rename, proxy/S3/HMAC changes,
  advanced settings and token reads. `GET /server/audit` is admin-only.

### Observability / lifecycle

- `/instance/status`, `/instance/inventory` and `/metrics` expose a per-instance
  verdict (`ok`, `reconnecting`, `looping`, `stalled`, `closed`), reasons, last
  event/inbound/ack, reconnect counts and event-handler age.
- `WATCHDOG_MODE=report` (default) logs stalled sockets; `on` rebuilds one socket
  through the normal reconnect path, never logs out, and stops after
  `WATCHDOG_MAX_REBUILDS`.
- `INSTANCE_IDLE_EVICT_MINUTES` removes a disconnected runtime client after the
  window while keeping the DB row, JID and session.

### Product / operations

- `Played` is stored above `Read`; played receipts are handled and shown in the
  manager.
- `GET /chat/history/search` searches a whole instance when `chat` is omitted,
  with keyset pagination and a new `(instance_id, timestamp)` index.
- Cleanup prunes `send_operations` (30 d default), `chat_reads`, `chat_states`,
  `typebot_sessions` and `poll_votes`; `evo_cleanup_deleted_total{table}` reports
  row counts. Expired global S3 objects are deleted only when no message still
  references the URL.
- `main` publishes immutable version/sha tags and a GitHub pre-release; a manual
  promote workflow moves `latest` and the minor tag by digest.
- Manager instance settings gained Storage (S3) and Security (HMAC + health)
  sections.
- Docs: corrected the Evolution/Baileys comparison, ticket examples, webhook
  retry description, passkey CORS and defaults; added audit and spike docs.

## Verification

Run in the pinned `golang:1.27.4` / `oven/bun:1.4.4` containers:

| Gate | Result |
|---|---|
| `go build ./...` | pass |
| `go vet ./...` | pass |
| `gofmt -l cmd pkg benchmarks` | empty |
| `go test -race -shuffle=on ./...` | pass (exit 0) |
| `golangci-lint run --timeout 8m ./...` | **0 issues** |
| `scripts/check-docs.sh` | pass |
| Manager `bun install --frozen-lockfile`, lint, build, test | pass, **217 tests** |
| Real Postgres repository suite (`EVO_BENCH_POSTGRES_DSN`) | pass |
| Image build + `scripts/image-smoke.sh` | pass; **17 migrations applied**, healthcheck, manager, generated-key auth |
| YAML parse of release workflows | pass |

Service-gated tests (NATS, RabbitMQ, MinIO integration) still skip without those
services; they were not silently disabled. The Postgres-gated repository suite
was run against a real Postgres 16 container.

## Benchmarks

Baseline worktree at `da268e8`; Postgres 16, `-benchtime=200x -count=3`,
amd64 Ryzen 7 5700X. Numbers are noisy, so read the range.

| Benchmark | Before | After |
|---|---|---|
| `InsertMessageSingle` | 2.87–3.60 ms/op | 2.44–2.66 ms/op |
| `InsertMessagesBatch` (100 rows) | 7.48–8.80 ms/op | 7.27–8.15 ms/op |
| `BenchmarkHealthSnapshot` | — | 3.8–5.6 µs/op, 5.5 KB, 8 allocs |
| `BenchmarkReconnectAllowed` | — | 175–187 ns/op, 112 B, 1 alloc |
| `BenchmarkReconnectStableDrop` | — | 283–325 ns/op, 224 B, 2 allocs |

The history insert path did not regress; the single-insert improvement is likely
run variance/cache state, not a code change.

## Remaining risks / deferred

1. **Durable inbound events are still not implemented.** Graceful drain covers
   SIGTERM, not kill -9/OOM. The Postgres-inbox spike is in
   [`spikes/durable-events.md`](./spikes/durable-events.md); it needs approval.
2. **Per-instance S3 cleanup is not implemented.** Only the global bucket is
   pruned, and only when no message references the URL. Per-instance buckets need
   an object-ownership tracker.
3. **Manager coverage is partial.** Typebot, labels, communities, newsletters,
   poll results and fleet inventory/limits are deferred with reasons in
   [`spikes/README.md`](./spikes/README.md).
4. **SSRF default may break internal integrations.** Webhooks/Typebot/per-instance
   S3 pointing at a private host now require `INTEGRATIONS_ALLOW_PRIVATE=true`.
   This is intentional and documented, but is a deployment-visible change.
5. **`WEBHOOK_INCLUDE_TOKEN` still defaults to true** for compatibility. Consumers
   receive the instance credential unless they opt out.
6. **Audit retention is manual.** The table grows append-only; SQL retention is
   documented in [`../wiki/referencia/audit-log.md`](../wiki/referencia/audit-log.md).
7. **Watchdog integration test** covers the policy with a fake clock; the service
   loop itself is thin but not exercised with a fake socket.
8. **Release workflows were YAML-validated, not executed** from this environment.
9. **No new runtime dependency** was added; no telemetry/license call was added.

## Decisions requested

- Approve or reject the **Postgres event-inbox spike** (recommended next big
  reliability project).
- Approve or reject a **generic HTTP chatbot adapter** (recommended instead of
  porting six Reforged providers).
- Keep **Kafka** deferred until a deployment requires it.
- Keep **Cloud API and MySQL** out of scope without a funded requirement.
