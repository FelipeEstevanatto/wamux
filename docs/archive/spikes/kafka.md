# Spike — Kafka producer

**Status:** design only; no production code.
**Date:** 2026-11-22

## Need

Reforged supports Kafka per instance and globally. WaMux already has webhook,
RabbitMQ, NATS (core + JetStream) and WebSocket. Kafka is the most requested of
the missing sinks, usually for an existing data platform.

## Cost

- Dependency: `github.com/twmb/franz-go` (pure Go, no cgo) or
  `segmentio/kafka-go`. franz-go is the better fit: one client per process,
  explicit acks, idempotent producer, no mandatory ZooKeeper.
- Configuration: brokers, client id, topic prefix, per-instance/global events,
  SASL/TLS. Mirrors the existing event config.
- Operational: partitions and retention belong to the operator. WaMux must not
  create topics by default.
- Failure mode: Kafka down must not block WhatsApp event handling. Use a bounded
  per-topic buffer and drop with a metric/log, like the webhook endpoint budget.

## Design

- Add `pkg/events/kafka` implementing the existing `Producer` interface.
- One writer per process (`kgo.Client`), topics `wamux.<event>` or
  `wamux.<instance>.<event>` when `KAFKA_GLOBAL_ENABLED=false`.
- `acks=all`, idempotent producer, retries with a bounded timeout.
- A `Drainer` implementation flushes on shutdown.
- Per-instance enable flags in the instance model, matching RabbitMQ/NATS.
- Metrics: `evo_kafka_publish_total{result}`, `evo_kafka_pending`.

## Recommendation

Build it only when a user asks for Kafka. It is a 3–5 day change with tests, and
does not belong in the core binary unless the dependency is acceptable: put it in
a separate module/build tag (`-tags kafka`) to keep the default binary lean.
The generic `Producer` interface already isolates it.

Decision: **defer** until there is a concrete deployment requirement.
