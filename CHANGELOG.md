# WaMux - Changelog

## Unreleased

### ⚡ Performance
- Audit writes are asynchronous and token reads are deduplicated per actor/target
  for one minute, so `/instance/all` no longer pays a DB insert per request. In a
  live 1,000-request benchmark it went from ~624 to ~1,277 req/s (baseline ~1,164).
- Per-instance log writers start with a 4 KB batch buffer and a 4,096-line queue
  instead of reserving ~450 KB each. Deleting an instance closes its writer and
  leaves a tombstone so a late background log cannot recreate it. Heap under a
  101-instance load dropped from ~76 MB to ~38 MB.
- `EncryptExistingTokens` no longer re-encrypts already-encrypted rows on every
  boot (reads hydrate the plaintext field). This removes one DB write and one log
  writer per instance at startup.
- `/instance/inventory` omits `health` for instances that never ran in this
  process, keeping the fleet response at its previous size.

### 📚 Docs
- Corrected the Evolution API comparison (Baileys, not whatsmeow), the WebSocket
  examples (instance tickets), the webhook retry description (exponential 2–60 s),
  the passkey CORS requirement (`CORS_ALLOWED_ORIGINS`) and the defaults for
  `WEBHOOK_FILES`/`MEDIA_LOCAL_STORE`.

### 🚀 Release
- **`main` publishes a pre-release; promotion is manual.** The version and
  `sha-<short>` tags are pushed without moving `latest`. **Actions → Promote
  release** retags the same digest to `latest` and `<major>.<minor>` and marks the
  GitHub Release latest. The publish job fails on an existing tag instead of
  overwriting it.

### 🔒 Security
- **Instance token can be omitted from event payloads.** `WEBHOOK_INCLUDE_TOKEN=false`
  removes `instanceToken` from webhook, RabbitMQ, NATS and WebSocket deliveries;
  `instanceId` and `instanceName` remain for routing. The default stays `true` for
  compatibility.
- **Tenant-configured endpoints are SSRF-guarded.** Webhook URLs, Typebot URLs
  and per-instance S3 endpoints now use the DNS-pinned guard. Loopback, RFC1918,
  link-local/metadata and CGNAT destinations require the explicit
  `INTEGRATIONS_ALLOW_PRIVATE=true` opt-in.
- **S3 is private by default.** The global bucket no longer receives a public
  `s3:GetObject` policy unless `S3_PUBLIC_READ=true`; media URLs are presigned.
  A per-instance `publicUrl` remains an explicit public-read signal.

### ✨ Features
- **S3 cleanup for expired media.** When message retention deletes rows, objects
  in the global MinIO/S3 bucket are removed only if no remaining message still
  references their URL. Per-instance S3 buckets are not pruned yet (they need a
  per-instance object tracker); this is called out as a remaining limitation.
- **Auxiliary-table retention.** The cleanup job can now prune
  `send_operations` (30 days by default), `chat_reads`, `chat_states`,
  `typebot_sessions` and `poll_votes` with per-table env knobs; `0` keeps a table
  forever. `evo_cleanup_deleted_total{table}` reports deleted rows.
- **Global message search.** `GET /chat/history/search` now accepts `q` without
  `chat`, searching every conversation of the instance with the same filters and
  keyset pagination. A new `(instance_id, timestamp DESC)` index backs the page.
- **Manager: Storage and Security sections.** Per-instance S3 (endpoint, bucket,
  credentials, test/remove), webhook HMAC (save, generate once, remove) and the
  connection health verdict are editable/visible from instance settings.
- **Idle runtime eviction.** `INSTANCE_IDLE_EVICT_MINUTES` (default `0`, off)
  removes a disconnected instance's in-memory client after the window, bounding
  memory for fleets with many offline instances. The DB row, JID and session are
  kept; a manual connect resumes it. A pending reconnect or in-flight pairing is
  never evicted.
- **Played status.** A `played` receipt now stores `Played`, above `Read` in the
  delivery order; late read/delivered receipts cannot move it back. The manager
  shows the played tick. History rows with WhatsApp status `PLAYED` import as
  `Played` instead of `Read`.
- **Socket watchdog.** `WATCHDOG_MODE` (`report` default, `on`, `off`) detects an
  instance whose state reads `open` but whose websocket is closed or whose
  keepalive is silent. Report mode logs; `on` rebuilds that one socket through the
  normal reconnect path, never logs out, and stops after `WATCHDOG_MAX_REBUILDS`.
  Unacked sends and a slow event queue are never triggers.
- **Per-instance health verdict.** `/instance/status`, `/instance/inventory` and
  `/metrics` expose the last event/inbound/ack, reconnect opens/drops in a
  10-minute window, conflicts, event-handler age and a verdict
  (`ok`, `reconnecting`, `looping`, `stalled`, `closed`) with reasons. Memory
  only, no DB/Redis I/O and no message content.
- **Local audit log.** Instance create/delete/rename, proxy/S3/HMAC changes,
  advanced settings and token reads are recorded in `audit_logs` with actor,
  action, target and a secret-free detail object. `GET /server/audit` (admin key)
  returns the newest entries. No external call.
- **Instance-scoped WebSocket tickets.** `POST /ws/ticket` now accepts an
  instance token and signs the instance id into the ticket. `/ws` subscribes that
  socket only to its own instance; the global key/ticket still opens any (or all)
  instance streams. Existing `?token=<global key>` integrations keep working.

### 🐛 Fixes
- **Reconnect backoff survives a 440 loop.** The counter was cleared on every
  `Connected` event; a replaced session opens briefly each attempt, so the
  budget reset forever. It is now cleared only after 60 s of continuous
  connection.
- **Non-retryable closes stop the loop.** Logged out, banned, primary-device-gone
  and replaced sessions persist a clear disconnect reason and suppress automatic
  reconnect; the session is kept for a manual connect. `RECONNECT_CONFLICT_LIMIT`
  (default 10, `0` = unlimited) caps consecutive 440s.
- **One bad row no longer drops a history batch.** A failed chunk is retried and
  then written row by row; the log reports inserted/skipped/duplicate/failed and
  why rows were skipped.
- **History depth is configurable.** `HISTORY_SYNC_DAYS`, `HISTORY_SYNC_MAX_MB`
  and `HISTORY_STORAGE_QUOTA_MB` replace the fixed 3650 d / 2 GB values
  (defaults: 3650 d / 10 GB / 10 GB).
- **Weak API keys are flagged.** A published example or a key shorter than 32
  characters logs a warning; the generated-key path is unchanged.

## 1.0.1 — albums, idempotent sends, and a clearer manager

Upgrade notes are in `.github/release-notes/1.0.1.md`.

### ✨ Manager
- Instance settings has a section list (connection, webhooks, events, proxy,
  behavior, danger). Webhook, events, proxy and behavior show when they have
  unsaved changes, and each save button stays on its own form.
- The inbox empty state links to Instances when nothing is connected. Storage
  warnings explain the limitation in plain language, with the server setting
  behind “how to turn this on”. A chat shows its name, with the number
  underneath, instead of a raw JID.
- An instance card opens that instance’s messages, its actions stay visible,
  and the list no longer stops at 24. Counts follow the selected language.
- `/messages` shows unread messages like WhatsApp: a count on each conversation, a
  divider at the first unread message, opening there, and a count on the jump-to-latest
  button while scrolled up. A chat counts as read when you reach its newest message, or
  when it is read on the phone. Older messages and recovered history start as read.

### 🔌 API
- `GET /chat/chats` adds `unread_count` and `last_read_at`, and `POST /chat/read` moves
  that position. It only tracks reading in the history: nothing is sent to WhatsApp, so
  no blue ticks (migration `0011`).
- **Albums** — `POST /send/album` sends 2–30 images/videos as one WhatsApp album, with
  per-item captions and the reply/mentions on the first item. Incoming album containers
  are classified as `album` and no longer stored as blank history rows.
