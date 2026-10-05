# WaMux performance scenarios

Measure before optimizing.

This package holds the realistic, end-to-end scenarios for the paths that decide
WaMux's throughput and memory footprint, plus a validation test for every
benchmark so a number is never reported for a path that is silently broken.

- The **HTTP pipeline** is benchmarked with the *real* middleware and handlers
  (auth, JID validation, per-instance send guard, JSON bind, response encode);
  only the outbound WhatsApp call is stubbed.
- The **group-send crypto** benchmark reproduces exactly what `whatsmeow` does in
  `send.go sendGroup` for the pinned version in `go.mod`.
- Benchmarks that need a database skip unless `EVO_BENCH_POSTGRES_DSN` is set,
  matching `pkg/message/repository`.

Run everything:

```bash
# validations + all scenarios
make bench-scenarios

# just the validations (fast, no benchmarks)
go test ./benchmarks/

# a single family
go test -run '^$' -bench 'GroupSendSim' -benchmem ./benchmarks/ -benchtime=20x

# persistence against a real database
EVO_BENCH_POSTGRES_DSN='postgresql://user:pass@localhost:5432/wamux?sslmode=disable' \
  go test -run '^$' -bench 'MessageInsert' -benchmem ./benchmarks/ -benchtime=300x
```

Compare before/after with `benchstat`:

```bash
make bench-scenarios > /tmp/before.txt
# ... change ...
make bench-scenarios > /tmp/after.txt
benchstat /tmp/before.txt /tmp/after.txt
```

## Scenarios

| File | Measures | Validation |
|---|---|---|
| `group_send_test.go` | One group send: 1 body SenderKey encryption + N pairwise SKDM encryptions. Also the counterfactual (full body per device). | `TestGroupSendRoundTrip` |
| `group_send_db_test.go` | The per-device `IsTrustedIdentity` round trip (the one DB call whatsmeow does *not* batch), with and without an identity cache. | (covered by the round trip) |
| `identity_cache_postgres_test.go` | The real `SQLStore.IsTrustedIdentity` against Postgres, raw vs `pkg/whatsmeow/identitycache` (DSN-gated). | `TestIdentityCachePostgres` |
| `http_pipeline_test.go` | Real middleware + handlers for `/send/text`, `/send/media`, `/instance/all`, `/server/health`; a variant with a modeled 20 ms WhatsApp RTT. | `TestHTTPPipeline`, `TestSendGuardRateLimit` |
| `webhook_test.go` | Event payload JSON encode, HMAC-SHA256 sign/verify, at-rest key encryption. | `TestWebhookSignVerify`, `TestWebhookEncryptDecrypt` |
| `persistence_test.go` | Single vs batched message insert against a real Postgres (DSN-gated). | `TestMessagePersistenceRoundTrip` |
| `idle_memory_test.go` | Per-client live heap, client construction cost, `debug.FreeOSMemory` reclaim, `GOMEMLIMIT`/`GOGC` bounding. | the tests themselves assert |

## Baseline (indicative)

Go 1.27.1, AMD Ryzen 7 5700X, Linux. Absolute numbers depend on the host; use
them as relative signals and for before/after comparisons, not as SLAs.

### Group send crypto (`-benchtime=20x`)

| Devices | Real path (body x1 + SKDM xN) | Naive full-body xN |
|---:|---:|---:|
| 1 | 80 µs | — |
| 10 | 228 µs | — |
| 100 | 1.86 ms | 3.77 ms |
| 300 | 4.06 ms | 11.4 ms |
| 600 | 7.79 ms | 20.7 ms |

Body encryption alone (1 KiB): **65.9 µs**. The message body is encrypted
**once**; the per-device cost is a ~14 µs pairwise encryption of the small
SenderKeyDistributionMessage.

### The one un-batched DB call (`-benchtime=5x`)

