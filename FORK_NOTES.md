# Evolution Go — `fork/community-stable`

This is a self-hosted fork of
[`evolution-foundation/evolution-go`](https://github.com/evolution-foundation/evolution-go)
based on `0.7.2` (upstream commit `9337afc`).

It exists for one reason: **make a self-hosted Evolution Go stable, fully
offline, and free of the vendor license/telemetry dependency** — while folding
in the community fixes that upstream had left sitting in open PRs.

Two things were changed on top of `0.7.2`:

1. **Vendor dependency removed** — no license activation, no heartbeat, no
   telemetry, nothing that talks to Evolution Foundation.
2. **Reviewed community PRs folded in** — every open PR was read against its
   linked issue; the sound ones were applied, duplicates were collapsed, and
   the risky/large ones were deliberately left out (documented below).

---

## 1. License & telemetry removal

| Removed | Where |
|---|---|
| `pkg/telemetry/` (posted every route hit to `log.evolution-api.com`) | deleted — it was dead code, never wired into the router, but it is gone |
| `pkg/core/` — obfuscated license client: registration, activation, HMAC-signed heartbeat, deactivation, message telemetry bundle | deleted |
| `core.GateMiddleware` — returned `503 LICENSE_REQUIRED` on every API call until activation | removed |
| `core.LicenseRoutes` — `/license/register`, `/license/activate` | replaced (see below) |
| `core.InitializeRuntime` / `StartHeartbeat` / `Shutdown` — startup activation + 30-minute heartbeat | removed from `main.go` |
| `EVOLUTION_OPERATOR_EMAIL` headless auto-activation | removed from `.env.example` |

**Local-only compatibility stub.** The prebuilt Manager UI (`manager/dist`)
probes `GET /license/status` and hides instance management unless it reads
`"active"`. Three handlers now answer locally in `cmd/evolution-go/main.go`:
`/license/status`, `/license/register`, `/license/activate`. They always return
`{"status":"active"}` and **never contact any server**. This keeps the UI usable
without re-enabling activation.

The API is fully operational from first boot — no 503 gate, no registration.

---

## 2. Applied community fixes

All of these are main-branch PRs based on `0.7.2`, except where noted as ported
from a `develop` PR. Duplicate PRs are collapsed to the implementation that was
kept.

### Stability / flakiness (the main goal)

| Fix | PR(s) | Issue(s) |
|---|---|---|
| One shared whatsmeow `sqlstore.Container`, reusing the existing pooled `authDB` — stops leaking a whole Postgres connection pool on every reconnect | **#206** (chosen over #200, #194, #174, #102, #142 and develop #131, #117, #178) | #175, #165, #118, #112, #109, #106 |
| Mutex-guarded shared maps (`clientPointer`, `myClientPointer`, `killChannel`) via new `pkg/safemap` — stops `fatal error: concurrent map writes` killing the process | **#196** (over develop #127) | #203, #75 |
| Bump whatsmeow to `2026-09-04` + Go 1.26 + `SetStatusMessage` API — fixes the `stream:error` reconnect loop that never recovers | **#190** | #185 |
| Reconnect backoff (5 free attempts, then 5m/15m/30m) with one scheduled attempt per instance — stops a logged-out device hammering WhatsApp | **#197** | #185, #188, #145 |
| Only auto-restart a `Disconnected` instance once it is actually paired (`Store.ID != nil`); mid-QR blips reconnect the same session in place — QR code survives long enough to scan | **#192** | #85, #148, #186 |
| Guard `*events.Archive` type assertion — was a nil/map panic on every archive | **#195** (over #125, develop #177, #159) | #101, #95 |
| WebSocket: per-connection write mutex + multiple subscribers per instance + prune dead ones | **#181** (over #135) | #99 |
| Restore **paired** sessions on startup (`jid != ''`), not just rows flagged `connected` — paired instances come back after a restart | ported from **#154** (targeted reimplementation) | — |
| App-state sync error recovery: controlled full-sync, then recovery request to the phone | **#144** | #72 |
| Skip connecting NATS when `NATS_URL` is empty | **#143** | — |
| Re-request undecryptable messages from the phone (`REREQUEST_FROM_PHONE`) | **#156** | — |

### Correctness / API

| Fix | PR(s) | Issue(s) |
|---|---|---|
| Decrypt secret-encrypted message edits before typing/forwarding | **#153** (over #128, develop #161) | #92, #62, #146 |
| `IsEdit` / `IsRevoke` / `messageType` flags on message webhooks | ported from **#122** | #92 |
| Canonicalise JIDs for revoke/edit and avatar (the `+` prefix made recipients ignore edits and made avatar lookups time out) | **#130**, **#121** (over develop #163) | #76, #120 |
| Avatar request bounded by an 8s context + LID→PN resolution | **#121** (contains/supersedes #120) | #76 |
| `PictureURL` returned by `POST /user/info` | **#121** | — |
| `POST /user/lid` — resolve a LID to its phone number | **#179** | — |
| `POST /user/contacts` — save a contact to the address book | **#129** (over develop #162) | — |
| History-sync request sent as a peer to our own JID + deep on-link backfill | **#133** | #133 |
| Multi-device outbound messages routed to `DeviceSentMeta.DestinationJID` | **#191** | — |
| Document-with-caption + `@lid` JIDs in `mentionAll` | **#137** | #114 |
| Preserve instance config when Connect/advanced-settings is used | **#136** | #111, #81 |
| Keep event subscriptions across disconnect (webhooks were silently dropped after reconnect) | **#187** (develop) | #111 |
| Array-aware `participants` validation on `POST /group/participant` | **#180** (develop) | #97, #52 |
| View-once media on `POST /send/media` (`viewOnce: true`) | **#147** | — |
| `POST /message/subscribe` — subscribe to a contact's presence | **#152** | #146 |

---

## 3. Reviewed but intentionally **not** applied

| PR | Why not |
|---|---|
| **#145**, **#154** | Large lifecycle rewrites that introduce a *second* reconnect/runtime model. They overlap #196/#197/#192/#206 and would leave two competing mechanisms. Their unique benefit (startup restore) was ported directly instead. |
| **#141** | 5,239-line "answer/control WhatsApp calls over WebSocket" feature: new packages, dependencies and routes. A feature, not a stability fix; high blast radius. |
| **#182** | 1,631-line new sender web UI. Feature. |
| **#184** | Patches the prebuilt Manager assets for mobile touch. Frontend-only. |
| **#202**, **#201** | README Windows instructions. Docs only. |
| **#90**, **#132**, **#166**, **#151**, **#160**, **#126**, **#149**, **#150**, **#167**, **#177**, **#178**, **#180\***, **#187\***, **#198**, **#199**, **#117**, **#131**, **#127**, **#102**, **#125**, **#128**, **#142** | Target `develop`, which has an **unrelated git history** to `main` (no merge base), so they cannot be merged mechanically. Where they fix a main-branch bug, the main-targeted equivalent was applied; #180 and #187 were ported by hand. |
| **#200**, **#194**, **#174**, **#102**, **#142**, **#135**, **#127**, **#125**, **#128**, **#161**, **#162**, **#163** | Duplicates of a main fix that was applied (kept the most complete one). |

\* #180 and #187 were ported by hand despite being `develop` PRs.

---

## 4. Build & run

From the repository root (the `docker-compose.yml` is there):

```bash
cp .env.example .env     # set GLOBAL_API_KEY (required)
docker compose up -d --build
```

- API: <http://localhost:8081> (override with `EVOGO_PORT`)
- Swagger: <http://localhost:8081/swagger/index.html>
- Manager: <http://localhost:8081/manager> (log in with `GLOBAL_API_KEY`)
- Postgres is bundled and databases are auto-created.

Optional brokers/storage, not started by default:

```bash
docker compose --profile minio    up -d   # then MINIO_ENABLED=true
docker compose --profile rabbitmq up -d   # then AMQP_URL=amqp://admin:admin@rabbitmq:5672/
docker compose --profile nats     up -d   # then NATS_URL=nats://nats:4222
```

### Local development (Go)

```bash
go build ./...
go test ./...
go vet ./...
```

---

## 5. Verification performed

- `go build ./...` — clean (Go 1.26).
- `go vet ./...` — clean.
- `go test ./...` — all unit tests pass, including the tests shipped with the
  applied PRs (`pkg/safemap`, `pkg/whatsmeow/service` app-state + secret-edit,
  `pkg/instance/service` QR runtime, `pkg/user/handler`, `pkg/events/nats`).
- Full `docker compose up --build` smoke test: `/server/ok` 200,
  `/license/status` reports active locally, `/instance/all` authenticated with
  `GLOBAL_API_KEY` returns 200. No outbound request to any Evolution Foundation
  host exists in the codebase (`grep` for the domains and the license client).

## 6. Known limitations

- The prebuilt Manager frontend still contains license-screen code; the local
  stub keeps it satisfied. Rebuilding the Manager UI from source is out of scope
  here.
- `docs/docs.go` / `swagger.json` still contain generated annotations for the
  removed `/license/*` routes. They are inert documentation, not code.
- The upstream LICENSE still applies (Apache 2.0 plus its brand-protection and
  attribution conditions). Removing the runtime *activation* does not change the
  license terms of the source — see `LICENSE` and `TRADEMARKS.md`, and note that
  retaining the project's logos/copyright in the frontend is still required by
  that license.
