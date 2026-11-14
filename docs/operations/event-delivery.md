# Event delivery and shutdown

Events can leave the process through HTTP webhooks, RabbitMQ, NATS or the
WebSocket stream. RabbitMQ publishes with confirms; the webhook and NATS
producers buffer work, so both implement `Drain(ctx)`.

## Shutdown

On `SIGTERM` the server stops accepting HTTP requests, drains the message
persistence pool, then drains the buffered event producers with an 8-second
budget. A dead webhook endpoint cannot hold shutdown open indefinitely; anything
still queued after the deadline dies with the process.

Look for these lines in `docker logs`:

```
[SHUTDOWN] Persistence queue drained
[SHUTDOWN] Event producers drained
```

## Webhook dead-letter queue

Permanently failed webhooks are published to RabbitMQ `WEBHOOK_ERROR_QUEUE_NAME`
(default `webhook_errors`) when RabbitMQ is configured. Each message is:

```json
{
  "url": "https://consumer.example/hook",
  "userID": "instance-id",
  "payload": { "...": "original event" },
  "statusCode": 500,
  "response": "upstream body",
  "attemptTime": "2026-11-14T12:00:00Z",
  "error": "..."
}
```

Replay from the RabbitMQ management UI or a small consumer that POSTs `payload`
to `url` again. The payload already carries the event and `instanceToken`; HMAC
signing is not added by a replay unless the consumer asks for it.

## NATS JetStream

Set `NATS_JETSTREAM=true` and `NATS_STREAM=WAMUX_EVENTS`, then create the stream
before starting WaMux, for example:

```bash
nats stream add WAMUX_EVENTS \
  --subjects 'sendmessage,*.sendmessage' \
  --storage file --retention limits \
  --max-msg-size 8388608
```

The default JetStream max message size is 1 MiB, while an event can embed up to
`WEBHOOK_BASE64_MAX_BYTES` of media (≈6.7 MiB after base64 for the 5 MiB
default). WaMux checks the stream limit at startup and warns when it is too
small. An event larger than the stream limit is still published on core NATS,
but it is **not durable**; that is why the stream limit should be raised (or
media moved to an object store).

Global subjects are the lower-case event name (`sendmessage`, `message`,
`receipt`); instance subjects are `<instance-id>.<event>`. The producer waits for
the stream ack and uses the SHA-256 of the payload as `Msg-Id`, so a retry inside
JetStream's duplicate window is stored once. If the stream is missing or rejects
the subject, the producer logs a warning and falls back to core NATS rather than
dropping the event.