- **Keyset history pagination** — `/chat/history` and `/chat/history/search` accept
  `keyset=true` plus the `before`/`before_id` cursor returned in `next_cursor`, so pages
  are exact even when many messages share one second. `GET /chat/history/capabilities`
  publishes the contract. The old `before`-only cursor still works.
- **Idempotent sends** — JSON `POST /send/*` except `/send/album` accepts an
  `Idempotency-Key`. A retry with the same body replays the first response without
  sending again; the same key with a different body is `409`. `GET /send/operations` and
  `GET /send/operations/:id` report the state (migration `0012`).
- **`/metrics` auth** — `METRICS_AUTH=admin` (default), `loopback` or `none`. The default
  now requires the global API key because the series include instance ids and queue depths.
- **Avatar cache** — `/user/avatar` caches a picture for 10 minutes and a missing picture
  for one minute per instance+JID+preview, so scrolling the manager no longer queries
  WhatsApp per avatar.
- **NATS JetStream** — `NATS_JETSTREAM=true` with `NATS_STREAM` publishes events through
  an existing stream, waits for the ack and uses a payload hash as the message id so a
  retry inside the duplicate window is not stored twice. A missing stream falls back to
  core NATS.
- **History sync request** — `POST /chat/history-sync` accepts `chat` + `timestamp` as
  well as `messageInfo`, clamps `count` to 100 and verifies group membership. An empty
  body is now `400` instead of a panic.
- **History pages** add `has_more` and `contract_version`; `next_cursor` is returned only
  when another page exists.
- **Idempotent sends** reserve a 32-character native WhatsApp id before transport
  (`planned_message_id`, migration `0013`) and reconcile a pending operation against the
  stored message when status is read.
- **Albums** store `album_id`/`album_index` (migration `0014`) and the manager renders one
  album container in item order.
- **Fleet inventory** — `GET /instance/inventory` returns connection state and a SHA-256
  owner fingerprint without tokens or raw JIDs.
- **`/metrics`** now exposes RSS, peak/virtual memory, heap, sys, goroutines, cgroup
  memory/CPU quota, GC count/pause, CPU seconds and uptime.
- **Chat state** — archived, pinned, muted and marked-unread state from WhatsApp events
  and history sync is stored (migration `0015`) and shown in the chat list.
- **LID mapping** — `POST /chat/lidMapping` batch-resolves up to 200 LIDs and phone
  numbers in both directions, returning `null` for unknown entries.
- **History filters** — `/chat/history` and `/chat/history/search` accept `after`,
  `until`, `type` and `from_me` in addition to the cursor.
- **Replay signing** — `wamux-replay-webhook` reads and decrypts the per-instance HMAC
  key from `instances.hmac_key`, so replayed events satisfy signature-checking
  consumers; `-hmac-key` remains a global override.
- **Avatar limits** — `AVATAR_RATE_LIMIT_PER_MINUTE` and `AVATAR_RATE_LIMIT_BURST`
  replace the hardcoded 30/min burst-5 (`0` disables).

### 🐛 Fixes
- **Button, carousel and product images** keep their real type. A PNG stays
  `image/png`, a still WebP is re-encoded as JPEG, and a video is sent only when
  the bytes are MP4. A header or card whose URL fails is not sent.
- **Albums** revoke the container and the items already accepted when a later
  item fails, and the API returns only the error.
- **Instance saves** no longer write the decrypted token back into the plaintext
  column. Encrypting a token also clears that column.
- **Idempotency-Key** covers every JSON send except albums. A retry of a location,
  poll, sticker, contact, interactive message or status was sent again.
- **Albums** fetch every item before sending the container. A URL item was
  downloaded from an empty address, and the API returned an error after earlier
  items had already been delivered.
- **WebP images** are re-encoded as JPEG before upload. The file was sent with a
  JPEG type and the original WebP bytes.
- **Chat list** preview skips a revoked message when the conversation still has
  an earlier visible one, instead of showing an empty tombstone.
- **`QRCODE_MAX_COUNT`** stops the instance. It restarted instead, with the count back at
  zero, so an unscanned instance rotated codes forever. `/instance/connect` starts again.
  The count now also survives the restart between batches of codes, or a limit above
  one batch was never reached.
- **Logout** finishes even if saving the state fails. WhatsApp has already ended the
  session by then, so it no longer answers `500` and leaves the client behind.
- **Message writes** are retried, since WhatsApp will not deliver an acked message again.
- **Inbound edits only apply from the original author.** The manager compares sender and
  direction before folding an edit into its target, and ingest rejects an edit whose
  stored target is missing/deleted, in another chat, or from another sender. Rejected
  edits are not stored and the webhook carries a `rejected` reason.
- **Caller-supplied media/link URLs always block private destinations.** DNS pinning and
  redirect revalidation still apply; `SSRF_PROTECTION` only remains as the explicit
  escape hatch for configured internal services.
- **Idempotency fingerprints are canonical JSON**, so a retry with the same fields in a
  different order replays instead of returning `409`.
- **Edit/delete immediately after a send** now writes the local tombstone/edit row even
  when the async outbound insert has not run yet. Edits reject inbound, deleted or
  older-than-15-minute targets with `WHATSAPP_EDIT_RESTRICTED`/`WHATSAPP_EDIT_TIME_EXPIRED`.
- **Inbound media is verified** against `FileLength` and `FileSHA256` before it reaches
  local/S3 storage; mismatches set `mediaError` and are not served.
- **Avatar queries** are cached for 10 minutes, misses for 15, invalidated by
  `Picture`/`Contact` events and limited to a 30/min burst-5 bucket per instance.
- **NATS JetStream** compares the stream's max message size with the expected event size
  at startup and logs the exact `--max-msg-size` to use; oversized events fall back to
  core NATS with an explicit warning instead of silently losing durability.
- **Shutdown flushes accepted events.** Webhooks wait for their backlog (bounded by the
  shutdown deadline) and NATS drains its publish buffer before the process exits, so a
  deploy no longer drops work that `Produce` had already accepted.
- **Album containers** no longer appear as blank `unknown` messages in history or the
  manager; the items are stored normally.
- **`POST /chat/history-sync`** no longer panics on `{}` and no longer asks for history
  from a group the account is not in.
- **Offline instances** read LID and phone chats as one, and rows stored under a LID move
  to the phone number at boot once it is known.
- Merged LID and phone chats add up their unread counts.
- A file upload accepts `ptv`, as URLs always did.
- Two nodes starting together could fail installing the message counters, and a Typebot
  cache could refresh twice. Both came up once tests ran in random order.

### 🧪 Project
- Tests run shuffled in CI, CodeQL and dependency review run on the code, and the image
  smoke test checks every migration was applied.
- Media uploads and URLs share one delivery path (`send_media.go`), and the repeated event
  JSON handling is one helper.

### ✨ Deployment
- whatsmeow moved to `19d61352436f` (16 November 2026), with libsignal 0.2.3.
  Block and unblock pass an empty blocklist hash so the API still returns the
  updated list. Message webhooks gain `StanzaTag` and still include
  `IsNewsletterStatus`.
- Image bases moved to `alpine:3.24.4`, `golang:1.27.4-alpine` and
  `oven/bun:1.4.4-alpine`. The Go module version matches the image.
- **A new install needs no configuration.** `docker compose up -d` works with no `.env`.
  `GLOBAL_API_KEY` is generated on first boot into `global-api-key` on the data volume,
  and is never logged. `DATABASE_SAVE_MESSAGES` now defaults to `true`, `CLIENT_NAME` to
  `wamux` and `OS_NAME` to `WaMux`.
- **`TRUSTED_PROXIES` defaults to private addresses**, so a reverse proxy on the same
  host, Docker network or LAN needs no setup. Public clients still cannot set their own
  address. `TRUSTED_PROXIES=none` restores the strict behaviour.
- **History sync stays within retention.** Messages `MESSAGE_RETENTION_DAYS` old or more
  and status posts are skipped, so a sync no longer writes rows the cleanup would delete.
