# Idempotent sends

`POST /send/text` and JSON `POST /send/media` accept an `Idempotency-Key`
header (up to 255 characters). The first request runs normally; a retry with the
same method, path and exact body replays the first response and sends nothing.

## Contract

| Situation | Result |
| --- | --- |
| First use | normal `200`/`4xx`; `X-Wamux-Operation-Id` is returned |
| Same key + same body, finished | the first status and body are replayed |
| Same key + different body | `409` |
| Same key while the first is still in flight | `202 {"status":"pending"}` |
| Key on a multipart upload | `400` (hash the JSON form instead) |
| Key on an album | `400` (each item is its own message) |

`GET /send/operations?state=pending|sent|failed&limit=50` lists this instance's
records, oldest first. `GET /send/operations/:id` returns one.

## Recovery

Every new operation reserves a 32-character WhatsApp id before transport and
puts it in `planned_message_id`. If the process dies while the row is
`pending`, `GET /send/operations/:id` checks whether a message with that id is
already in history and marks the operation `sent` if so. A missing message is
not proof that nothing was sent, so the operation stays pending in that case.

Inspect the pending list after a restart, then check WhatsApp's history or the
recipient before deciding to resend with a new key.

The records live in `send_operations` (migration `0012`). Keep the table when
rolling back: it is the evidence of what was attempted.
