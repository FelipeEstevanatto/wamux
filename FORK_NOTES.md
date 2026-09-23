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

## 1b. Distribution & compliance

This is an **unofficial community fork**. Nothing here is affiliated with,
endorsed by, or an official release of Evolution Foundation.

- **Version**: the `VERSION` file is the single source of truth (the Dockerfile,
  the GHCR workflow and `cmd/evolution-go/main.go` all read it). It is now
  `0.8.0`; the Makefile used to grep `CHANGELOG.md`, which started matching
  unrelated text (`amqp091-go`) and produced a broken `-X main.version`.
- **Image**: `ghcr.io/felipeestevanatto/evo-gofork`, published by
  `.github/workflows/publish_docker_image.yml` with the automatic
  `GITHUB_TOKEN`. It is never pushed to the upstream `evoapicloud/evolution-go`
  Docker Hub repo. OCI labels mark it unofficial.
- **Apache-2.0 §4**: the image ships `LICENSE`, `NOTICE`, `TRADEMARKS.md` and
  this file under `/app` (the Dockerfile copies them into the final stage).
- **Usage notification (LICENSE additional condition 1.b)**: the manager has a
  **Sobre / About** page (`/manager/about`) stating that the system uses
  Evolution Go, reachable from the sidebar. `README.md` documents the
  obligation for anyone embedding this image in another product.