- A missing MinIO/S3 bucket is created on start. It used to be left to the operator, and
  every upload failed until then.
- The image has a `HEALTHCHECK`.
- The image ships `wamux-replay-webhook`, a dry-run-first CLI for replaying the RabbitMQ
  webhook dead-letter queue with a plan hash and `-apply -confirm`.
- The compose examples default to `latest`, not `dev`. `docker-compose.yml` replaces
  `docker-compose.ghcr.yml`, reads an optional `.env` and uses port 8081, as do the
  others. `init-db.sql` is gone: databases are created on boot.
- `docker-compose.full.yml` no longer sends events to a sample `WEBHOOK_URL`, its admin
  consoles only listen on localhost, its passwords can be set in `.env`, and it boots:
  `minio/minio` is replaced.

## 1.0.0 — complete message history, large files, a React dashboard

Upgrade notes are in `.github/release-notes/1.0.0.md`.

### ⚡ Performance
- **Large media streams to disk** — files above `WEBHOOK_BASE64_MAX_BYTES` (5 MiB)
  are downloaded straight to a file and handed to the local store and S3/MinIO, so
  memory stays flat whatever the size. A file nothing would keep is not downloaded.
- **`ListChats`** — a skip scan replaces reading every message: 1.6 s to 42 ms on an
  instance with 1M messages. Results are identical.
- **Inserts** — GORM's implicit transaction is skipped for single-statement writes
  (1.15 ms to 0.50 ms per row), and two redundant indexes are gone (migration `0009`).
- **Request log** — `HTTP_ACCESS_LOG` defaults to `errors`.
- **Auto-read** — one call off the dispatch path, instead of two with one blocking.

### 🔌 API
- Pairing QR codes are drawn in the instance log and on the terminal, so an instance
  can be paired from `docker logs`. `QRCODE_TERMINAL=false` turns this off.
- `POST /chat/markunread` shows a chat as unread on the account's devices.
- `GET /chat/media/:messageId/preview` gives a JPEG of a stored PDF's first page.
- History adds `file_name`, `file_size`, `is_forwarded`, `page_count` and
  `file_thumbnail`, for new messages.
- `POST /chat/media/:messageId/link` gives a 5-minute link that downloads one stored
  file without an API key.
- `/send/media` accepts files up to `MAX_UPLOAD_BYTES` (2 GiB by default), by upload or
  by URL, up from 64 MiB. Files over 64 MiB stream through disk with flat memory and
  must be a document or an MP4 video. Refused files now answer `413` with the reason,
  not a `500` saying "cannot read file".
- `POST /instance/create` accepts `websocketEnable`. The manager turns it on.
- `GET /chat/history/search` finds text in one conversation, ignoring case.
- `POST /ws/ticket` issues a short-lived credential for `/ws`.

### 📚 Docs
- The compatibility table now shows real results per client. PIX failed everywhere,
  lists were missing on Desktop and carousels were missing on the phone app, even
  with a `200` and a `Read` receipt.
- The channel guide no longer claims documents and polls work there.

### ✨ Manager
- Loading a page keeps the sidebar and header on screen and shows a skeleton of that
  page, instead of replacing the whole app with a generic grid.
- Mensagens por dia shows the exact count on each column.
- Downloaded photos, videos and audio get a real name and extension, such as
  `WaMux Image 2026-11-06 at 00.06.12.jpg`, instead of the bare message id.
- The media viewer fills the screen, with zoom by wheel, pinch, double click or keyboard,
  panning, rotation, and the file's name, size, dimensions and date.
- Documents show as larger cards like WhatsApp's, with a colored type badge, the page
  count and a preview. PDFs get their first page, rendered the first time it is viewed.
- Files in `/messages` show their name, type and size with a download button, like
  WhatsApp, and messages are marked Forwarded. Downloads go straight to disk, and a
  document or a large video is no longer loaded into the page to draw the chat.
- Dates, numbers, form errors and connection errors follow the chosen language.
- The sidebar collapses to icons, with titles on hover.
- The divider between conversations and chat on `/messages` can be dragged, or moved
  with the arrow keys. A double click resets it.
- Edited messages show the new text and an "Edited" label.
- Message bubbles have a small gap, and the jump-to-latest button no longer covers
  the media preview's close button.
- The dashboard is now part of the manager, in React, instead of a separate page in
  an iframe: translated, themed, and complete on phones. It no longer loads Chart.js
  from a CDN or keeps its own copy of the global API key, and copies left in the
  browser are removed. `/dashboard` redirects to `/manager`.
- Dashboard logs sit with the selected instance, as colored rows instead of raw
  JSON. Filter by level (on the server, so older errors are found) and by text,
  choose how many lines to load, or follow them live.
- `/manager` sends `Cache-Control: no-cache`, so a deploy shows up on the next load.
- New Events page: a live feed from every instance, with filters by instance, type
  and text, pause, and the JSON of each event. Tokens and keys are hidden, long
  content is cut, and instances with WebSocket off are listed with a link to fix it.
- The unfinished Settings page is gone. Its options live in the header and in each
  instance's settings.
- Deleting an instance, disconnecting it or removing its proxy asks in a themed,
  translated dialog instead of the browser's `confirm`, or not asking at all.
- `/messages` can search the open conversation on the server, including messages
  not loaded yet, and jump to a result.

### 🔒 Security
- The global API key no longer reaches the access log through `/ws?token=`.
- The manager opens `/ws` with a signed ticket from `POST /ws/ticket`, valid for 30
  seconds, so the global API key stays out of URLs and proxy logs. `?token=` still
  works for existing integrations.

### 🐛 Fixes
- **Revoked messages** — a message deleted for everyone no longer stays readable. Its
  row becomes a `deleted` tombstone with no text, media details or preview, its local
  file is removed and any edits go with it. An object store copy is not removed. A
  revoke only counts inside the chat it arrived in, and in a direct chat only for the
  sender's own messages. `delete` rows now carry `quoted_message_id`.
- **Retry receipts** — a revoked message can no longer be sent again in reply to a
  recipient's retry receipt.
- **History sync** — batches are now stored, so **Recuperar** can show what the phone
  sends back. Stored rows are never changed by a sync, media is not downloaded, and
  `DATABASE_SAVE_HISTORY_SYNC=false` turns this off.
- **Repeated messages** — a message delivered twice fires its webhook and Typebot once.
- **Logs** — vCards and whole call, presence, label, app-state and undecryptable
  message events no longer reach the logs or stdout.
- **Docs** — `WADEBUG` and `LOGTYPE` were documented, but the server reads
  `DEBUG_ENABLED` and `LOG_TYPE`.
- **Mexican and Argentine numbers** — `formatJid` no longer rewrites them. Mexican
  13-digit numbers lost two digits, one of them from the subscriber number, and
  Argentine `549...` mobiles lost the 9, so sends could fail as "not registered".
- Dashboard totals counted receipts for messages that were never stored, such as
  status posts. Those rows are no longer counted and are removed after a day, and migration `0010` clears the existing ones. The receipt
  still reaches its message when it arrives first.
- `mentionAll` and `mentionedJid` tagged nobody unless the text already had `@number`.
  The API now adds the missing tokens to text and captions, and `mentionAll` leaves
  out the sender.
- `GET /instance/logs` stopped at the end of the UTC day in timezones west of UTC, so
  in `-03` nothing logged after 21:00 showed until local midnight.
- A poll sent to a channel fails with a clear error instead of retrying into a `479`,
  and the document error no longer suggests polls.
- Polls, events and products are stored with their question or title, instead of as
  empty or `unknown` messages.
- `formatJid: false` was ignored: every send reformatted the number anyway. Numbers
  now go out as given.
- Carousels never honored `formatJid`, and uploaded the error page as the card's
  media when an image or video failed to download.
- Link previews use `og:title`, and an inline SVG's title no longer replaces the
  page title.
