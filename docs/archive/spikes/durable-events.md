# Spike — durable inbound event handling

**Status:** design only; no production code.
**Date:** 2026-11-22

## Problem

WhatsApp acknowledges a message before whatsmeow dispatches `events.Message`. If
the process crashes (or is killed during a deploy) after the ack but before the
handler finishes, WhatsApp will not resend it. The same applies to receipts,
history batches, chats, contacts and groups. WaMux currently processes events in
memory and drains in-flight handlers on a graceful shutdown, which covers SIGTERM
but not a crash or an OOM kill.

Reforged solves this with a per-instance Redis Stream (XADD before processing,
XDEL after success, replay on the next socket). We should solve it with WaMux's
constraints: no mandatory Redis, Postgres already present, and at-least-once
replay that does not double-count messages.

## Options

| | Postgres inbox | NATS JetStream | Redis Streams |
|---|---|---|---|
| Extra service | none | only when NATS already deployed | Redis required |
| Write cost per event | one INSERT/upsert | one publish + ack | one XADD |
| Ordering | per-instance `seq` | subject + consumer order | stream order |
| Replay | `processed_at IS NULL` on boot | durable consumer replay | XRANGE pending |
| Retention | SQL delete job | stream max age/bytes | MAXLEN |
| Multi-node | safe (DB) | safe (stream) | safe |
| Operational complexity | low | medium | medium-high |

### A. Postgres inbox (recommended)

Table:

```sql
CREATE TABLE event_inbox (
  instance_id text NOT NULL,
  seq         bigserial,
  event_key   text NOT NULL,       -- hash(type + stable ids + timestamp)
  kind        text NOT NULL,
  payload     bytea NOT NULL,      -- protobuf/JSON, never message text
  received_at timestamptz NOT NULL DEFAULT NOW(),
  processed_at timestamptz,
  attempts    int NOT NULL DEFAULT 0,
  PRIMARY KEY (instance_id, seq),
  UNIQUE (instance_id, event_key)
);
```

Flow: `BEGIN; INSERT ... ON CONFLICT DO NOTHING; COMMIT` before dispatch →
process → `UPDATE processed_at`. On boot, each instance replays rows with
`processed_at IS NULL` in `seq` order, then resumes live events. A failed handler
leaves the row unprocessed and `attempts` increments; after N attempts it goes to
a quarantine table/alert.

Risks:

- **Ordering:** live events can overtake a replay. Gate live processing behind a
  per-instance replay barrier.
- **Idempotence:** message upserts already use `ON CONFLICT`; the counter rollup
  trigger only fires on insert, so a replay of an already-inserted row is safe.
  Webhooks and chatbots would fire again for replayed-but-unacked events; that is
  the at-least-once contract and must be documented.
- **Payload size:** history batches are the largest; cap at 8 MiB and split like
  Reforged. A single message over the cap is processed without durability.
- **Write amplification:** one insert + one update per event. Batch the update
  (flush every 100 events or 200 ms). Measure before enabling by default.

### B. NATS JetStream

Attractive when the operator already runs NATS (we already support NATS
JetStream for outbound). But it is not universal, and replay of typed whatsmeow
events still needs the same serialization/idempotence work. Use as an optional
backend, not the default.

### C. Redis Streams

Closest to Reforged and operationally proven there. Rejected as the default
because it adds a mandatory service to every deployment; acceptable as an
optional backend only if a future deployment already standardizes on Redis.

## Recommendation

1. Ship the **graceful drain** already implemented (SIGTERM, in-flight handlers).
2. Prototype the Postgres inbox behind `EVENT_INBOX_ENABLED=false`.
3. Success criteria: zero lost messages in a kill -9 test during a 5k-message
   burst; replay completes in < 30 s; p95 event latency increase < 10 ms;
   no duplicate message rows; webhook duplicates documented.
4. Only then decide whether the default should change.

Estimated effort: 2–3 weeks including tests, migration, metrics and a kill -9
fault-injection harness.
