# Evolution GO - Changelog

## Unreleased — `fork/community-stable`

Fork-specific fixes on top of `0.7.2` (see `FORK_NOTES.md` for the full list).

### 🐛 Bug Fixes
- **`GET /group/myall` always returned an empty list** — the owner filter
  compared `types.GroupInfo.OwnerJID` against a JID parsed with `utils.ParseJID`,
  which prefixes phone numbers with `"+"`, so it never equalled WhatsApp's owner
  JID; and on LID-addressed accounts the owner is reported as a LID while the
  account's own `Store.ID` is a phone number. The filter now normalises both
  sides (`ToNonAD()`) and matches `OwnerJID`/`OwnerPN` against both `Store.ID`
  and `Store.LID`.
- **Permanent error 463 on cold sends for instances paired before v0.7.2**
  (#124) — those instances never received the NCT salt (their one-time
  HistorySync ran under the old fork), and whatsmeow only re-reads app-state
  categories that are not yet marked synced, so the salt was never backfilled.
  On `Connected`, when no salt is stored, the fork now forces a `regular_high`
  full sync once per instance (rate-limited to one try per 6 h), mirroring
  Baileys' `ensureNctSaltSynced()`.

### ✨ Features
- **Typebot integration** — bot CRUD, per-contact sessions, `startChat`/
  `continueChat`, plus flood/loop protections and a `TypebotAutoPaused` alert.
  Endpoints under `/typebot` (instance-token auth). Config:
  `TYPEBOT_CONTACT_RATE_LIMIT`, `TYPEBOT_CONTACT_RATE_WINDOW`,
  `TYPEBOT_SEND_RATE_LIMIT`, `TYPEBOT_SEND_RATE_BURST`.
- **`POST /send/event`** — WhatsApp event/calendar message (`waE2E.EventMessage`);
  ISO 8601 or epoch times.
- **`POST /send/product`** — catalog product card (`waE2E.ProductMessage`).
- **`PUT /instance/name/:instanceId`** — rename an instance (id/token unchanged).
- **`GET /server/stats`** and **`GET /dashboard`** — runtime/host metrics and
  message aggregates, plus a self-hosted dashboard page.
- **Multiple webhooks per instance** — the `Webhook` field accepts a JSON array or
  a newline/comma/semicolon separated list; the payload is delivered to each URL.
- **`POST /user/savecontact`** route aligned (with a legacy `POST /user/contacts`
  alias) and app-state desync recovery.

### 🔧 Improvements
- **Animated WebP stickers** are uploaded untouched instead of being re-encoded,
  which previously failed on animated WebP and flattened static WebP.
- **Interactive buttons/Pix** rewritten to the `native_flow` payloads WhatsApp
  actually renders (reply/CTA top-level `interactiveMessage`; Pix via
  `ViewOnceMessage`). Reply/CTA relay node later completed with the
  `actual_actors`/`host_storage`/`privacy_mode_ts` attributes, `quality_control`
  child and `native_flow v="9"`; confirmed rendering on mobile.
- **Shared sqlstore container** no longer caches a transient database failure.

### 🐛 Bug Fixes
- **`POST /send/event` and `POST /send/product` failed with `invalid
  messageType`** — `SendMessage` now registers both types (quoted and
  non-quoted ContextInfo).
- **Mentions were ignored on `EventMessage`** — `mentionAll`/`mentionedJid` now
  apply to events too.
- **`POST /group/create` and `POST /group/participant` hung with
  `"info query timed out"` for phone-number participants** — `utils.ParseJID`
  emitted `+<number>@s.whatsapp.net`, which WhatsApp cannot resolve, so the
  request IQ was dropped (passing a LID worked, which hid the bug). Participants
  are now canonicalised with `utils.CanonicalJID` (strips the `+`, leaves
  LID/group JIDs untouched) before being sent.

## v0.7.2

**Docker:** `evoapicloud/evolution-go:0.7.2`