- `/messages` kept polling off whenever the socket was open, so an instance with
  WebSocket disabled never refreshed until the page was reloaded.
- Routine messages no longer log as `WARN`: presence updates, skipped webhooks, QR
  timeouts, new devices and subscription changes. Failed logouts of an unpaired
  QR session are no longer reported at all.
- **`DEBUG_ENABLED=0` no longer enables debug logging.** whatsmeow treats unknown
  levels as DEBUG, so it logged every protocol node.
- **Stored media is pruned** — files were never deleted, so retention only applied
  to rows. New `MEDIA_RETENTION_DAYS`, and deleting an instance removes its media.
- `GET /chat/media/:messageId` now serves files up to 2 GiB, not 64 MiB.
- **Label colors** — `label_color` and `predefined_id` were saved as memory
  addresses. Existing rows correct themselves on the label's next edit.
- **`ButtonClick`** — `phone`, `jid`, `pushName`, `messageId`, `chat` and `fromMe`
  were always null. `phone` stays empty when only a LID is known.
- **`LoggedOut`** was delivered twice, to webhooks and to global queues.
- **Delivered receipts** now update every message they cover, not only the first.
- **History pagination lost messages** — the `before` cursor is exclusive with
  one-second precision, so a page ending partway through a second dropped the rest
  of it. Pages now end on a whole second, and may run slightly over `limit`.
- **Message status only moves forward** — a late Delivered no longer overwrites
  Read, and a send recorded after its receipt no longer resets it to Sent.
- **Batches with a repeated message id** no longer fall back to row-by-row inserts.
- **`WEBHOOK_URL`** now receives events from instances with no webhook of their
  own, as documented. An instance set to `disabled` opts out.
- **RabbitMQ and NATS `"true"`** published nothing, though the instance looked
  enabled.
- **RabbitMQ** waits for the broker's confirm, so a lost message is now an error,
  and reuses channels: 9.7 ms to 5.8 ms per message. Reconnects no longer race.
- **Captioned documents** we send are stored as `document` with their caption.
- **`ffmpeg`, `ffprobe` and `pdftoppm`** are killed after a time limit instead of
  being able to hang a send forever.
- **Profile pictures** are capped at 16 MiB, and an error page is no longer uploaded.
- **Undecryptable IDs starting 66/67** reconnect through the backoff, off the event
  path, instead of inline once per message.
- **A `null` JSON body** returns 400 on every endpoint instead of panicking.
- **Message edits arrived as an unreadable blob** — they were decrypted after the
  LID was swapped for the phone number, which changes the key. They now reach
  webhooks flagged `IsEdit` with the new text, and are stored as `edit`.
- **Buttons** are stored as `interactive` and `buttons_response`, not `unknown`.
- **Captioned documents we send** keep their mimetype.
- **Edits made through `POST /message/edit`** are now stored, so history shows them.

### ⚠️ Behaviour changes
- With `DATABASE_SAVE_MESSAGES=true`, the history sent after pairing is now stored.
  This can add many rows to `messages`; set `DATABASE_SAVE_HISTORY_SYNC=false` to keep
  the old behaviour. The webhook is unchanged.
- Events for files above the limit carry `mediaUrl` and `base64Omitted: true`
  instead of `base64`. `mediaUrl` is an API path needing the instance token, unless
  an object store is configured. Outbound media follows the same limit.
- Auto-read no longer marks messages that are ignored (groups, status).
- An instance with an empty `events` value now receives `MESSAGE` and
  `SEND_MESSAGE`. That default never applied, so it received nothing.
- `GET /chat/history` can return slightly more than `limit` rows.
- With `WEBHOOK_URL` set, instances without a webhook start sending to it.
- Instances with `rabbitmqEnable` or `natsEnable` set to `"true"` start publishing.

## 1.0.0 — security, teardown and performance

### 🔒 Security
- **Tenant isolation** — `GET`/`PUT /instance/:instanceId/advanced-settings` now
  return `403` unless the id is the token's own instance. Any instance token
  could previously read or change another instance's settings.
- **Credential guessing** — failed authentication is budgeted per IP (30 per
  window). The limiter keys on the presented credential, so rotating keys used
  to bypass it entirely. Credentials already active in the window are not
  affected by a neighbour's failures.
- **`X-Forwarded-For`** — no longer trusted by default. **Behind a reverse proxy,
  set the new `TRUSTED_PROXIES`**, or every client shares the proxy's IP for rate
  limiting. An invalid list stops the boot.

### 🐛 Fixes
- **Message history across instances** — uniqueness is now per instance
  (migration `0008`). Two instances sharing a conversation overwrote each other's
  row, because WhatsApp message ids are only unique per account.
- **Rollup deadlocks** — batches now take `message_counters` rows in one order;
  concurrent batches used to deadlock under load.
- **Client teardown** — a new one-shot `killswitch` replaces the kill channels.
  Reconnects no longer leak a loop and its client, deletes are always noticed,
  and an `alwaysOnline` presence goroutine can no longer swallow the signal.
- **`POST /instance/disconnect` and `DELETE /instance/logout`** now leave the
  client down; the teardown loop used to start it again.

### ⚡ Performance
- **Media no longer blocks an instance** — messages that need a download run on a
  bounded pool instead of whatsmeow's one-at-a-time dispatch, where a slow file
  held up every later message and receipt. They can now finish after later events.
- **Slow consumers only affect themselves** — WebSocket subscribers get a bounded
  queue, write deadlines and ping/pong, and are disconnected if they fall behind.
  Webhooks are budgeted per endpoint, so one dead URL can no longer drop other
  tenants' events; events refused by a full backlog go to the dead-letter queue.
- **`encoding/json` is the default build** — `go_json` compiled an encoder for
  WhatsApp's protobuf types on first use, stalling the first message after every
  boot for ~3 s and holding 175 MB of heap, to save ~2 µs per event. Set
  `GO_JSON_TAG=go_json` to opt back in.
- **Large media is limited to 2 at a time** — files of 8 MiB or more peak at about
  20x their size in memory, so they run on their own small pool.
- **Rate limiter** — expired windows are swept once per window rather than on each
  new key, which let junk keys slow every request.

## 0.11.1 — messages workspace, auto-reconnect, security

A focused follow-up to 0.11.0: a rebuilt Messages workspace, safer boot-time
reconnect behaviour, dashboard identity resolution, and a clean dependency/CI
baseline. No breaking API or data changes.

### 🆕 Manager
- **Messages workspace** — chats, history, contacts, sender names and server
  flags now use TanStack Query, with infinite history pagination, optimistic
  sends, WebSocket-driven invalidation and polling fallback when real time is
  unavailable.
- **Richer thread** — day separators, quoted-message previews, sender grouping,
  linkified text, unsupported-message placeholders and a "jump to latest"
  action.
- **New conversation flow** — searchable contacts/groups, recent chats and
  phone-number entry in a dedicated dialog. Recover-history moved to the thread
  header.
- **Media** — image/video lightbox with navigation and download, document
  cards, upload progress, drag/drop and clipboard-paste attachments.
- **Mobile polish** — responsive header, bottom-sheet new-chat dialog, safe-area
  composer and improved mobile thread navigation.
- **Message tests** — the interactive button/list/carousel tester moved from the
  instance cards to the Messages header, leaving card footers less crowded.

### 🔧 API / reliability
- **Boot-time reconnect** — paired instances reconnect after a restart by
  default. A new `auto_reconnect` intent survives restarts, while an explicit
  API disconnect or logout keeps the instance down (migration 0007).
- **Dashboard identities** — “Conversas mais ativas” resolves LIDs to phone
  numbers and saved contact names even when the owning instance is offline.
  Also fixes a nil contact-store panic in `/server/stats`.

### 🔐 Dependencies / CI
- `golang.org/x/net` 0.62.0 and the patched `golang.org/x/crypto`; manager
  `axios`, `zod`, `vite` and transitive overrides updated. `govulncheck` and
  `bun audit` are clean.
