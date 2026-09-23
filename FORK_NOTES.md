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

## 3b. Fixes found by direct code review (not from a PR)

These were not in the PR queue — found while auditing the code against the open
issues.

| Fix | Issue |
|---|---|
| Unified the AMQP and NATS `globalEventType` switches into one mapping. They had diverged: `PICTURE`, `USER_ABOUT` and `BUTTON_CLICK` configured for `NATS_GLOBAL_EVENTS` were never published. Also removed an early `return` in the AMQP branch that skipped NATS for unmapped events. | #193 |
| `POST /user/profileName` called `SetGroupName(ctx, EmptyJID, name)` — a group rename addressed to a non-existent group — which is why it hung forever. Now sends the correct account-level profile IQ with a bounded context. | #176 |
| Quoted replies sent `ContextInfo.QuotedMessage` as an **empty** `Conversation`, producing an empty, non-tappable quote card. Now `quoted.message` can be supplied and is rendered; otherwise the empty payload is omitted. | #189 |
| Bounded every outbound HTTP call with a timeout. The worst was the WhatsApp Web version lookup (`http.Get` with no timeout) running inside `StartClient`, so a slow network could block instances from coming online; webhooks, media downloads, link previews and profile/group photo fetches were unbounded too. | reliability |
| `GET /group/myall` always returned an empty list. The owner filter compared `types.GroupInfo.OwnerJID` against a JID parsed by `utils.ParseJID`, which prefixes phone numbers with `"+"` (so it never equalled WhatsApp's owner JID); and on LID-addressed accounts the owner is reported as a LID while the account's own `Store.ID` is a phone number. The filter now normalises both sides with `ToNonAD()` and matches `OwnerJID`/`OwnerPN` against both `Store.ID` and `Store.LID`. | — |
| `POST /group/create` and `POST /group/participant` hung and failed with `"info query timed out"` when the participants were phone numbers. `utils.ParseJID` emitted `"+<number>@s.whatsapp.net"`, which WhatsApp cannot resolve, so the request IQ was dropped and the call timed out; passing a LID worked, which hid the bug. Participants are now canonicalised with `utils.CanonicalJID` (strips the `+`, leaves LID/group JIDs untouched) before being sent. | — |

### Stability refinements + features ported from `ecosb2b/evo-go-v2`

Reviewed independently against [ecosb2b/evo-go-v2](https://github.com/ecosb2b/evo-go-v2) (a fork of the same `0.7.2` base). Where both fixed the same problem, the better implementation was kept (noted below).

| Fix / feature | Notes |
|---|---|
| Animated WebP stickers were re-encoded through `convertToWebP`, which fails on animated WebP ("webpDecodeRGBA: failed") and flattens static WebP. Now WebP is uploaded untouched (`isWebP`/`isAnimatedWebP`), `IsAnimated` is set, and the download is bounded (30s, 16 MiB) with scheme/status validation. | upstream **#151**; supersedes our earlier unbounded download |
| Interactive buttons/Pix used shapes WhatsApp no longer renders (`ButtonsMessage` in a `DocumentWithCaptionMessage`, a guessed `native_flow_name`). Rewritten to the captured real payloads: top-level `interactiveMessage` for reply/CTA, `ViewOnceMessage` for Pix, `{"from":"api","templateId":<uuid>}`, and `<native_flow v="2" name="mixed"/>` / `name="payment_info"` biz nodes. | from evo-go-v2; closes our biggest known gap |
| `POST /send/event` (WhatsApp event/calendar, PR #90), `POST /send/product` (catalog card as a message, not the dead `w:biz:catalog` IQ), `PUT /instance/name/:instanceId`, `GET /server/stats`, `GET /dashboard`. | new features ported |
| Multiple webhooks per instance (`splitWebhookURLs`: JSON array or newline/comma/semicolon list). | small, backwards compatible |
| Typebot integration (bot CRUD, sessions, startChat/continueChat, flood/loop protections, `TypebotAutoPaused` alert) + `TYPEBOT_*` config. | large feature ported |
| `POST /user/savecontact` route aligned and app-state desync recovery added (force full sync on 409/LTHash, fall back to fatal recovery); BR/MX number normalisation via `ParseJID`+`CanonicalJID`. | evo-go-v2 alignment |
| Shared sqlstore container no longer caches a transient init failure (`sync.Once` → mutex + memoize success). | evo-go-v2 edge case; our shared-pool approach kept |
| Webhook delivery: a dotless queue name (e.g. `sendstatus`) silently dropped the event before any HTTP call; delivery now uses exponential backoff, skips retrying 4xx (except 408/429), caps the read body at 8 KiB, and bounds concurrent deliveries. | NathanAshford; supersedes our earlier webhook timeout-only change |
| Proxy tooling: `GET /instance/proxy/:id`, `POST .../test`, `POST .../reconnect` — checks reachability, the exit IP vs the server IP, and whether WhatsApp is reachable through it. | NathanAshford |
| Account limits: `GET /instance/limits/:instanceId` — WhatsApp reachout timelock and new-chat messaging quota (the limits behind error 463), cached on connect with a live fallback. | NathanAshford (`pkg/walimits`) |

**Kept our implementation over evo-go-v2's** (ours handles an edge case theirs does not): `/group/myall` owner filter (ours is strictly owner via `Store.ID`+`Store.LID`; theirs broadens to admin/superadmin); shared Postgres pool (reuses the existing `authDB`; theirs opens a second pool); `pkg/safemap` generic wrapper (vs their global mutex at ~103 call sites); conservative reconnect backoff (vs their `runtime_lifecycle` supervisor); WebSocket multi-subscriber (theirs replaces the previous connection); startup restore of paired instances; unbounded-HTTP hardening; and a newer whatsmeow.

## 3d. Typebot integration — caveats and known limitations

Typebot is an **optional** chatbot integration ported from `ecosb2b/evo-go-v2`. It is
**inert until configured**: with no bot created the inbound path short-circuits, no
outbound HTTP happens, and the rest of the API behaves exactly as before (see the
"without Typebot" checklist in §5). Things to know before relying on it:

**How it talks to Typebot.** We call Typebot's HTTP chat API (`startChat` /
`continueChat`) from the Go process and send the replies back through the
instance. We deliberately do **not** use Typebot's own Meta/WhatsApp channel, so
it works with a *linked-device* number (whatsmewow) instead of requiring the
official WhatsApp Business API + Meta app. Consequence: this path is only as
reliable as the configured Typebot URL — if Typebot is slow/down, replies are lost
for that contact (failures are logged, never propagated to message handling).

**Not implemented (inherited from the reference fork).**
- **No JavaScript engine**, so `clientSideActions` scripts do **not** run. The JID
  is instead pre-decomposed into `prefilledVariables` (`normalizedUserId`,
  `userPhone`, `userLid`, `jidType`). A flow containing a script block should drop
  it and use those variables; when one is still present the log says so instead of
  failing silently.
- **No `debounceTime`** — rapid successive messages are each processed, so a flow
  can advance several blocks in a row.
- **No `keepOpen`** — there is no "keep the bot open after the flow ends" mode.
- **No fallback bot** and **no keyword/regex routing** for choosing a bot.

**Behavioural caveats.**
- **Only text advances a conversation.** Media (image/audio/document) neither opens
  nor advances a session — a flow waiting on a text input will sit until the
  contact sends text.
- **`closed` ≠ silenced.** Closing a session clears it, so the next message starts
  over from the greeting. To stop replying to someone, use `paused`
  (`POST /typebot/changeStatus`).
- **Protections can pause a contact.** The per-contact rate limit is **on by
  default** (10 messages / 60s). Exceeding it pauses the session and emits
  `TypebotAutoPaused`. Legitimate bursts can therefore pause a real contact.
  The per-instance send ceiling is **off by default**.
- **One bot per instance.** The inbound path resolves a single enabled bot; there
  is no per-contact or per-keyword bot selection.
- **Sessions are ephemeral by design** — they expire by inactivity (`expire`),
  end on a keyword (`keywordFinish`), or close when the operator replies
  (`stopBotFromMe`). They are **not** durable conversation history.
- **`TYPEBOT_*` config is read at boot** — changing it needs a container recreate,
  not just an image upgrade.
- **Inbound text capture is best-effort**: only `conversation` and
  `extendedTextMessage` are forwarded (same as the reference fork); captions and
  other text-bearing wrappers are ignored.
- **Group messages are not excluded by the Typebot hook itself** — it runs after
  the existing broadcast/group checks, so `IgnoreGroups` / `EVENT_IGNORE_GROUP`
  still gate it; without those, a group message can start a session.

**Operational surface.** Adds two tables (`typebots`, `typebot_sessions`) via
`AutoMigrate`, a `/typebot` route group (instance-token auth), and one outbound
HTTP call per inbound text when a bot is enabled.

**Verified with Typebot unused (no bot configured):** the fork boots and runs
normally — `/server/ok`, `/dashboard`, `/swagger`, `/manager`, `/instance/all`,
`/server/stats`, `/group/list`, `/group/myall`, `/user/contacts` and `/send/text`
all behave as before, with zero Typebot log lines or errors. The inbound hook is a
no-op in that state (`ProcessMessage` returns right after `GetActiveBot` finds
nothing; no HTTP call is made) and the two Typebot tables are simply empty.

## 3e. VoIP / call history — not implemented here (pointer for future work)

The fork does **not** implement WhatsApp voice calls beyond `POST /call/reject`
(upstream's only call feature). If that becomes a requirement, there is a working
reference implementation to study rather than start from zero:

**[NathanAshford/evolution-go-custom](https://github.com/NathanAshford/evolution-go-custom)** —
`pkg/voip/**` (~25k lines) plus `pkg/call/{handler,service}` routes:

```
POST /call/offer        ring a number, returns a callId
POST /call/accept       answer an incoming call
POST /call/terminate    end
POST /call/hangup       end
GET  /call/audio/:callId  bidirectional call audio (16 kHz mono PCM) over a WebSocket
GET  /call/list         active calls
GET  /call/history      past calls
GET  /call/status/:callId
```

What it actually contains: a WaCalls-based signalling stack, MLow/CELP codec
(encoders/decoders, LPC/LSF/FFT/range coder), SRTP, STUN, an SCTP relay, RTP
handling, and an in-memory call manager/state machine.

**Important framing:** `/call/list`, `/call/history` and `/call/status` are **not**
standalone features — they read from that call manager, so there is nothing to
list without porting the VoIP stack. This is the same territory as upstream PR
**#141** ("answer/control WhatsApp calls over WebSocket"), which this fork
deliberately excluded (§3): a feature, not a stability fix, with a large
dependency surface and real-time media that needs live-call validation. Treat it
as a self-contained product feature, not a patch.

## 3c. Known remaining issues worth tackling next

Not fixed here — they are larger or need protocol work. Ordered by impact.

1. **Interactive buttons / lists don't render** (#170, #59, #71, #110, #51, #69).
   WhatsApp deprecated the legacy `buttonsMessage` / `listMessage` /
   `templateMessage` nodes the fork still builds; only the `InteractiveMessage`
   carousel path still works, and it splits into two bubbles. Needs porting to
   the current interactive/native-flow format. **Biggest functional gap.**
2. **Poll results always return 404** (#60). Votes arrive in the webhook but
   `GetPollResults` finds none — likely the stored `poll_message_id` /
   `instance_id` doesn't match the query. Needs tracing `SavePollVote` vs
   `GetPollResults`.
3. **Disappearing-messages timer not applied to outgoing messages** (#79), so
   recipients see "this message will not disappear". Needs the chat's ephemeral
   expiration copied into the outgoing message/context.
4. **Error 463 / NCT tokens not persisted** (#124, #50). The whatsmeow bump
   applied here plus the shared auth-store fix may already help; verify on a
   previously-affected instance before deeper work.
5. **Push notifications suppressed after connecting** (#70 23 comments, #54,
   #55). 0.7.2 already respects `alwaysOnline` on the `Connected` path, but the
   reports persist — audit every `SendPresence(PresenceAvailable)` call site
   (typing, subscribe, presence loop) when `alwaysOnline=false`.
6. **Passkey events / ceremony** (#105, #107, #172, #173). `PASSKEY*` event
   groups are not in `event_types` or the subscription filter, so they can never
   reach a webhook; the ceremony state machine also gets stuck.
7. **Media fidelity** (#104 missing image width/height → square placeholder,
   #103 link thumbnail not uploaded).
8. **Group announcement mode / settings** (#113, #98, #42) — the
   `/group/settings` route exists; the service implementation appears partial.

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
