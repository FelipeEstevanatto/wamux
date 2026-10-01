# WaMux — engineering notes

WaMux was originally forked from
[`evolution-foundation/evolution-go`](https://github.com/evolution-foundation/evolution-go)
at `0.7.2` (upstream commit `9337afc`). This document summarises what was changed
and why.

- Per-release detail: [`CHANGELOG.md`](./CHANGELOG.md)
- Full history — PR reviews, feature archaeology and live-test findings:
  [`docs/archive/fork-history.md`](./docs/archive/fork-history.md)

## What changed vs. upstream

- **No license, heartbeat or telemetry.** The activation gate, heartbeat and
  telemetry are gone; the API is fully operational from first boot. The
  `LICENSE`, `NOTICE` and `TRADEMARKS.md` notices are retained (Apache-2.0) and
  the Manager keeps a local `/license/*` compatibility stub so the prebuilt UI
  works without re-enabling activation.
- **Stability.** One shared whatsmeow `sqlstore` container (no leaked Postgres
  pool per reconnect), mutex-guarded shared maps, a newer whatsmeow, reconnect
  backoff, paired-session restore on startup, WebSocket multi-subscriber races,
  app-state desync recovery, NATS skipped when unconfigured.
- **Correctness.** Decrypt edits/revokes before forwarding, canonical JIDs,
  bounded avatar/HTTP calls, LID→phone resolution, contact saving, multi-device
  destination routing, disappearing-messages timers, and modern interactive
  message payloads (buttons/lists/carousels/PIX).
- **Features.** Versioned manager source, per-instance overview (picture,
  platform, counts), proxy settings and checks, account limits, Typebot,
  webhook HMAC signing, SSRF protection, message history readback, per-instance
  S3 storage, `/server/stats` and a self-hosted `/dashboard`.

## Distribution & compliance

- Apache-2.0. The image ships `LICENSE`, `NOTICE`, `TRADEMARKS.md` and
  `FORK_NOTES.md` in `/app`.
- The license's **usage-notification** condition is satisfied by the Manager's
  **Sobre / About** page; anyone embedding the image inherits that obligation.
- WaMux is an independent project, originally forked from Evolution Go. It is
  **not** affiliated with, endorsed by, or an official release of Evolution
  Foundation.

## Operational notes

These are current behaviours worth knowing; full detail is linked.

- **Typebot** is optional and inert until a bot is configured. It talks to
  Typebot's HTTP chat API (no JS engine, so script blocks don't run; no
  debounce; only text advances a conversation; the per-contact rate limit is on
  by default; one bot per instance; `TYPEBOT_*` is read at boot).
  → [archive §3d](./docs/archive/fork-history.md)
- **Media processing** shells out to `ffmpeg`/`ffprobe`/`pdftoppm`, which live in
  the runtime image, not the Go binary. Run the container, or install those
  tools on the host, or some enhancements degrade silently.
  → [archive §3f](./docs/archive/fork-history.md)
- **Interactive messages**: rendering differs by client — `single_select` lists
  render on mobile but not Web/Desktop; carousels need media on every card; the
  legacy `listMessage` is dead and must not be used.
  → [archive §3g](./docs/archive/fork-history.md)
- **Calls**: only `POST /call/reject` exists; a full VoIP stack is a separate
  feature (a reference implementation is noted in the archive).
  → [archive §3e](./docs/archive/fork-history.md)

## Build & run

```bash
make setup     # deps + .env
make dev       # run with -dev
make build     # -> build/wamux
make test      # go test ./...
make swagger   # regenerate the OpenAPI docs
```

Docker: `docker compose up -d --build` (also builds the manager SPA via an
`oven/bun` stage). Manager alone: `make manager-install && make manager-build`.

## Verification

- `go build ./...`, `go vet ./...` and `go test ./...` pass.
- `govulncheck ./...` reports **0** reachable vulnerabilities; the `Security`
  workflow also runs `go test -race` and gitleaks.
- Feature-level live-test notes (interactive rendering, NCT/error 463, presence,
  disappearing messages) are recorded in the archive.

## Known limitations

- Unofficial WhatsApp client: WhatsApp can change or block it; use at your own
  risk.
- `PASSKEY_PUBLIC_URL` must be reachable by the browser for passkey pairing.
- The Typebot integration is best-effort (covered above).
- Local media copies (for in-thread previews) are opt-in; outbound media is not
  uploaded to S3.