- Workflows pinned to `ubuntu-24.04`; `setup-go` v7 and `gitleaks-action` v3.
- Fixed a race-job flake caused by async test loggers writing during temp-dir
  cleanup.

### 🐳 Container image
`docker pull ghcr.io/felipeestevanatto/wamux:0.11.1`

## 0.11.0 — manager connection flow, API hardening

A smoother instance connection flow in the manager, plus API and database fixes
for paired instances, instance creation and unknown routes. No breaking API or
data changes.

### 🆕 Features
- **One-click connect** — the instance card now connects with the saved
  settings. The QR dialog opens immediately with inline progress instead of
  showing a toast first.
- **Faster scan detection** — the QR dialog checks the instance every ~2s,
  updates as soon as WhatsApp confirms the scan, and offers a manual
  "I've scanned it" action.
- **Connection settings in Instance Settings** — removed the duplicate connect
  configuration modal. Webhook, events, event producers, advanced options and
  proxy remain in Instance Settings; a new Connection card adds phone pairing
  with a copyable pairing code and automatic connection detection.
- **Already-paired instances** — the manager now shows a "reconnecting existing
  session" state instead of an empty QR/error when the instance already has a
  stored WhatsApp session.

### 🔧 Fixes
- **Multiple encrypted instance tokens** — replaced the `UNIQUE(token)`
  constraint with a partial unique index covering only non-empty legacy tokens
  (migration 0006). Creating a second encrypted instance no longer fails with a
  duplicate-key error.
- **QR for paired instances** — `GET /instance/qr` returns
  `{paired:true, connected:...}` instead of a phantom 400. It resumes the stored
  session once and never replaces a client that is still connecting.
- **Fail fast on unpaired instances** — chat, group, user, label, newsletter and
  instance endpoints now return structured errors instead of proceeding with an
  unpaired instance; invalid instance IDs are rejected early.
- **Unknown API routes** — return a NestJS-style JSON 404 instead of the HTML
  default.
- **Manager card footer** — pinned to the bottom of the card, regardless of how
  much instance detail is shown.

### 🧪 Testing
- Added coverage for paired-instance guards, QR behavior, migration 0006 and
  unknown-route responses.
- Pinned jsdom to 30.0.0 for Bun compatibility.

### 🐳 Container image
`docker pull ghcr.io/felipeestevanatto/wamux:0.11.0`

## 0.10.0 — manager overhaul, HTTP compression, performance

Manager UX overhaul, response compression, two measured performance
optimizations, a whatsmeow update, and a new benchmark suite. No breaking API or
data changes.

### 🆕 Features
- **Manager overhaul** — full i18n across every page and component, accessible
  dialogs, a command palette, a connection-status banner and a no-flash theme.
- **Manager data layer** — instances moved to TanStack Query with optimistic
  delete/disconnect, per-page error boundaries, route prefetching, and the
  bundle decoupled from `@evoapi/design-system` and split for faster loads.
- **Messages screen** — conversation tabs (contacts / groups / channels),
  avatars, group sender names, live receipt updates and "load older"
  pagination.
- **HTTP compression** — gzip responses for clients that accept it, on by
  default; set `HTTP_COMPRESSION=false` when a proxy already compresses. Boolean
  env parsing is now consistent across the app.

### ⚡ Performance
- **Identity cache** (`pkg/whatsmeow/identitycache`) — memoizes whatsmeow's
  per-device `IsTrustedIdentity`, which is one SELECT per device inside the
  group-send loop and is not batched. Against Postgres, 600-device lookups drop
  from ~0.6–0.9 s to ~0.2 ms (~2000–3000x).
- **Go heap tuning** (`pkg/gctune`) — derives a soft memory limit from the
  container's cgroup limit (or `GO_MEMORY_LIMIT_MB`), optional `GOGC_PERCENT`,
  and a periodic idle-heap reclaimer (`GO_MEMORY_RECLAIM_*`).

### 🔧 Fixes
- `gin.Static` no longer blocks on its pre-written 404 when compression is
  enabled.
- Memory benchmark made robust on busy runners (peak-relative assertion).

### 📦 Dependencies
- `go.mau.fi/whatsmeow` updated to 2026-10-05: `GetBlocklist` gained an optional
  `dhash` cache hash, and a new WASA-root-secret migration is applied
  automatically on startup.

### 🧪 Testing
- New `benchmarks/` suite with validation tests: group-send crypto, the HTTP
  request pipeline, webhook signing, persistence, idle memory, the identity
  cache, and apime/wa-rs comparisons.

### 🐳 Container image
`docker pull ghcr.io/felipeestevanatto/wamux:0.10.0`

## 0.9.2 — docs, security and cleanup

- Security: removed a leaked credential (a `GLOBAL_API_KEY` and Postgres
  password) and rewrote git history to purge it; `.env.example` now uses a
  placeholder; gitleaks allowlists doc/env placeholders. Rotate those values if
  used.
- Docs: fixed broken links/ports, added `SECURITY.md`, rewrote `README.md`,
  moved pre-fork history to `docs/changelog-upstream.md` and full engineering
  history to `docs/archive/fork-history.md` (`FORK_NOTES.md` is now a summary).
- No API or data changes.

## 0.9.1 — security and CI fixes

- Go toolchain bumped to 1.26.8 (`go.mod` + `golang:1.26.8-alpine`), clearing 26
  standard-library advisories reported by `govulncheck`.
- `github.com/quic-go/quic-go` bumped to v0.59.1 (GO-2026-5676, HTTP/3 QPACK).
- gitleaks: `jsonx` benchmark uses an all-zero placeholder token; added
  `.gitleaks.toml` allowlisting that fixture. No API or data changes.

## 0.9.0 — renamed to WaMux

Rebrand from the `evo-gofork` community fork to **WaMux** (independent project,
originally forked from Evolution Go).

- Product name, manager UI and documentation rebranded to WaMux.
- Go module `github.com/evolution-foundation/evolution-go` →
  `github.com/felipeestevanatto/wamux`.
- Image `ghcr.io/felipeestevanatto/evo-gofork` →
  `ghcr.io/felipeestevanatto/wamux`.
- Env prefix `EVOGO_` → `WAMUX_`; databases `evogo_*` → `wamux_*`; volumes and
  containers `evolution_go_*` → `wamux_*`.
- Attribution to Evolution Go / Evolution Foundation retained in `LICENSE`,
  `NOTICE` and `TRADEMARKS.md` (Apache License 2.0).

## Unreleased

Three additive features, all disabled/unchanged by default unless configured.

### 🆕 New Features
- **Webhook HMAC signing** — HTTP webhook deliveries can carry an
  `x-hmac-signature` header (hex HMAC-SHA256 of the exact body). Configure a
  per-instance key with `POST /instance/hmac` (`GET`/`DELETE` to inspect/clear)
  or a global `WEBHOOK_HMAC_KEY`; per-instance wins. Keys are encrypted at rest
  (AES-256-GCM) with `WEBHOOK_HMAC_ENCRYPTION_KEY` (falls back to
  `GLOBAL_ENCRYPTION_KEY`, then a key derived from `GLOBAL_API_KEY`). No key
  configured means deliveries are byte-for-byte unchanged.
- **Message history readback** — message rows now persist content
  (`chat_jid`, `sender_jid`, `message_type`, `text_content`, `media_url`,
  `media_mimetype`, `quoted_message_id`, `is_from_me`), exposed via
  `GET /chat/history?chat=…&limit=&before=` (keyset-paginated) and
  `GET /chat/chats?limit=`. Status-only receipts no longer overwrite content or
  reorder a message. Requires `DATABASE_SAVE_MESSAGES=true`.
- **Group join requests** — `POST /group/requestparticipants` (list pending
  requests) and `POST /group/updaterequestparticipants` (`approve`/`reject`),
  wiring service methods that already existed but were never exposed.
