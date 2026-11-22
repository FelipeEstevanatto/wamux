# Spike — generic HTTP chatbot adapter

**Status:** design only; no production code.
**Date:** 2026-11-22

## Why

Reforged carries eight chatbot integrations (Typebot, Chatwoot, OpenAI, Dify,
Flowise, N8N, EvoAI, EvolutionBot). Porting them one by one is a maintenance
trap. Most of them are the same loop: receive a message, POST it to a provider,
send the reply. A single generic adapter covers N8N, Dify, Flowise and custom
services; Chatwoot and OpenAI transcription can remain separate if demand
justifies them.

## Proposed contract

`POST <bot.url>` with:

```json
{
  "instanceId": "…",
  "sessionId": "…",
  "remoteJid": "5511…@s.whatsapp.net",
  "pushName": "Alice",
  "fromMe": false,
  "text": "hello",
  "media": {"url": "…", "mimeType": "image/jpeg"},
  "quoted": {"id": "…", "text": "…"},
  "timestamp": 1760000000
}
```

The provider returns one of:

```json
{"messages": [{"type": "text", "text": "hi"}]}
{"messages": [{"type": "media", "url": "https://…", "caption": "…"}]}
{"messages": [], "close": true}
```

Responses are bounded (max 5 messages, 64 KiB), time-limited (15 s), and sent
through the existing `sendService` so rate limits and idempotency apply.

## Features to port from the Reforged model

- **Triggers:** always / keyword / regex, with a per-bot operator.
- **Session state:** one row per (instance, bot, remoteJid), TTL and pause.
- **`ignoreJids`:** never auto-reply to groups/channels/self.
- **Stop keyword:** closes the session.
- **Debounce:** optional window so a burst produces one call, not five.
- **`keepOpen`:** keep the session after a reply.
- **Per-contact rate limit:** already implemented for Typebot; reuse it.
- **Flood auto-pause:** reuse the Typebot protection and operational alert.

## Storage

Add `chatbot_bots` and `chatbot_sessions` tables. A bot has `provider` (only
`http` initially), URL, secret/header, enabled, triggers and settings. Secrets
use the existing `DataEncryptionKey`. Migration is additive.

## SSRF and secrets

The provider URL is tenant-configured: use `ssrf.Integrations()` and the
`INTEGRATIONS_ALLOW_PRIVATE` opt-in. Do not forward the instance token by
default; sign the callback instead (`X-WaMux-Signature`, HMAC with a per-bot
secret) so the provider can verify and call back safely.

## Recommendation

Build one `http` provider behind `CHATBOT_HTTP_ENABLED=false`. Do not port the
other six. If Chatwoot is required, treat it as a separate project: it is a
bidirectional CRM sync, not a request/response bot.

Estimated effort: 1–2 weeks for the adapter, tables, manager form and tests.
