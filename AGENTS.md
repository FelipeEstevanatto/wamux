# Working in WaMux

WaMux is a self-hosted WhatsApp API: Go on [whatsmeow](https://github.com/tulir/whatsmeow),
with a React manager in `wamux-manager/`. This file is for anyone changing the code,
people and coding agents alike. `scripts/check-docs.sh` keeps it honest.

## Layout

| Path | What lives there |
| --- | --- |
| `cmd/wamux/main.go` | Startup and wiring |
| `pkg/routes/routes.go` | Every HTTP route |
| `pkg/whatsmeow/service/whatsmeow.go` | Client lifecycle and the event switch |
| `pkg/whatsmeow/service/message_lifecycle.go` | Dedupe, revokes and history sync |
| `pkg/sendMessage/service/send_service.go` | Outbound messages |
| `pkg/message/repository/message_repository.go` | Message storage |
| `pkg/migrations/migrations.go` | Schema migrations |
| `wamux-manager/src/` | The manager |
| `docs/wiki/` | User documentation, in Portuguese |
| `benchmarks/` | Benchmarks and their baselines |

## Commands

| Command | Use |
| --- | --- |
| `make test` | Go tests |
| `make test-race` | Go tests with the race detector, as CI runs them |
| `make test-manager` | Manager tests |
| `make lint` | gofmt and golangci-lint |
| `make docs-check` | Checks this file and the environment variable docs |
| `make smoke` | Builds the image and boots it against Postgres |
| `make swagger` | Regenerates the API docs after changing handler comments |
| `make hooks` | Installs the git hooks in `.githooks/` |

Many tests skip without their service, and a skipped test passes silently. Set
`EVO_BENCH_POSTGRES_DSN` for anything touching `pkg/message/repository`: the upsert
semantics only run on real Postgres. CI sets all of them, see
`.github/workflows/security.yml`.

## Rules

- **No real data.** The repository is public. Keep phone numbers, JIDs, tokens and
  anything from `.env` out of code, tests, docs and commits. Use the fake numbers
  already in the tests.
- **Log ids, not content.** Message text, vCards, contacts and whole events do not go
  to the logs or stdout. Avoid `%+v` on an event. The one exception is the pairing QR,
  which expires within a minute.
- **Webhook payloads are a contract.** Add fields; do not rename or remove them.
- **Status only moves forward.** Go through `upsertClause` and `keepHighestStatus`
  rather than writing `status` directly.
- **A revoked message stays empty.** Writes must not bring back a `deleted` row.
- **Peers are untrusted.** A revoke or edit is a message someone sent. Check it
  belongs to the chat it arrived in before acting on it.
- **LID and phone JIDs are one chat.** Store with `CanonicalChatJID`, and read with
  both forms.

## Changes

- Commits follow `type(scope): summary`, such as `fix(send): keep the number as given`.
- Add behaviour changes to `CHANGELOG.md`, under Unreleased.
- A new environment variable goes in `.env.example`, or the docs check fails.
- WhatsApp clients differ. Before calling a message type supported, read the
  compatibility notes in `docs/wiki/guias-api/api-interactive.md`.