- **Per-instance S3 storage** — `POST/GET/DELETE /instance/s3` and
  `POST /instance/s3/test` let one instance use its own S3-compatible bucket
  (overriding the global MinIO config) with a `mediaDelivery` mode of `base64`,
  `s3` or `both`. The secret is encrypted at rest; the config is cached and
  rebuilt on change.
- **Messages screen (manager)** — the manager's `/manager/messages` page lists an
  instance's conversations (`GET /chat/chats`, split into Contatos / Grupos /
  Canais), reads a thread back (`GET /chat/history`) with "load older"
  pagination, sends text and media (`POST /send/text`, multipart
  `POST /send/media` for photo/video/audio/document), can start a new
  conversation from a typed number, applies inbound/receipt updates live over
  `/ws` (with a connection indicator), and has a refresh button with feedback.
- **Messages screen polish** — the conversation list now groups contacts/groups/
  channels behind **tabs with counts** (All / Contacts / Groups / Channels)
  instead of stacked sections, and each row shows a rounded avatar (profile
  picture for contacts, a glyph for groups), the phone number, and the message
  count. The filter field gains a clear (✕) button. Group and channel threads
  now label **who sent each message** (coloured by sender), and — the reported
  bug — a group's real subject is shown instead of `Grupo <id>`: the stored chat
  JID is resolved to the group name server-side (new `GET /chat/contacts` and
  `GET /chat/senders`; `GET /chat/chats` rows now carry a `name`). Contact
  avatars are fetched **lazily** via a shared `IntersectionObserver`, so opening
  a list of 50 conversations no longer fires 50 avatar requests at once — only
  the rows about to be seen load their picture.
- **Local attachment previews (opt-in)** — when `MEDIA_LOCAL_STORE=true`,
  incoming and outgoing media is also stored on the data volume
  (`<dataDir>/media/<instanceId>/<messageId>`) and served by
  `GET /chat/media/:messageId` (instance-authenticated). This makes the manager
  render real image/video/audio previews and document links in the thread even
  without MinIO/S3; the manager fetches the path as a blob and uses an object
  URL. When an object store is configured its URL is preferred. Controlled by
  `MEDIA_LOCAL_STORE` (**default `false`**, i.e. the previous behaviour: media is
  only forwarded to the webhook and never written to disk). Inbound copies also
  require `WEBHOOK_FILES=true`; outbound copies require
  `DATABASE_SAVE_MESSAGES=true`. Only messages stored while enabled have bytes
  available.
- **Feature-flag reporting** — `GET /server/stats` now exposes
  `storage.historyEnabled`, `storage.mediaLocal` and `storage.webhookFiles`. The
  manager reads them and shows an inline note when attachments are not being
  stored (so a file showing only `[image]`/`[document]` is explained) and when
  history readback is off.
- **On-demand recovery (manager)** — the Messages screen can list the instance's
  contacts (`GET /user/contacts`) to start a chat, and a **Recuperar** button
  asks the phone for older messages of the open chat (`POST /chat/history-sync`
  with the newest known message as the cursor). Inbound `HistorySync` batches were
  forwarded to the webhook but not persisted at this point.

### 🔒 Security
- **Instance tokens encrypted at rest** (`pkg/tokencrypt`) — tokens are stored
  as a deterministic HMAC-SHA256 (`token_hash`, for auth lookup) plus an
  AES-256-GCM ciphertext (`token_enc`), never plaintext. Existing rows are
  backfilled once at startup; the plaintext column is cleared. The auth lookup
  is by hash, so the credential is not compared byte-by-byte.
- **Container runs as a non-root user** (UID 10001) with `cap_drop: ALL` and
  `no-new-privileges` in the compose.
- **`/debug/pprof` requires the admin key** (and stays off unless
  `PPROF_ENABLED=true`).
- **CI security workflow** (`.github/workflows/security.yml`): `govulncheck`,
  `go vet` + `go test -race`, and a gitleaks secret scan, on push/PR/weekly.

### 🛡️ Resource safety
- **Rate-limiter no longer locks the operator out** — health/observability
  routes (`/`, `/server/ok`, `/server/health`, `/metrics`, `/favicon.ico`,
  `/license/*`) are exempt from the limiter, so an orchestrator/scrape burst
  never makes the service look down; and the admin/global key gets its **own,
  more generous bucket** (10x, min 600/min) separate from per-instance/per-IP
  traffic, so a dense dashboard/load burst cannot 429 the operator. Found by a
  load test that tripped the shared bucket.
- **Stored media serving is capped** at 64 MiB per request (`413` above it),
  with the read bounded by a `SectionReader`.
- **Cluster-aware DB pool budget** (`DB_POOL_BUDGET_PER_NODE`, `DB_NODE_COUNT`):
  the three per-node pools (auth, users, key store) are divided out of a shared
  budget so N nodes do not exhaust Postgres `max_connections` (default keeps the
  previous 25/5).

### ⚡ Performance
- **Docker build cache + leaner binary** — the build stage now uses BuildKit
  cache mounts for `/go/pkg/mod` and `/root/.cache/go-build`, so a source change
  no longer lands on an empty `GOCACHE` and recompiles the whole dependency
  tree. Measured on the `build` target, one file changed:
  **`go build` 38s → 3.1s, total 60s → 8s**; a cold build also drops from 135s to
  82s because `go mod download` is cached. The binary is additionally built with
  `-trimpath -ldflags "-s -w"`: **80.2 MiB → 58.6 MiB (−27%)**, verified to still
  run (`--help`). The mounts are exported to CI through the existing
  `cache-to: type=gha` in the publish workflows.
- **Batched message persistence** — `persistPool` now coalesces queued messages
  into batches (up to 100 rows, flushed within a 20 ms window) instead of one
  `INSERT` per message, and `MessageRepository.InsertMessages` groups rows by
  their ON CONFLICT update-set so receipt semantics are preserved. Measured
  against real Postgres: **3.54 ms → 79.7 µs per message** (~44×), which turns a
  history sync from thousands of round trips into tens. A failed batch falls back
  to per-message writes so nothing is lost.
- **Selectable fast JSON** (`pkg/jsonx`) — the JSON entry point delegates to
  `encoding/json` by default and to `goccy/go-json` with `-tags go_json` (the
  image builds with it). Measured: Marshal **3783 → 2467 ns (−35%)**, allocs
  **41 → 6**; Unmarshal **6193 → 2971 ns (−52%)**. No new dependency (goccy was
  already vendored via Gin); arm64-safe (sonic was avoided).
- **gzip response compression** (`pkg/httpguard.Compression`) — the JSON API and
  the manager SPA were served uncompressed. A middleware now gzip-encodes
  compressible responses (JSON/JS/CSS/HTML/SVG, ≥1 KB) for clients that send
  `Accept-Encoding: gzip`, removing ~60–80% of JSON and ~65–75% of JS on the wire.
  No new dependency. Responses are buffered up to 1 MiB and stream through
  uncompressed beyond that (so `/chat/media/:messageId` is never held in memory),
  already-encoded bodies and `206 Partial Content` are left alone, and WebSocket
  upgrades are untouched. Disable with `HTTP_COMPRESSION=false` when a proxy in
  front already compresses.

### 🧪 Testing
- **HTTP route-table tests** (`pkg/routes`) — this package had **0% coverage**,
  so a route mounted under the wrong method, path or middleware group (or a
  handler left off) was invisible to the suite. `NewTestRouter` wires the full
  `AssignRoutes` table to no-op handlers, letting a test assert every endpoint
  group is registered, that `/swagger` follows `SWAGGER_ENABLED`, that protected
  routes 401 without a valid key (and that the admin route rejects an instance
  token), and that an unknown path is a 404.
- **Parser fuzzing** (`go test -fuzz`, run on every PR and via `make fuzz`) —
  already fuzzed: the JID builder/parser (`utils`), the SSRF URL pre-check, the
  webhook HMAC verifier and AES-GCM decryptor, the token "looks encrypted"
  backfill discriminator, the media path-traversal guard, and the message
  summary classifier. Fuzzing immediately surfaced two real edges — a captured
  `@s.whatsapp.net` JID with an empty user part, and the `+`-prefix display form
  — and encoded both as documented invariants.