- **Brand assets**: the manager keeps the Evolution Go copyright line (correct
  attribution, previously "© Evolution GO") and adds a visible fork disclaimer.
  It does **not** add the official logo. Note the tension between LICENSE
  condition 1.a (do not remove the LOGO/copyright from the console) and
  `TRADEMARKS.md` §4.2 (a *modified* UI must remove the brand assets and pick a
  clearly distinct name): this fork's UI is modified, so §4.2 arguably applies,
  while removing the logo triggers 1.a. Any use not expressly permitted needs
  written permission (`TRADEMARKS.md` §5, suporte@evofoundation.com.br).

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
| Poll votes from any contact other than the poll author never decrypted (`cipher: message authentication failed`), so `GET /polls/:id/results` only ever saw the author's own vote. The JID swap rewrites the event's `Sender`/`Chat` before decryption, and whatsmeow derives the vote's GCM additional data from those. Decryption now runs before the swap. | #60 |
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
| Images/videos carried no pixel dimensions, so clients drew a square/generic placeholder until the media loaded (#104); link previews embedded the raw `og:image` bytes (often PNG/WebP) in the JPEG-only `JPEGThumbnail` field and never uploaded a preview (#103). Images and video notes now set width/height (video also seconds + a first-frame JPEG), and link previews are converted to JPEG, sized, and uploaded so the large preview renders. | #104, #103 |
| Webhook delivery: a dotless queue name (e.g. `sendstatus`) silently dropped the event before any HTTP call; delivery now uses exponential backoff, skips retrying 4xx (except 408/429), caps the read body at 8 KiB, and bounds concurrent deliveries. | NathanAshford; supersedes our earlier webhook timeout-only change |
| Proxy tooling: `GET /instance/proxy/:id`, `POST .../test`, `POST .../reconnect` — checks reachability, the exit IP vs the server IP, and whether WhatsApp is reachable through it. | NathanAshford |
| Account limits: `GET /instance/limits/:instanceId` — WhatsApp reachout timelock and new-chat messaging quota (the limits behind error 463), cached on connect with a live fallback. | NathanAshford (`pkg/walimits`) |
| Outgoing messages did not respect the chat's disappearing-messages timer, so the recipient saw "This message will not disappear". The chat's timer is now cached (learned from `EPHEMERAL_SETTING` notifications and received ephemeral messages, and read once from group metadata) and stamped onto every outgoing message's `ContextInfo.Expiration`; `POST /chat/ephemeral` sets the timer and remembers it. | #79 |

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

*All items originally listed here have been resolved; see below. New issues
should be triaged and added here as they are reported.*

### Resolved from this list

- **Passkey events / ceremony** (#105, #107, #172, #173). Two independent bugs in
  the passkey (WebAuthn) pairing flow, plus the stale-ceremony case:
  - `PasskeyRequest` / `PasskeyConfirmation` / `PasskeyError` had no case in the
    `CallWebhook` subscription filter, so they hit `default: return` and were
    silently dropped. (The `===== DISPATCHING WEBHOOK =====` log is printed
    before the filter runs, which made it look like they were sent.) A dedicated
    `PASSKEY` event type was added, and the events also go to `QRCODE`
    subscribers since they are part of the #wapk pairing flow.
  - The `ALL` subscription was only recognised as the **first** element of
    `subscribe`, and was expanded to `AllEventTypes` — which does not contain
    `"ALL"` — so the literal `ALL` was never stored and `CallWebhook`'s
    all-events fast-path never fired. `ALL` in any position now persists the
    literal `ALL` (so it also covers event types added later).
  - The ceremony store kept a stale challenge when the socket dropped and the
    instance reconnected: the new socket has a fresh pairing context, so the old
    ceremony could never complete. It is now cleared on `Disconnected` /
    `StreamReplaced`; `Clear` reports whether anything was removed so the clear
    is logged only when real.
  Verified live: `subscribe:["PASSKEY"]` is accepted and stored (no *"Message
  type discarded"*), and `subscribe:["ALL"]` stores `"ALL"`. The ceremony clear
  is covered by a store unit test (no passkey-locked account was available to
  drive a real ceremony). #172 (passkey pairing) is already implemented in this
  fork via the `pkg/passkey` bridge; #173 is about the browser extension
  (`tools/passkey-helper`: MAIN-world WebAuthn plus a service worker for
  1Password), which is not shipped in this repo.

- **Push notifications suppressed after connecting** (#70, #54, #55). The
  `Connected` path already sent `Unavailable` when `alwaysOnline=false`, but
  three other call sites could still leave the linked device "available", which
  makes WhatsApp stop notifying the operator's phone:
  - `POST /message/presence` (typing) and `POST /message/subscribe` marked the
    device available and never restored it. Both now return to the instance's
    configured presence afterwards via `presenceAfterTransientOnline` (available
    when `alwaysOnline=true`, otherwise unavailable). Presence subscription
    inherently needs a continuously-online device, so when `alwaysOnline=false`
    it logs a warning that live `Presence` events require `alwaysOnline=true`.
  - The periodic presence loop is only started for `alwaysOnline` instances, but
    the flag can be toggled while it runs. It now re-reads the instance row each
    tick and stops as soon as `alwaysOnline` is off, instead of going available
    again.
  - `PUT /instance/:id/advanced-settings` now applies `Unavailable` immediately
    on a true→false transition, rather than waiting for the loop's next tick
    (which can be hours away). (Turning it on still takes effect on the next
    connect, where the loop is started.)
  Verified live: both endpoints log *"Restored presence to unavailable"*; a
  runtime toggle-off logs *"alwaysOnline is off, stopping presence updates"*;
  the advanced-settings transition logs *"Marked self as unavailable
  (alwaysOnline turned off)"*; sending still works throughout.

- **Error 463 / NCT tokens** (#124, #50).
  - **#50 (tctoken/cstoken not persisted after inbound messages)** is fixed by
    the whatsmeow bump: this version implements the full token lifecycle
    (`ensureTCToken`, `IncludePrivacyToken`, inbound extraction in
    `message.go` → `PutPrivacyTokens`, the `nct_salt_sync` app-state mutation,
    and the `whatsmeow_privacy_tokens` / `whatsmeow_nct_salt` stores). No fork
    change was needed.
  - **#124 (pre-v0.7.2 instances never receive the NCT salt)** is fixed here.
    Those instances did their one-time HistorySync with the old fork, which had
    no NCT concept, and whatsmeow's `handleAppStateSyncKeyShare` calls
    `FetchAppState(..., onlyIfNotSynced=true)`, so an already-synced
    `regular_high` is never re-read and the salt is never backfilled — making
    error 463 permanent and silent on cold 1:1 sends. On `events.Connected`,
    when no salt is stored, the fork now forces
    `FetchAppState(ctx, appstate.WAPatchRegularHigh, true, false)` once per
    instance, rate-limited to one try per 6 h (`reserveNctSaltSync`), mirroring
    Baileys' `ensureNctSaltSynced()`. Verified live: the forced sync runs
    cleanly, `regular_high` keeps a valid version/hash, no app-state event flood
    (`EmitAppStateEventsOnFullSync` is default false), the instance keeps
    sending, and the try is not repeated within the cooldown. (The test account
    itself has no salt provisioned server-side, so the sync legitimately finds
    none — the log distinguishes "backfilled" from "none provisioned".)
- **Interactive buttons / lists render** (#170, #59, #71, #110, #51, #69).
  Reply/CTA buttons were still built with an incomplete relay node — a `<biz>`
  with no `actual_actors`/`host_storage`/`privacy_mode_ts`, no `quality_control`,
  and `native_flow v="2"`. Completed to the reference shape (`v="9"`,
  `quality_control`, `<bot>` before `<biz>`); delivered with receipts and
  confirmed rendering on mobile. Lists and PIX were already fixed (see §3g).
- **Disappearing-messages timer** (#79). Inbound `EPHEMERAL_SETTING` is cached
  per chat and applied to outgoing messages as `ContextInfo.Expiration`. Verified
  live: a change made on the phone is captured, and `POST /chat/ephemeral`
  enable (86400/604800) / disable (0) is reflected on the next outgoing message.
  The residual client notice *"Disappearing messages are not supported in this
  chat"* is WhatsApp's judgement about the linked-device peer, not the payload —
  it cannot be cleared from message content.


## 3f. Media processing: ffmpeg dependency and running outside Docker

Some media features shell out to external binaries that live in the **runtime
image**, not in the Go binary:

| Tool | Version in this image | Used for |
|---|---|---|
| `ffprobe` | **ffmpeg-8.1.2-r0** (Alpine 3.24) | video duration/width/height (issue #104) |
| `ffmpeg` | **ffmpeg-8.1.2-r0** (Alpine 3.24) | first-frame JPEG thumbnail; audio → Opus conversion |
| `pdftoppm` | `poppler-utils` 25.12.0-r1 (Alpine 3.24) | PDF document thumbnails |

The runtime base was `alpine:3.19.1` (Alpine 3.19 had become end-of-life and no
longer received security updates: 1 critical / 6 high / 11 medium findings). It
was moved to **`alpine:3.24`**, matching the build stage
(`golang:1.26-alpine` = 3.24.2). This both clears those CVEs and removes a
cross-version libc mismatch: the CGO binary is built against 3.24's musl and was
previously run on 3.19's. The Go binary's only dynamic dependency is musl
(`ldd` → `ld-musl-x86_64.so.1`); libwebp/libjpeg are linked statically, and the
runtime `libjpeg-turbo`/`libwebp` packages exist for ffmpeg.

Upgrade check: every runtime package still exists in 3.24
(`tzdata`, `ffmpeg`, `libjpeg-turbo`, `libwebp`, `poppler-utils`) and the exact
ffprobe/ffmpeg/pdftoppm invocations used below were re-verified on ffmpeg 8.1.2 /
poppler 25.12 (same output shape: dimensions, duration, frame thumbnail, Opus).

On Alpine the `ffmpeg` package provides **both** `ffmpeg` and `ffprobe`
(`/usr/bin/ffmpeg`, `/usr/bin/ffprobe`). The Dockerfile runtime stage installs
`tzdata ffmpeg libjpeg-turbo libwebp poppler-utils`; that line is **upstream
0.7.2** — this fork only changed the Go base image and the runtime Alpine tag.

**Caveat for a non-Docker deployment.** If the compiled binary is run directly on
a host (not in this image), those tools are not present and the affected features
degrade silently rather than failing:

- video messages ship **without** dimensions/duration/first-frame thumbnail (the
  bubble shows a generic placeholder);
- audio conversion to Opus, PDF thumbnails and other shell-outs fail the same
  way (the request may error where the container would have succeeded).

Install `ffmpeg` and `poppler-utils` on the host to restore parity, or run the
container. The helpers check `exec.LookPath` first, so a missing tool only drops
the enhancement; `go test ./pkg/sendMessage/...` skips the video test when
`ffprobe` is unavailable locally.

The exact version moves with the Alpine base tag and is not pinned in the
Dockerfile; `docker exec <container> apk info --who-owns /usr/bin/ffprobe`
reports it.

## 3g. Interactive messages: per-client rendering findings

Live-tested behaviour of the interactive message types against a real account (Oct 2026). These are WhatsApp-client behaviours, not bugs we can fix in the sender — recorded so they aren't re-investigated.

**Delivery vs rendering.** A message can be *delivered* (a `Receipt` event arrives, echoed by the recipient's devices) and still not *render*. Check receipts first when debugging: `/send/list` used to return success with a message id and produce **no receipt at all** — the server silently dropped it.

| Type | Wire shape | Delivered | Mobile | Web/Desktop |
|---|---|---|---|---|
| reply / CTA buttons | `Message.interactiveMessage` + `nativeFlowMessage` | yes | renders | renders |
| PIX (`payment_info`) | `Message.interactiveMessage` (NO `viewOnceMessage`) + flat `<biz native_flow_name="payment_info"/>` | yes | renders | renders |
| list | `Message.interactiveMessage` + `single_select` button | yes | **renders** | *"This message couldn't load. Open the message on your phone to view it."* |
| carousel | `Message.interactiveMessage` + `carouselMessage` | yes | renders **only with media on every card** | renders |

**Legacy `listMessage` is dead.** The top-level (and `documentWithCaptionMessage`-wrapped) legacy `listMessage` is accepted by the API but **never delivered** — no receipt, nothing on any client. Lists must use the `interactiveMessage` + `single_select` form. Reuse `sectionsToString` for the button params.

**`viewOnceMessage` breaks interactive messages.** Wrapping PIX (and previously reply/CTA) in `viewOnceMessage` makes Web show *"couldn't load"* and the phone drop it. Interactive messages must be top-level.

**Web/desktop can differ from mobile.** The `single_select` list renders on mobile but not on Web/Desktop (a known WhatsApp limitation — the modern list format is not supported by the web client). The reverse happens for a text-only carousel: Web renders it, mobile does not.

**Carousel cards require media.** WhatsApp's carousel format requires an image or video header on **every** card. The endpoint currently accepts text-only cards; those are delivered and render on Web but show nothing on mobile. Always supply `header.imageUrl` (or `videoUrl`) per card.

**`carouselCardType` is intentionally unset.** None of the forks set it and it is not needed; the comments claiming an iOS requirement predate the current client.

## 3h. Review of other forks / Evolution API 2.4.0-rc — hypothesis check

Each fix/feature from `evolution-api` 2.4.0-rc1/rc2 (the reference Node implementation) and the Go forks was checked against this codebase. Verdicts below; "Not affected" items were tested live, not assumed.

| Item (source) | Verdict | Evidence / note |
|---|---|---|
| `onWhatsApp` returns `exists:false` for `@lid` → sends throw (2.4.0-rc2 #2544) | **Not affected** | Sending directly to a `@lid` JID is delivered (receipt). whatsmeow resolves it; the Baileys LID bug does not exist here. |
| `quoted` not propagated on audio sends (2.4.0-rc2 #2516) | **Not affected** | The audio message's `contextInfo` carries `stanzaID` + `participant`; the quote threads correctly. |
| Instance name not trimmed on create → 404 by name (2.4.0-rc2 #2546) | **Fixed** | `Create` now trims the name (and rejects an all-space name); `Rename` already trimmed. Verified: `"  padded-name  "` stores `"padded-name"`. |
| Interactive buttons: max 2 CTA, no mixing with reply/PIX (2.4.0-rc1) | **Fixed** | reply↔others and PIX isolation were already enforced; added the **max 2 CTA (url/copy/call)** rule. Verified: 3 CTA now returns 400, 2 CTA still sends. |
| Carousel: single card without image falls back to `nativeFlowMessage` (2.4.0-rc1) | **Fixed** | A one-card, no-media carousel is now sent as a plain interactive message. Verified: top-level `interactiveMessage`, not a carousel. |
| List: switched to legacy `listMessage` (2.4.0-rc1) | **Superseded** | The legacy `listMessage` is silently dropped by the current server; we send `interactiveMessage` + `single_select` instead (see §3g). |
| PIX (`payment_info`) support (2.3.7 / 2.4.0-rc1) | **Fixed** | §3g. |
| Native GIF / `gifPlayback` (2.4.0-rc2 #2540) | **Fixed** | `type: "gif"` (or `gifPlayback: true` on a GIF) transcodes the animated GIF to a silent MP4 with ffmpeg and sends it as a `VideoMessage` with `gifPlayback=true` — a looping animation without controls. See note below. |
| Path traversal in `/assets` (2.3.3, CRITICAL) | **Not affected** | We mount assets via gin's `Static` (Go `http.FileServer`), which cleans paths; the vuln was in their custom handler. |
| Incoming events stop after reconnect (Baileys/rxjs) (2.3.7) | **Not applicable** | Baileys' RxJS subject lifecycle; whatsmeow registers its event handler differently. |
| License activation required (2.4.0-rc1, breaking) | **N/A** | This fork removed licensing entirely. |
| History-sync `isLatest`/`progress` in the event payload (2.3.7) | **Already present** | We forward the raw whatsmeow event, whose `HistorySync` already carries `IsLatest` and `Progress`. |
| mediaKey conversion to avoid bad-decrypt (2.3.3) | **Not applicable** | Baileys-specific key handling; no equivalent path here. |

### Sending GIFs

`POST /send/media` accepts `type: "gif"` (or `type: "video"` with `gifPlayback: true`).
WhatsApp does not accept a raw GIF as video, so the animated GIF is transcoded to a
silent H.264 MP4 with ffmpeg (`convertGifToMP4`) and sent as a `VideoMessage` with
`gifPlayback=true` — the client autoplays it as a looping animation without
controls. Width/height, duration and a first-frame thumbnail are attached as for
any video. Two ffmpeg details worth remembering if this is touched again: the MP4
muxer needs a **seekable output** (a pipe fails with "muxer does not support non
seekable output"), and the output file must be passed with **`-y`** because the
temp file already exists.

## 3i. Manager SPA vs. the self-hosted dashboard

Two different UIs are served from the same origin:

| Route | Source | Editable here? |
|---|---|---|
| `/manager` (+ `/manager/*`) | React SPA. Source is **vendored** at `evolution-go-manager/`; the served build is `manager/dist/index.html` + `manager/dist/assets/index-*.js`/`.css`. | **Yes** — edit `evolution-go-manager/src/`, then `make manager-build` (or let Docker rebuild it, below). |
| `/dashboard` | **Hand-written** static page `manager/dist/dashboard.html` (plain HTML + vanilla JS + Chart.js from CDN), added by this fork. | **Yes** — no build step; edit the file and reload. |

`/dashboard` is the fork's own operational view. It shows instance KPIs, a
connection donut, messages/day, host RAM/load/goroutines/uptime, top
conversations, an instance table (avatar + contact count) and per-instance logs,
and it supports `?embed=1` so it can be iframed inside the Manager's Dashboard
tab.

### Manager source (`evolution-go-manager/`)

The manager source is **not** on upstream `main` (only the compiled `manager/dist`
is). It lives on upstream's **`develop`** branch under `evolution-go-manager/`,
and is vendored here from there (commit `706c9a4`, 2026-05-06). Keeping the same
path upstream uses means a future `develop`→`main` merge sees identical files.

Build it with:

```bash
make manager-install   # pnpm, bun or npm — whichever is installed
make manager-build     # builds and syncs into manager/dist (keeps dashboard.html)
```

`make manager-build` copies `evolution-go-manager/dist/{index.html,assets}` over
`manager/dist/` and leaves `manager/dist/dashboard.html` untouched (Vite does not
produce it). The Dockerfile does the same in a dedicated **`oven/bun`** stage, so
`docker compose up -d --build` rebuilds the SPA automatically; `bun install`
reads the committed `package-lock.json`, which pins `@evoapi/design-system` to
`0.0.5` (the root import is used; `0.0.6` changed the export layout).

Two caveats, both inherited from upstream:

- **`develop` is older than the bundle `main` shipped.** The previous committed
  `manager/dist` was `main`'s (688 KB JS); building `develop` gives ~682 KB. This
  fork ships the rebuilt `develop` output **plus two bug fixes** taken from the
  `ecosb2b/evo-go-v2` fork:
  - `services/api/instances.ts` read the QR fields capitalised (`data.Qrcode` /
    `data.Code`) while the API returns lowercase, so the QR modal opened blank
    with no error; both spellings are now accepted.
  - `components/instances/QRCodeModal.tsx` had `onRefresh` (and `instance`) in
    the auto-refresh effect's dependency list; both change on every refresh, so
    the interval was cleared and restarted before it ever fired — automatic QR
    refresh never ran. `onRefresh` now lives in a ref and the effect keys on
    `isConnected`.
- The manager still carries the vendor **license screen**; the local `/license/*`
  stub (see §1) keeps it satisfied, so nothing here talks to any Evolution
  server.

### Fork customisations to the manager

Beyond the QR fixes above, this fork adds (all under `evolution-go-manager/src/`):

- **Instances page** — each card shows the connected account's **profile picture**
  (falling back to initials), its **contact count**, **chat count** and **message
  count**. These come from `GET /instance/overview/:instanceId` (below), fetched
  once per instance per minute by `instancesStore.fetchOverviews`, so the 5 s
  instance poll stays cheap.
- **Instance settings** — the token has a **copy button** next to the
  reveal/hide eye (works while hidden or visible), and a **Proxy** card to
  set/get/test/reconnect/delete the instance proxy
  (`/instance/proxy/:instanceId*`, admin key).
- **Phone number display** — `normalizeInstance` strips the WhatsApp `:device`
  agent suffix from `jid` (`5514991421911:5@s.whatsapp.net` →
  `5514991421911`), so the "Número"/"Proprietário" fields no longer show `:5`.
- **Sidebar** — the running **version** (from `GET /server/stats` →
  `system.version`) and a **GitHub** link in the footer.
- **Dashboard page** — the upstream placeholder is replaced by a small
  system-wide view: instances (total/connected), messages, contacts, host
  RAM/load/goroutines/uptime and a messages-per-day bar chart.

The per-instance **message and chat counts** needed backend work: the `messages`
table had no instance attribution (`source` holds the contact number). A
nullable `instance_id` column was added (GORM AutoMigrate creates it), filled on
insert (`whatsmeow` event handler), and exposed as `messagesCount` /
`chatsCount` in `/instance/overview/:instanceId`. **Chats** is the number of
distinct contacts with at least one persisted message — the Go fork does not
store a chat list, so this is the closest available metric. The instance delete
path now also cleans up by `instance_id` (it previously filtered on `source`,
which never matched). Rows written before the column existed are not counted.

### Per-instance overview (profile picture + device + contacts + chats + messages)

`GET /instance/overview/:instanceId` (AuthAdmin) returns the connected
account's own profile picture, push name, the **platform of the phone that
paired**, local contact count, chat count and the number of messages persisted
for that instance:

```json
{"data":{"connected":true,"platform":"android","businessName":"…","profileName":"…","profilePicUrl":"https://pps.whatsapp.net/…","contactsCount":346,"chatsCount":12,"messagesCount":40}}
```

Backed by `whatsmeowService.GetInstanceOverview` (a preview profile-picture IQ,
time-bounded to 15 s, plus `Store.Contacts.GetAllContacts`) and
`MessageRepository.CountByInstance` / `CountChatsByInstance`. `GET /server/stats`
also reports `system.version` (the `-X main.version=` / `VERSION` value).

**Device info is the platform, not a model.** whatsmeow captures the phone's
`<platform name="…">` from the pair-success node into `store.Device.Platform`
(persisted as `whatsmeow_device.platform`, e.g. `android`, `ios`,
`smb_android`). It is read from the device store, so it is available even while
the instance is disconnected. WhatsApp does **not** send a phone model string
(and the older Evolution API/Baileys did not expose one either), so the UI shows
the platform — the closest obtainable value.

To add more widgets, edit `manager/dist/dashboard.html` (fork's own page) or
`evolution-go-manager/src/` (the SPA); if a value is missing from the API, add
it to `/server/stats` or `/instance/overview/:instanceId`.

### Dashboard UI: light mode, embedded theme and resolved conversation names

Three UI problems were found while testing the dashboard against a live account:

- **Light mode was broken when the OS was in dark mode.** Tailwind v4 defaults
  the `dark:` variant to `@media (prefers-color-scheme: dark)`, but the app
  switches themes with a `.dark` class on `<html>` (the JS `darkMode: 'class'`
  config is ignored by Tailwind v4 unless `@config` is used). With a dark OS and
  the app toggled to light, `dark:*` utilities kept applying — e.g.
  `dark:text-gray-200` left the header items light gray on the now-white header.
  Fixed in `globals.css` with `@custom-variant dark (&:is(.dark *));` so `dark:`
  follows the class. (Verified in the built CSS: zero
  `prefers-color-scheme: dark` blocks.)
- **The messages-per-day bar chart rendered nothing.** Each bar's height was a
  percentage, but its parent (a flex column) had no definite height, so the
  percentage resolved to zero and every bar collapsed. The bars now sit in a
  fixed-height (`h-32`) flex wrapper.
- **The embedded `/dashboard` was always dark.** It is a separate static page, so
  it did not follow the manager theme. It now reads `?theme=light|dark` (the
  manager passes its current theme when embedding), listens for an
  `egogo-theme` `postMessage`, and falls back to `prefers-color-scheme` when
  opened standalone. It is also rebranded to **Evo-GoFork** (the page still said
  "Evolution GO") and links to this fork's repository.

**"Conversas mais ativas" showed raw identifiers** (`+269182931329179`,
`+5514981170846`, `+status`). `/server/stats` now annotates each `topSources`
entry with a resolved `name` (`whatsmeowService.ResolveChatNames`): a saved
contact's name, the phone number a LID maps back to (`Store.LIDs.GetPNForLID`),
a group subject (`GetGroupInfo`), or `Status`/`Transmissão` for the special
sources. Resolution is tried across every connected client (the endpoint is
global and the stored `source` has no instance id) and cached for 10 minutes,
since the dashboard polls every ~15 s and group lookups are network IQs. When
nothing resolves, the frontend falls back to `+<key>`. Confirmed live: a LID and
a phone number both resolved to the saved contact's name, and `status` →
`Status`.


## 3j. Security audit & hardening

An audit (static review + `govulncheck` / `bun audit` + live runtime checks)
found the following. Each was reproduced against the running stack before being
fixed.

| # | Finding | Reproduced how | Status |
|---|---|---|---|
| S1 | SQL injection in `ForceUpdateJid` (`number` interpolated into `LIKE`) | `POST /instance/forcereconnect/:id` with `{"number":"zzz' UNION SELECT '1234567890:7@s.whatsapp.net' --"}` set the instance JID to the injected value (an escaped `LIKE '%zzz%'` matches 0 rows) | **Fixed** — parameterised: `LIKE '%' \|\| $1 \|\| '%'` |
| S2 | 5 reachable CVEs (`x/image` 2021 pin, `pgx/v5` 5.5.5, `amqp091-go` 1.10.0) | `govulncheck ./...` | **Fixed** — `x/image v0.46.0`, `pgx/v5 v5.11.0`, `amqp091-go v1.15.0`; `govulncheck` now reports **0** |
| S4 | Path traversal in `GetLogs` (`instanceId` joined into a path) | **not reachable over HTTP** — gin 404s on both `%2F` and a raw `..` (the `:instanceId` param never contains `/`) | **Fixed defensively** — UUID validation before `filepath.Join` |
| S5 | Instance delete left the whatsmeow device (sessions/keys/contacts) orphaned | cloned a device row, deleted the instance, the row survived | **Fixed** — `DeleteInstanceDevice` purges the device (FK cascade removes the rest); live device untouched |
| S7 | Admin key compared with `!=` | code review (a timing exploit was not demonstrated) | **Fixed** — `subtle.ConstantTimeCompare` |
| S11 | (a) `/instance/all` returns instance tokens; (b) `/swagger` public; (c) proxy password returned in cleartext | `curl` on each | (a)/(b) **documented trade-offs** (admin-only; Swagger is gateable with `SWAGGER_ENABLED=false`); (c) **fixed** — GET no longer returns the password (`hasPassword` instead), and an empty password on save keeps the stored one |

Not fixed here (lower priority, tracked for later): CORS `*` together with
credentials, the container running as root, no API rate limiting, SSRF on
user-supplied media/webhook/typebot URLs, and the proxy password stored in
plaintext at rest.

### `golang.org/x/image` pin

It was pinned at `v0.0.0-20211028202545-6944b10bf410` (a 2021 commit) and is a
**direct** import (`webp.Decode` in `pkg/whatsmeow/service/whatsmeow.go`).
`go mod graph` shows the version was inherited from
`github.com/chai2010/webp v1.1.1` (also a direct dependency, used in
`pkg/sendMessage/service/send_service.go`), which requires exactly that
pseudo-version; MVS takes the maximum and nothing required anything higher.
There was no functional reason for it — the `x/image/webp` API is stable — so it
is now `v0.46.0`.

### Message persistence (parity with Evolution API)

Only *received* messages used to be stored (`Status="Received"`), so the
per-instance counts and `/server/stats` ignored everything the instance sent.
Now:

- the central `SendMessage` persists outgoing messages as `Status="Sent"`
  (`sendService.persistSentMessage`, best-effort and asynchronous);
- the inbound event handler records `IsFromMe` echoes as `Sent` and true inbound
  as `Received`;
- the Read/Delivered receipt upserts carry `instance_id` too.

The `message_id` unique key makes all three idempotent, and the receipt upsert
does not touch `instance_id` (it is not in `messageUpdateColumns`), so the
attribution survives a status change.

## 3k. whatsmeow API audit

Compared our usage against whatsmeow's exported surface (136 `Client` methods;
45 event types in `types/events`). We call 66 methods and handle the events
listed in §3g/§3j; no deprecated method is used. Each finding below was
reproduced before fixing and re-verified after (one commit each).

| Item | Finding | Fix |
|---|---|---|
| Media retry | `SendMediaRetryReceipt` was never called and `events.MediaRetry` never handled, so media that 403/404/410s stayed undownloadable forever | `RequestMediaRetry` / `HandleMediaRetry` + a retry cache; `/message/downloadmedia` accepts optional message context (`id`/`chat`/`fromMe`/`isGroup`/`participant`) |
| `WaitForConnection` | fixed `time.Sleep(2s)` guesses before using a just-started client (a slow connect failed early) | `waitForClient` uses `client.WaitForConnection` (paired devices only; pairing still needs the QR, so its sleeps stay) |
| Retry-receipt cap | `SetMaxParallelRetryReceiptHandling` defaults to **unlimited** and was never set | capped at 10 before connect |
| Reactions | hand-built `MessageKey`: trusted the API `fromMe` and set `participant` even for 1:1 | `client.BuildReaction` / `BuildMessageKey` with a unit-tested author derivation |
| `StreamError` / `KeepAliveTimeout` | unhandled → a hung socket waited for the TCP drop | reconnect via the extracted `scheduleReconnect` (acts on the 2nd consecutive keepalive timeout) |
| `PairError` | unhandled → only an "Unhandled event" warning; a passkey ceremony stayed stuck | logged + `SetError` on the ceremony + webhook |
| `NotifyAccountReachoutTimelock` | unhandled (the push side of error 463) | forwarded as `AccountReachoutTimelock` to CONNECTION subscribers |
| More webhooks | `PrivacySettings`, `Blocklist`, `NewsletterLiveUpdate`, `NewsletterMuteChange` unhandled | forwarded to CONNECTION / NEWSLETTER subscribers |

Deliberately **not** done: `BlocklistChange` and `NewsletterMessageMeta` are not
dispatched standalone (they are carried inside `Blocklist` / `Message`), and
`AcceptTOSNotice`, `GetUserDevices`, `GetBusinessProfile`, `SetMediaHTTPClient`,
`SetDefaultDisappearingTimer` remain unused (niche or low value).

## 4. Build & run

From the repository root (the `docker-compose.yml` is there):

```bash
cp .env.example .env     # set GLOBAL_API_KEY (required)
docker compose up -d --build
```

- API: <http://localhost:8081> (override with `EVOGO_PORT`)
- Swagger: <http://localhost:8081/swagger/index.html>
- Manager: <http://localhost:8081/manager> (log in with `GLOBAL_API_KEY`)
- Dashboard (fork's own, editable): <http://localhost:8081/dashboard> (same key; see §3i)
- Postgres is bundled and databases are auto-created.

`docker compose up --build` also builds the manager SPA from
`evolution-go-manager/` (bun stage), so frontend edits are picked up by the same
command. To build it locally instead, use `make manager-build` (§3i).

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

- The Manager SPA source is vendored at `evolution-go-manager/`, taken from
  upstream's `develop` branch (the only place it exists — `main` ships only the
  compiled `manager/dist`). `develop` is an older revision than the bundle
  `main` previously shipped, so the rebuilt `manager/dist` differs slightly; see
  §3i for the two bug fixes applied and how to rebuild.
- Swagger docs (`docs/`) are regenerated with `swag init --parseDependency`
  (plain `swag init` cannot resolve `gin.H`/`types.JID`). This picked up the
  endpoints added by this fork and dropped the stale `/license/*` paths, so the
  frontend API Tester (which reads the live `/swagger/doc.json`) is current.
  Regenerate with `make swagger` after changing routes/annotations.
- The upstream LICENSE still applies (Apache 2.0 plus its brand-protection and
  attribution conditions). Removing the runtime *activation* does not change the
  license terms of the source — see `LICENSE` and `TRADEMARKS.md`, and note that
  retaining the project's logos/copyright in the frontend is still required by
  that license.