Models a **~1 ms per device** database round trip (the model uses `time.Sleep`,
so the effective cost is Linux's timer granularity — a slow/remote database).
The authoritative before/after against a real Postgres is in the next section.

| Devices | Uncached (per device) | With identity cache | Delta |
|---:|---:|---:|---:|
| 100 | 130 ms | 1.92 ms | ~68x |
| 300 | 384 ms | 4.70 ms | ~82x |
| 600 | 764 ms | 7.63 ms | ~100x |

This is the concrete "cache identities" lever: once group size is in the
hundreds, the per-device identity SELECT dominates the send, not the crypto.

**Real before/after against Postgres 18** (`identity_cache_postgres_test.go`,
600 devices, `-benchtime=10x`; absolute times vary with the database):

| Path | 600 lookups | per lookup |
|---|---:|---:|
| raw `SQLStore.IsTrustedIdentity` | ~0.6–0.9 s | ~1–1.5 ms |
| `identitycache.Store` (warm) | ~0.2 ms | ~0.3–0.4 µs |
| | | **~2000–3000x faster** |

The cache is now wired in `pkg/whatsmeow/service` (`StartClient` wraps
`deviceStore.Identities`), invalidates on every identity write, and is bounded by
a 5-minute TTL and 50k entries. A unit benchmark with an injected slow store
shows 600 lookups going from 727 ms to 148 µs, and the hit path is ~225 ns
sequential / ~239 ns under `RunParallel`.


### HTTP pipeline (`-benchtime=2000x`)

| Endpoint | ns/op | allocs/op |
|---|---:|---:|
| `POST /send/text` | 20.8 µs | 69 |
| `POST /send/media` (JSON URL) | 17.7 µs | 73 |
| `GET /instance/all` (admin) | 4.5 µs | 15 |
| `GET /server/health` | 1.0 µs | 8 |
| `POST /send/text` + 20 ms WhatsApp RTT | **20.86 ms** | 69 |

The request pipeline is ~0.1 % of a real send; the outbound WhatsApp round trip
dominates. Optimizing WaMux's own request path will not move send latency.

### Webhook delivery (`-benchtime=1s`)

| Benchmark | ns/op | throughput |
|---|---:|---:|
| payload marshal (realistic 1.4 KB event) | 4.83 µs | ~285 MB/s |
| HMAC sign (1 KiB) | 1.42 µs | ~970 MB/s |
| HMAC verify (1 KiB) | 1.40 µs | ~983 MB/s |
| AES-GCM key encrypt | 1.46 µs | — |

Signing is cheap relative to the network delivery it guards.

### Idle memory

- `TestClientMemory`: **59.7 KiB per client** (200 clients). This is the library
  client struct only; a *connected* instance adds websocket buffers, populated
  caches and history. WaMux's own wiki conservatively quotes 50–100 MB per
  instance — measure a real deployment with `PPROF_ENABLED=true`.
- `BenchmarkNewClient`: 17 µs, 61 KiB, 48 allocs per session created.
- `TestFreeOSMemoryReclaimsTransientHeap`: RSS base 37 MiB → peak 101 MiB →
  **29 MiB after `debug.FreeOSMemory()`**. Returning memory after a burst works.
- `TestGOMEMLIMITBoundsHeap`: with `GOMEMLIMIT=64 MiB` and `GOGC=50`, heap peaked
  at **52 MiB** during a churn storm. The soft limit bounds idle drift.

## How this maps to the optimization list

Both levers below are now implemented; the benchmarks above are the before/after.

| Proposed lever | Status | Evidence |
|---|---|---|
| Cache identities (`IsTrustedIdentity`) | **Implemented** | `pkg/whatsmeow/identitycache` + `BenchmarkIdentityLookupPostgres*`: 582 ms → 0.22 ms at 600 devices |
| Set `GOMEMLIMIT` / `GOGC`; `FreeOSMemory` after bursts | **Implemented** | `pkg/gctune`, wired in `cmd/wamux/main.go`; `TestGOMEMLIMITBoundsHeap`, `TestFreeOSMemoryReclaimsTransientHeap` |
| Keep Postgres local and indexed | **Ops** | `persistence_test.go` (DSN) + the identity model above |
| Prioritize the WhatsApp RTT, not the request path | **Confirmed** | endpoint is 20.8 µs vs 20.86 ms with RTT |
| Parallelize group encryption inside a client | **Do not** | `whatsmeow` serializes sends with `messageSendLock`; ratchets are not safe to parallelize |
| Cache-size setters (`SetContactCacheSize`, …) | **Not available** | they do not exist in the pinned whatsmeow |
| `sync.Pool` to cut idle RAM | **Wrong tool** | reduces allocation, not idle footprint |
| Unload idle sessions | **Breaks the gateway** | a disconnected client stops receiving messages |

### New env knobs

| Variable | Default | Meaning |
|---|---|---|
| `GO_MEMORY_LIMIT_MB` | `0` | `0` derive from cgroup, `>0` explicit MiB, `<0` disable |
| `GOGC_PERCENT` | `0` | `>0` sets `GOGC`; `0` leaves the Go default |
| `GO_MEMORY_RECLAIM_INTERVAL_SECONDS` | `300` | idle-heap reclaimer period; `0` disables |
| `GO_MEMORY_RECLAIM_MIN_IDLE_MB` | `64` | only call `FreeOSMemory` when idle heap exceeds this |

`GOMEMLIMIT`/`GOGC` set directly in the environment are honoured and never
overridden.