- **Manager tests (Vitest + fast-check)** — the SPA had no test runner; it now
  has one (`bun run test`) covering the `/ws` frame parser with property tests
  (never throws, drops non-object payloads, round-trips valid frames). CI gains
  `fuzz-parsers` and `manager-tests` jobs.

### 🔧 Improvements
- **Per-process resource stats & benchmarking** —
  - `GET /server/stats` now separates **this service's** usage (`system.process`:
    RSS, peak RSS, heap, goroutines, GC, CPU seconds) from clearly-labelled
    **host** context (`system.host`). The old host-only numbers were useless on a
    shared machine. RSS comes from `/proc/self` and the container's cgroup, so it
    is attributable to the API even when other workloads run on the box.
  - `PPROF_ENABLED=true` (off by default) mounts `/debug/pprof` (CPU, heap,
    goroutine, …) using only the standard library — no new dependency.
  - `scripts/bench.sh` drives a load against a running node (throughput + RSS
    before/after) and `make bench` runs allocation benchmarks over the hot paths;
    use `benchstat` to compare before/after.
- **Observability** — a dependency-free Prometheus exporter at `GET /metrics`
  and a compact readiness probe at `GET /server/health`:
  - `evo_connections_up` / `evo_instances_total` — the "one silently failed to
    reconnect" signal
  - `evo_webhook_inflight` — webhook delivery back-pressure
  - `evo_persist_queue_depth` / `_capacity` / `evo_persist_dropped_total` —
    message-persistence saturation
  - `evo_bg_dropped_total` — dropped background jobs
  - `evo_sends_total` / `evo_send_failures_total` / `evo_send_latency_last_ms`
  - `/server/health` returns JSON (`status`, `instances`, `persist`) and reports
    `degraded` when instances are known but none are connected.
  Both endpoints are public, like the other health routes, and expose only
  counts — never message contents. `/metrics` includes a per-instance series
  (`evo_instance_connected{instance="…"}`) so a scrape shows *which* instance is
  down, not just how many.
- **Per-tenant limits & single-writer scaling** —
  - `MAX_INSTANCES` caps how many instances the deployment accepts (`409` past
    the limit).
  - `SEND_RATE_LIMIT_PER_MINUTE` and `SEND_MAX_CONCURRENT` bound `POST /send/*`
    per instance, so one tenant cannot starve the shared pools.
  - **Single-writer guard**: before connecting, a node takes a Postgres advisory
    lock per instance (`pkg/ownership`). A second replica refuses to
    double-connect ("already connected on another node") instead of racing —
    this was the one correctness bug that made horizontal scaling unsafe.
  - **Ownership lease + heartbeat + failover**: `instance_ownership` records
    which node owns each instance and until when; a heartbeat renews it, and
    after a node dies its leases expire so another node adopts the instances.
    No manual intervention. This is the state the routing layer reads; automatic
    request routing to the owning node is not yet wired (each node still serves
    the instances it holds, and clients should target the owning node).
    Single-node installs are unaffected (no Postgres ⇒ the guard is inert).
- **Production hardening** —
  - **Cross-tenant isolation**: a new `GetMessageByIDForInstance` repository
    lookup scopes by `instance_id`, and `GET /message/status` plus
    `GET /chat/media/:messageId` now use it, so a known message id from one
    tenant can no longer be read with another tenant's token. Covered by
    isolation tests.
  - **HTTP rate limiting**: per-credential (instance token / admin key) or
    per-IP token bucket (`RATE_LIMIT_PER_MINUTE`, default 600/min, `0` disables)
    with `429` + `Retry-After`, protecting `apikey` brute force and `POST
    /send/*` floods.
  - **CORS allowlist**: replaced the `Access-Control-Allow-Origin: *` +
    credentials combination (spec-invalid and unsafe) with a configurable
    allowlist (`CORS_ALLOWED_ORIGINS`); empty means same-origin only.
  - **Bounded background work**: fire-and-forget event work (poll votes, account
    limit refresh, app-state recovery, auto-read) now runs through a fixed-size
    `bgpool` worker pool instead of an unbounded goroutine per event.
  - **One DB pool source of truth**: `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS`
    now size the Postgres auth pool and the whatsmeow key store too (they were
    hardcoded to 25/5).
  - **Versioned migrations**: schema changes now run through ordered, recorded
    steps in `schema_migrations` (with idempotent, reviewable SQL) after the
    additive `AutoMigrate` bootstrap.
- **Public info banner** — `GET /` now answers with a JSON welcome/health
  payload (`status`, `message`, `version`, `clientName`, `manager`,
  `documentation`, `whatsappWebVersion`, and `error` when a boot/config problem
  was detected), mirroring the Evolution API banner. The manager landing page
  (`/manager` → `/manager/home`) reads it to show version and service status.
- **Webhook dead-letter queue** — webhooks that fail permanently (retries
  exhausted, or a non-retryable response) are published to RabbitMQ
  `WEBHOOK_ERROR_QUEUE_NAME` (default `webhook_errors`) with the URL, user,
  original payload, last status/response and error, when `AMQP_URL` is set.
- **SSRF protection (opt-in)** — outbound fetches of caller-supplied URLs
  (media by URL, link previews, stickers, avatars, group photos, button/carousel
  headers) can refuse loopback, private, link-local (cloud metadata), CGNAT and
  multicast destinations, connecting to the validated IP so redirects and DNS
  rebinding are covered too. Enabled by `SSRF_PROTECTION=true`; the default
  (`false`) is permissive so updating the fork does not break senders that fetch
  media from an internal host (self-hosted MinIO, Typebot, internal CDN).
- **Tests** — new unit/integration coverage for `pkg/webhooksign`, `pkg/ssrf`,
  `pkg/message/content`, the webhook producer's signing path, the message
  repository (upsert column selection, pagination, chats) and the
  HMAC/history HTTP handlers.

### 🐛 Bug Fixes
- **Chat history: one conversation per contact** — a 1:1 chat could be stored
  twice, once under its LID (`…@lid`) and once under its phone-number JID, so it
  showed as two conversations. Chat and sender are now canonicalized to the
  phone-number JID when persisting, and `/chat/chats` + `/chat/history` merge
  rows already stored under a LID (resolved via the whatsmeow LID store).
- **Manager: `/manager/messages` on a direct load** — the page now loads the
  instance list itself (previously it reported "no connected instance" until
  another page populated the store), and a **Mensagens** entry was added to the
  sidebar.
- **Manager: sending a file failed with "URL is required"** — the HTTP client's
  default `Content-Type: application/json` made axios serialize the multipart
  FormData to JSON, dropping the file. The media upload now removes that header
  so the browser sends `multipart/form-data`; the composer also shows a
  thumbnail and file size for the pending attachment.
- **Manager: read receipts** — sent messages now show the blue double check when
  the status is `Read` (previously `Read` and `Delivered` looked identical).
- **Repo: `manager/dist` build output is no longer committed** — Vite emits
  content-hashed filenames, so tracking them added a new blob on every build.
  `manager/dist/assets/` and `manager/dist/index.html` are now git-ignored
  (regenerated by `make manager-build` and the Docker build);
  `manager/dist/dashboard.html` stays tracked.

## 0.8.1

Rebrand do painel para **WaMux** (fork comunitário, não oficial), remoção
completa do fluxo de licença no manager, code-splitting das páginas e o
dashboard self-hosted embutido na aba Dashboard. Fluxo de publicação separado:
`main` gera release versionada, `develop` publica uma tag `dev` única.

> **Build comunitário não oficial.** Não é afiliado, endossado ou uma release
> oficial da Evolution Foundation. Veja `README.md` e `NOTICE`.

## 0.8.0

