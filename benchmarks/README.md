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

## Hardware comparison: dedicated vs shared VPS

Same code (develop), same commands, same Go 1.27.1, measured on the two
deployments. Ratios are **VPS ÷ dedicated** (>1 means the VPS is slower).

- **Dedicated:** AMD Ryzen 7 5700X (8c/16t), 16 GB RAM, local SSD.
- **Shared VPS:** AMD EPYC 9354P with **2 vCPU** exposed, 7.8 GB RAM, co-tenanted
  with other workloads.

| Scenario | Dedicated (µs) | Shared VPS (µs) | VPS/local |
|---|---:|---:|---:|
| Group body encrypt | 78.5 | 107.8 | 1.37x |
| Group send, 100 devices | 1539 | 1687 | 1.10x |
| Group send, 300 devices | 4378 | 6454 | 1.47x |
| Group send, 600 devices | 7927 | 11730 | 1.48x |
| Naive full-body×600 | 20985 | 28483 | 1.36x |
| `POST /send/text` | 16.6 | 17.2 | 1.04x |
| `POST /send/media` | 17.3 | 16.9 | 0.98x |
| `GET /instance/all` | 4.2 | 5.3 | 1.26x |
| `GET /server/health` | 0.82 | 2.74 | 3.35x |
| `/send/text` + 20 ms RTT | 20866 | 20565 | 0.99x |
| New client (allocs) | 18.5 | 31.0 | 1.67x |
| Webhook marshal (1.4 KB) | 4.97 | 5.24 | 1.05x |
| Webhook HMAC sign | 1.51 | 1.57 | 1.04x |
| Webhook HMAC verify | 1.41 | 1.77 | 1.25x |
| AES-GCM key encrypt | 1.49 | 2.35 | 1.58x |

Database-bound (throwaway Postgres 18 container on each host):

| Scenario | Dedicated | Shared VPS | Note |
|---|---:|---:|---|
| Identity lookup, 600 devices (raw) | 593 ms | 353 ms | absolute DB time is host/config dependent |
| Identity lookup, 600 devices (cached) | 0.203 ms | 0.276 ms | |
| **identity-cache speedup** | **~2900x** | **~1280x** | the win holds on both |
| Message insert (single) | 3.65 ms | 1.62 ms | |
| Message insert (batch, per row) | 90.5 µs | 67.6 µs | |

Memory behaviour is effectively identical (the point of the two optimizations):

| Metric | Dedicated | Shared VPS |
|---|---:|---:|
| Heap per idle client | 59.65 KiB | 59.57 KiB |
| RSS after `FreeOSMemory` (base→peak→after) | 37→101→**28** MiB | 38→102→**32** MiB |
| Peak heap under `GOMEMLIMIT=64 MiB`, `GOGC=50` | 54 MiB | 57 MiB |

Takeaways:

- **CPU-bound paths** (group crypto, webhook, request pipeline) are ~1.0–1.7x
  slower on the shared 2-vCPU box — the expected penalty of fewer, shared cores.
  A single group send to 600 devices is ~12 ms of crypto on the VPS versus
  ~8 ms dedicated, and the naive full-body model is ~28 ms, confirming the
  sender-key design is not the bottleneck on either.
- **The request path is irrelevant** on both: `/send/text` is ~17 µs against a
  ~20 ms WhatsApp round trip (0.08%).
- **DB latency varies by host**, so the identity cache matters more than the
  absolute query time: it removes the per-device round trips entirely
  (~1280–2900x on the same lookup).
- **Memory is host-independent**: per-client heap, RSS reclamation and the
  `GOMEMLIMIT` bound match on consumer and VPS hardware.

Deployed on the VPS as `ghcr.io/felipeestevanatto/wamux:dev` (built from
`develop`); the running container logs `[GCTUNE] idle-heap reclaimer every 300s`,
confirming the changes are live.

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

