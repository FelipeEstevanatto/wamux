# Metrics and avatar cache

## `/metrics`

`METRICS_AUTH` controls the Prometheus endpoint:

| Value | Behaviour |
| --- | --- |
| `admin` (default) | requires `apikey: $GLOBAL_API_KEY` |
| `loopback` | only a loopback TCP peer can scrape; forwarded headers do not grant access |
| `none` | public, the pre-1.1 behaviour |

The series include instance ids, connection state, send latency and queue depth.
Prefer `admin` or `loopback` on an internet-facing deployment. The health probes
`/server/ok` and `/server/health` stay public.

## Avatar cache

`POST /user/avatar` caches per instance + resolved JID + `preview` flag:

- a found picture for 10 minutes;
- "no picture" for 1 minute;
- temporary failures are not cached.

Concurrent misses for the same key are collapsed into one WhatsApp profile query.
The cache is in-process, so a restart starts cold and a multi-node deployment
queries once per node.