Community fork release on top of `0.7.2` (see `FORK_NOTES.md` for the full
list). Highlights: vendored manager source + dependency refresh, a large set of
API/manager features, a security audit pass, and a whatsmeow API audit.

> **Unofficial community build.** Not affiliated with, endorsed by, or an
> official release of Evolution Foundation. See `README.md` and `NOTICE`.

### 🐛 Bug Fixes
- **`PairError` is handled** — pairing failures after pair-success (e.g. the
  device identity could not be stored) were only visible as an "Unhandled
  event" warning, and a passkey ceremony stayed stuck until its TTL. Now the
  error is logged, surfaced on the passkey ceremony (`SetError`) and forwarded
  as a `PairError` webhook to PASSKEY/QRCODE subscribers.
- **Media retry for expired media** — `/message/downloadmedia` now accepts
  optional message context (`id`/`chat`/`fromMe`/`isGroup`/`participant`). When a
  download fails with 403/404/410, the server asks the sender's phone to
  re-upload the media (`SendMediaRetryReceipt`) and handles the
  `events.MediaRetry` response (decrypt, refresh the direct path, re-download,
  cache the bytes); a later request with the same `id` returns them. Previously
  such media stayed permanently undownloadable.
- **Security hardening from an audit** — parameterised the `ForceUpdateJid`
  device lookup (SQL injection via `POST /instance/forcereconnect/:id`), bumped
  the vulnerable dependencies (`x/image`, `pgx/v5`, `amqp091-go`; `govulncheck`
  now reports 0 reachable CVEs), added UUID validation before the log-path join
  in `GetLogs`, purge the whatsmeow device (sessions/keys/contacts) when an
  instance is deleted, and compare the admin key in constant time. The proxy
  password is no longer returned by `GET /instance/proxy/:id` (it reports
  `hasPassword`; an empty password on save keeps the stored one). See
  `docs/archive/fork-history.md` §3j.
- **Sent messages are now persisted** — previously only received messages were
  stored, so per-instance counts and `/server/stats` ignored outbound traffic.
  `SendMessage` records `Status="Sent"`, `IsFromMe` echoes are recorded as sent,
  and receipt upserts carry `instance_id`.
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
- **Push notifications suppressed after connecting** (#70/#54/#55) — `POST
  /message/presence`, `POST /message/subscribe` and the periodic presence loop
  could leave the linked device "available" even when `alwaysOnline=false`,
  which makes WhatsApp stop notifying the operator's phone. Those paths now
  restore `Unavailable` (or stop the loop) after their brief online window, and
  turning `alwaysOnline` off via `PUT /instance/:id/advanced-settings` applies
  it immediately.
- **Passkey events never reached a webhook, and `ALL` was position-dependent**
  (#105/#107) — `PasskeyRequest`/`PasskeyConfirmation`/`PasskeyError` had no
  case in the subscription filter and were silently dropped; a `PASSKEY` event
  type was added (also delivered to `QRCODE` subscribers). Subscribing with
  `ALL` first no longer expands to a list that omits `"ALL"`; the literal is
  persisted so the all-events fast-path works. An in-flight passkey ceremony is
  now cleared on `Disconnected`/`StreamReplaced` instead of being left stale.

### ✨ Features
- **More webhook events** — `PrivacySettings`, `Blocklist` (with its per-JID
  changes) and the newsletter events `NewsletterLiveUpdate` /
  `NewsletterMuteChange` are now forwarded. `PrivacySettings`/`Blocklist` go to
  CONNECTION subscribers and the newsletter events to NEWSLETTER subscribers.
- **`AccountReachoutTimelock` webhook** — whatsmeow's
  `NotifyAccountReachoutTimelock` (the account-level reach-out timelock behind
  error 463) is now forwarded to CONNECTION subscribers, so operators learn
  about a lock before cold sends start failing. `GET /instance/limits` still
  exposes the same data on demand.
- **Device platform in the instance info** — `GET /instance/overview/:id` now
  also returns `platform`/`businessName` from the whatsmeow device store (the
  phone that scanned the QR, e.g. `android`/`ios`), shown as "Dispositivo" on
  the instance cards and settings. whatsmeow has no phone *model* string.
- **Manager UI additions** — the Instances page now shows each connected
  account's profile picture, contact count, chat count and message count (from
  `GET /instance/overview/:instanceId`); instance settings gain a token **copy**
  button and a **Proxy** card (set/test/reconnect/delete); the phone number no
  longer shows the WhatsApp `:device` suffix; the sidebar shows the running
  version and a GitHub link; and the Dashboard placeholder is replaced by a
  small system-wide view (instances, messages, contacts, host metrics,
  messages/day).
- **Per-instance message and chat counts** — the `messages` table gained a
  nullable `instance_id` column, filled on insert and exposed as `messagesCount`
  / `chatsCount` in `/instance/overview/:instanceId`; deleting an instance now
  cleans up its messages by `instance_id`.
- **Swagger docs regenerated** — `docs/` now reflects the fork's routes
  (`/chat/ephemeral`, `/send/event`, `/send/product`, `/typebot/*`,
  `/instance/overview/:id`, `/server/stats`, `/instance/limits/:id`, …) and no
  longer lists the removed `/license/*` paths. The frontend API Tester reads the
  live `/swagger/doc.json`, so it is current. Use `make swagger`
  (`swag init --parseDependency`) after route/annotation changes.
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
- **`GET /instance/overview/:instanceId`** — the connected account's own profile
  picture, push name and local contact count (AuthAdmin). `GET /server/stats`
  now also returns `system.version`, and `/dashboard` shows the profile picture,
  contact count, running version and a GitHub link (see `docs/archive/fork-history.md` §3i).
- **Multiple webhooks per instance** — the `Webhook` field accepts a JSON array or
  a newline/comma/semicolon separated list; the payload is delivered to each URL.
- **`POST /user/savecontact`** route aligned (with a legacy `POST /user/contacts`
  alias) and app-state desync recovery.

### 🔧 Improvements
- **Reconnect on `StreamError` / `KeepAliveTimeout`** — whatsmeow only emits
  `StreamError` for unrecognised stream errors and never acts on
  `KeepAliveTimeout` itself. Both now heal the socket via the same reconnect
  machinery as `Disconnected` (extracted into `scheduleReconnect`), so a dead
  socket is recovered instead of waiting for the TCP layer. A single keepalive
  blip is ignored (acts on the second consecutive timeout).
- **Reactions use `BuildReaction`/`BuildMessageKey`** — the message key's
  `FromMe` is now derived from the author (comparing against both our phone
  number and our LID) and the group participant is set only for group messages,
  instead of trusting the API's `fromMe`/`participant` verbatim. The
  author-derivation is a unit-tested helper.
- **Bound retry-receipt concurrency** — `SetMaxParallelRetryReceiptHandling(10)`
  before connect. whatsmeow's default is unlimited, so a burst of
  undecryptable messages (each triggering a retry receipt) could spawn
  unbounded goroutines.
- **Wait for the connection instead of sleeping** — `ensureClientConnected` and
  `Reconnect` used fixed `time.Sleep(2s)` guesses before checking the client.
  They now use `client.WaitForConnection(timeout)` (whatsmeow's supported wait),
  so a slow connect is no longer failed early and a fast one returns
  immediately.
- **Manager frontend source is now vendored and built in-repo** — the React SPA
  source lives at `wamux-manager/` (taken from upstream's `develop`
  branch, the only place it exists) and is built into `manager/dist` by
  `make manager-build` or the Dockerfile's `oven/bun` stage. Two QR bugs from the
  `ecosb2b/evo-go-v2` fork are fixed: the QR modal read the fields capitalised
  (`data.Qrcode`/`data.Code`) while the API returns lowercase, and the
  auto-refresh effect had `onRefresh`/`instance` in its deps, so the interval
  reset before it ever fired. See `docs/archive/fork-history.md` §3i.
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


---

Earlier upstream history (up to `v0.7.2`) is in [`docs/changelog-upstream.md`](./docs/changelog-upstream.md).