### 🆕 New Features
- **Passkey (WebAuthn) pairing** — support for linking accounts that the WhatsApp
  server locks behind a **passkey** (the *Shortcake* / CRSC flow). When the
  server demands a passkey, whatsmeow's `PairPasskeyRequest` is surfaced through a
  new ceremony flow: the backend mints a short-lived ceremony token, and a bundled
  browser extension (`passkey-helper`) runs the WebAuthn assertion on the
  `web.whatsapp.com` origin and posts it back. Three public endpoints drive it:
  `GET /passkey-ceremony/{token}`, `POST .../response`, `POST .../confirm`. The
  manager detects the passkey stage and shows an "Abrir WhatsApp Web" button.
  Confirmation is always manual (never auto-confirm on `SkipHandoffUX`).
  Configure the public API base via **`PASSKEY_PUBLIC_URL`**. Full guide:
  `docs/wiki/guias-api/passkey-pairing.md`. Note: there is no headless bypass —
  the ceremony requires the account owner's real authenticator; the extension is
  web-only.
- **Headless license auto-activation** — set `EVOLUTION_OPERATOR_EMAIL` to the
  email used in your first manual license registration; on startup the service
  silently calls `/v1/register/auto` and skips the browser flow (falls back to the
  manual flow if the email isn't registered yet).
- **Button message media support** — additional media handling for interactive
  button messages.

### 🔧 Improvements / CI
- **Dropped the whatsmeow fork — now uses official `go.mau.fi/whatsmeow`.** The
  project previously vendored a fork (`whatsmeow-lib` submodule) to carry a
  PostgreSQL pool patch; upstream rejected that patch in favor of `NewWithDB`
  (app-side config). Removing the fork also pulled in upstream's native passkey
  support. Pinned to the commit that adds passkeys
  (`v0.0.0-20260630180629-b572e5bcb92b`). The `sync-releases` workflow no longer
  re-adds the submodule.
- **QR pairing consumes `events.QR` directly** instead of `GetQRChannel`. The QR
  channel auto-confirms passkey on `SkipHandoffUX` and disconnects the socket when
  codes run out — both break an in-flight passkey ceremony. Connecting without it
  keeps the socket alive for as long as pairing (QR or passkey) needs. QR rotation
  now pauses while a passkey ceremony is active.
- **Public sync fixes** — the release workflow drops the obsolete whatsmeow-lib
  step, targets `evolution-foundation/*`, and now ships the `passkey-helper`
  extension to the public repo.

### 🐛 Bug Fixes
- **`POST /instance/pair` returned an empty `PairingCode`** — the handler
  swallowed `PairPhone` errors and returned HTTP 200 with `PairingCode: ""`, and
  the client wasn't connected/awaiting-auth before `PairPhone`. Now starts the
  instance, waits for the websocket, and surfaces real errors (#21).
- **`GET /instance/status` returned 400 after a manual disconnect** — now returns
  200 with the disconnected status instead of erroring until a container restart
  (#20).

### 🏷️ Org rename
- Repository references updated from **EvolutionAPI** to **evolution-foundation**
  (module path, imports, GitHub URLs, submodule URLs).

## v0.7.1

**Docker:** `evoapicloud/evolution-go:0.7.1`

### 🆕 New Features
- **Test-send modal in Manager** — new modal in the embedded manager UI to test message sending directly from the panel, covering text, media and interactive message types. Useful for validating an instance right after pairing without leaving the manager.

### 🔧 Improvements / CI
- **whatsmeow-lib SHA now pinned in the public sync** — the `sync-releases` workflow previously re-cloned whatsmeow `main` on every run, so the SHA listed in the CHANGELOG could drift from what the public repos actually built against. The workflow now captures the SHA from the dev submodule and checks out that exact commit in the target, restoring release reproducibility.
- **Repository cleanup** — dropped tracked binaries (`evolution-go`, `build/server`), IDE config (`.idea/`) and scratch files (`DIFF-COMPLETO.txt`, `API-INTERACTIVE-DOCS.txt`, `carousel-sender.html`). Expanded `.gitignore` to prevent reincidence.

### 📝 Docs
- **Postman collection** — added `Set Proxy` request and multipart hints on `/send/media`; collection file renamed from `Evolution GO.postman_collection (2).json` to `Evolution GO.postman_collection.json`.
- **Interactive messages docs** — additional examples and corrections.

## v0.7.0

**Docker:** `evoapicloud/evolution-go:0.7.0`

### 🆕 New Features
- **Multi-platform interactive messages** — Buttons, lists and carousel working on Android, iOS and WhatsApp Web/Desktop
  - **SendButton**: removed `ViewOnceMessage` wrapper that blocked rendering on iOS and WhatsApp Web; `Footer` and `Header` are now conditional
  - **SendList**: migrated from `InteractiveMessage`/`NativeFlowMessage` to legacy `ListMessage` (native protobuf) for broad compatibility
  - **SendCarousel**: new endpoint `POST /send/carousel` with cards (image, text, footer, buttons) and automatic JPEG thumbnail generation for instant image loading
  - `whatsmeow-lib`: added `biz` node for `InteractiveMessage` and pinned `product_list` type on the `biz` node for `ListMessage`
- **Base64 media support on `/send/media`** — The `url` field on `POST /send/media` now also accepts base64-encoded media. When the value does not start with `http://` or `https://`, it is treated as base64 and decoded; reuses the existing `SendMediaFile` flow
- **WhatsApp status endpoints** — new `POST /send/status/text` and `POST /send/status/media` publish text/image/video status to `status@broadcast`. Media endpoint supports both JSON (with URL) and multipart/form-data (file upload). Thanks @Eduardo-gato (#15)
- **Webhook routing for GROUP / NEWSLETTER** — when the primary `MESSAGE` / `SEND_MESSAGE` / `READ_RECEIPT` subscription is absent, events from `@g.us` chats are forwarded to `GROUP` subscribers and events from `@newsletter` chats to `NEWSLETTER` subscribers. Thanks @oismaelash (#18)

### 🔧 Improvements
- **Proxy protocol** — new optional `protocol` field (and `PROXY_PROTOCOL` env) supporting `http`, `https`, `socks5`. Replaces the hardcoded SOCKS5 dialer with `client.SetProxyAddress`, fixing HTTP-proxy QR pairing (#12). Thanks @TBDevMaster (#13)
- **WhatsApp Web version cache** — `fetchWhatsAppWebVersion` now caches the result for 1 hour with a mutex instead of issuing one request per instance startup. Thanks @VitorS0uza (#24)
- **Manager flicker fix** — instance page no longer replaces the list with skeleton cards on every 5s polling cycle (`hasLoaded` flag). Thanks @TBDevMaster (#14), closes #11
- **`WEBHOOKFILES` → `WEBHOOK_FILES`** — `.env.example`, docker-compose and docs aligned with the env var the runtime actually reads. Thanks @VitorS0uza (#22)
- **Dependency cleanup** — removed unused `github.com/evolution-foundation/evo-gate` from `go.mod`
- **whatsmeow-lib** bumped to `0923702fb`
- **Telemetry removed** — dropped legacy `pkg/telemetry`

### 🐛 Bug Fixes
- **`/message/edit`** — was silently ignored because the edit payload used `Conversation` while the original message was sent as `ExtendedTextMessage`. WhatsApp requires matching types; now the edit uses `ExtendedTextMessage` and the response returns the actual server timestamp instead of the zero value. Closes #16
- **Sticker upload to S3/MinIO** — when `webp.Decode` or `png.Encode` failed, the whole media pipeline aborted and the sticker was lost from the webhook. Now we log a warning and keep the raw `.webp` bytes so the sticker still reaches the bucket. Closes #5
- **Multipart `/send/media`** — the binary-upload branch silently dropped `mentionAll`, `mentionedJid` and `quoted`. These fields now parse from the form (with `mentionedJid` accepting repeated or comma-separated values) and reach the send service. Closes #2

### ⚠️ Breaking changes
- **Proxy** — previously all proxies were forced through SOCKS5. If you run SOCKS5 on a non-standard port (anything outside 1080/2080/42000-43000), set `PROXY_PROTOCOL=socks5` in the env or pass `"protocol": "socks5"` in the proxy body explicitly — otherwise the new protocol inference will fall back to HTTP.

### 📝 Docs
- **README** — updated WhatsApp support number and issue templates
- **Interactive messages guide** — new `docs/wiki/guias-api/api-interactive.md`
- **Proxy docs** — environment variables, configuration guide and API reference updated with the new `protocol` field

## v0.6.1

### 🆕 New Features
- **Group invite info endpoint** — `GET /group/invite-info` to get group details from invite link
- **Enhanced media sending** — GIF playback, video stickers, and transparent sticker support

### 🐛 Bug Fixes
- **Admin revoke** — Allow deleting messages from others in groups (admin revoke)

### 🔧 Improvements
- **Version management** — Reads version from `VERSION` file with ldflags fallback
- **CORS global middleware** — Applied before all routes
- **Makefile compatibility** — Fixed `$(shell)` syntax for GNU Make 3.81 (macOS default)
- **CI/CD cleanup** — Removed `develop` branch trigger and `homolog` tag from Docker workflow
- **README updated** — New links, documentation, and hosting info

## v0.6.0

### 🆕 New Features
- **Version from VERSION file** — Reads version from `VERSION` file at startup instead of hardcoded value

### 🔧 Improvements
- **Makefile compatibility** — Fixed `$(shell)` syntax for GNU Make 3.81 (macOS default)

## v0.5.4

### 🔧 Improvements
- **Update whatsmeow lib**

## v0.5.3

**Docker:** `evoapicloud/evolution-go:0.5.3`

### 🔧 Improvements

- **Update context handling in service methods** 
  - Refactored multiple service methods across various packages to include `context.Background()` as the first argument in client calls. This change ensures that all client interactions are properly context-aware, allowing for better cancellation and timeout management.
  - Updated methods in `call_service.go`, `community_service.go`, `group_service.go`, `message_service.go`, `newsletter_service.go`, `send_service.go`, `user_service.go`, and `whatsmeow.go` to enhance consistency and reliability in handling requests.
  - This adjustment improves the overall robustness of the API by ensuring that all client calls can leverage context for better control over execution flow and resource management.

## v0.5.2

**Docker:** `evoapicloud/evolution-go:0.5.2`

### 🆕 New Features
- **SetProxy Endpoint**: New endpoint `POST /instance/proxy/{instanceId}` to configure proxy for instances
  - Support for proxy with/without authentication
  - Validation of required fields (host, port)
  - Automatic cache update via reconnection
  - Integrated Swagger documentation

### 🔧 Improvements
- **CheckUser Fallback Logic**: Implemented intelligent fallback logic
  - If `formatJid=true` returns `IsInWhatsapp=false`, automatically retries with `formatJid=false`
  - Significant improvement in valid user detection
  - Added `RemoteJID` field to use WhatsApp-validated JID
- **LID/WhatsApp JID Swap**: Automatic handling of special cases
  - When `Sender` comes as `@lid` and `SenderAlt` comes as `@s.whatsapp.net`
  - Automatic inversion: `Sender` and `Chat` receive `@s.whatsapp.net`, `SenderAlt` receives `@lid`
  - Detailed logs for tracking swaps

### 🐛 Bug Fixes
- **SendMessage**: Standardization of WhatsApp-validated `remoteJID` usage
- **User Validation**: Improvement in phone number validation and formatting

---

## v0.5.1

**Docker:** `evoapicloud/evolution-go:0.5.1`

### 🔧 Improvements
- **Instance Deletion**: Enhance instance deletion and media storage path resolution
- **Media Storage**: Improvements in media storage and path resolution

---

## v0.5.0

**Docker:** `evoapicloud/evolution-go:0.5.0`

### 🔧 Improvements
- **Media Storage**: Enhance media storage and logging in Whatsmeow event handling
- **Retry Logic**: Implement retry logic for client connection and message sending
- **Media Handling**: Enhance media handling in event processing

---

## v0.4.9

**Docker:** `evoapicloud/evolution-go:0.4.9`

### 🔧 Improvements
- **Connection Handling**: Add instance update test scenarios and improve connection handling
- **FormatJid Field**: Update FormatJid field to pointer type for better handling in message structures
- **Dependencies**: Update dependencies and fix presence handling in Whatsmeow integration

---

## v0.4.8

**Docker:** `evoapicloud/evolution-go:0.4.8`

### 🔧 Improvements
- **Audio Duration**: Improve audio duration parsing in convertAudioToOpusWithDuration function

---

## v0.4.7

**Docker:** `evoapicloud/evolution-go:0.4.7`

### 🔧 Improvements
- **Phone Number Formatting**: Improve phone number formatting and validation in user service
- **Brazilian/Portuguese Numbers**: Update Brazilian and Portuguese number formatting in utils

### 🆕 New Features
- **Media Handling**: Enhance media handling in event processing

---

## v0.4.6

**Docker:** `evoapicloud/evolution-go:0.4.6`

### 🆕 New Features
- **User Existence Check**: Add user existence check configuration and JID validation middleware

---

## v0.4.5

**Docker:** `evoapicloud/evolution-go:0.4.5`

### 🔧 Improvements
- **Dependencies**: Update dependencies and enhance audio conversion functionality

---

## v0.4.4

**Docker:** `evoapicloud/evolution-go:0.4.4`

### 🆕 New Features
- **CLAUDE.md**: Add CLAUDE.md for project documentation and enhance RabbitMQ connection handling

---

## v0.4.3

**Docker:** `evoapicloud/evolution-go:0.4.3`

### 🔧 Improvements
- **PostgreSQL Connection**: Fix in PostgreSQL connection configuration for session auth
  - Controlled configuration of pool, idle, etc.
  - Adjustment on top of whatsmeow lib
- **User Endpoints**: Fix in 'User Info' and 'Check User' endpoints
  - Now return with contact's LID information

---

## v0.3.0

### 🆕 New Features
- **Own Message Reactions**: Additional 'fromMe' parameter using Chat id
- **CreatedAt Field**: CreatedAt field added to instances table

---

## v0.2.0

### 🆕 New Features
- **Advanced Settings**: Advanced configurations in instance creation
  - `alwaysOnline` (still to be implemented)
  - `rejectCall` - Automatically reject calls
  - `msgRejectCall` - Call rejection message
  - `readMessages` - Automatically mark messages as read
  - `ignoreGroups` - Ignore group messages
  - `ignoreStatus` - Ignore status messages
- **Advanced Settings Routes**: New routes for get and update of advanced settings
- **QR Code Control**: `QRCODE_MAX_COUNT` variable to control how many QR codes to generate before timeout
- **AMQP Events**: `AMQP_SPECIFIC_EVENTS` variable to individually select which events to receive in RabbitMQ

### 🔧 Improvements
- **Reconnect Endpoint**: Fix in reconnect endpoint
- **Sender Info**: `Sender` and `SenderAlt` no longer come with session id, only the id

### 🐛 Bug Fixes
- **QR Code Generation**: Fix to not generate QR code automatically after disconnection or logout

---

## v0.1.0

### 🆕 Initial Features
- Base implementation of Evolution API in Go
- WhatsApp integration via whatsmeow
- Instance system
- Basic message sending endpoints
- Webhook support
- RabbitMQ and NATS integration
- Authentication system
- Swagger documentation

---

## 📋 Migration Notes

### v0.5.2
- The new `SetProxy` endpoint requires admin permissions (`AuthAdmin`)
- The `CheckUser` fallback logic is automatic and transparent
- LID/WhatsApp JID handling is automatic

### v0.4.3
- Check PostgreSQL connection settings if using postgres auth

### v0.2.0
- Review advanced settings configurations if necessary
- Configure `QRCODE_MAX_COUNT` if you want to limit QR codes
- Configure `AMQP_SPECIFIC_EVENTS` for specific RabbitMQ events

---

## 🔗 Useful Links

- **Docker Hub**: `evoapicloud/evolution-go`
- **Documentation**: Swagger available at `/swagger/`
- **GitHub**: [Evolution API Go](https://github.com/evolution-foundation/evolution-go)

---

## 🤝 Contributing

To contribute to the project:
1. Fork the repository
2. Create a branch for your feature
3. Commit your changes
4. Open a Pull Request

---

*Last updated: October 2025*

