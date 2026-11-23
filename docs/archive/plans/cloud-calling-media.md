# Calling media plane — TODO and design notes

**Status:** not implemented; the Calling control plane is implemented behind
`CLOUD_CALLING=true`. This document is the working TODO for adding media.

**Scope:** Cloud API (WABA) calling first. The Pion/TURN/browser layer is designed
to be reusable, but **whatsmeow is not enabled by this work**; see
[WABA vs whatsmeow](#waba-vs-whatsmeow).

**Last updated:** 2026-11-23.

---

## Current state

Implemented in `pkg/cloud/calling`:

- call start/get/action control (`connect`, `pre_accept`, `accept`, `reject`,
  `terminate`);
- call permissions and call settings;
- `calls` webhook persistence in `cloud_call_events`;
- `POST /cloud/calling/calls/:id/media` returns an explicit
  `501 not_implemented_on_channel`.

Deliberately not implemented:

- SDP offer/answer media negotiation;
- WebRTC/DTLS/SRTP audio (or video);
- TURN/STUN infrastructure;
- agent browser signaling and call UI;
- recording, IVR, transfer, supervisor/multi-agent.

The current 501 is intentional: accepting a call without a media endpoint would
produce silence, not a working call.

---

## Architecture

Recommended production path:

```text
WhatsApp user
   ⇅ WebRTC / Opus / DTLS-SRTP
Meta Cloud Calling API
   ⇅ SDP via Graph calls endpoint + signed `calls` webhooks
WaMux Calling control plane
   ⇅ internal signaling
Pion media gateway (one PeerConnection to Meta, one to the agent)
   ⇅ WebRTC / TURN
Agent browser
```

Pion is enough for 1:1 agent calls. An SFU (LiveKit/Janus) is only needed for:

- multiple agents on one call;
- supervisor barge-in/listen;
- recording;
- IVR/SIP/PSTN;
- call queues and transfers.

TURN alone does **not** implement calls. It only helps browsers/gateways traverse
NAT and firewalls.

### Recommended key split

```text
pkg/calling/media        Pion gateway, TURN credentials, media session state
pkg/calling/signaling    channel-neutral CallSignaler interface
pkg/cloud/calling        WABA signaling adapter (Graph + calls webhooks)
pkg/whatsmeow/calling    future adapter only if upstream supports call media
```

Suggested interface:

```go
type CallSignaler interface {
    Start(ctx context.Context, req CallRequest) (SDPOffer, error)
    Accept(ctx context.Context, callID string, answer SDPAnswer) error
    PreAccept(ctx context.Context, callID string, answer SDPAnswer) error
    Reject(ctx context.Context, callID string) error
    Terminate(ctx context.Context, callID string) error
}
```

The media gateway must not depend on WABA types so it can be reused later.

---

## WABA SDP flow

Meta's Cloud Calling API uses WebRTC signaling over Graph + webhooks. Meta does
not provide TURN/STUN. Meta uses ICE-lite; the business side should take the
**controlling** ICE role.

### Business-initiated call

1. Build an SDP offer in the media gateway.
2. Call `POST /{phone-number-id}/calls`:

```json
{
  "messaging_product": "whatsapp",
  "to": "14085551234",
  "action": "connect",
  "session": {
    "sdp_type": "offer",
    "sdp": "<RFC 8866 SDP>"
  },
  "biz_opaque_callback_data": "internal-call-id"
}
```

3. Meta sends a signed `calls` webhook with an SDP answer.
4. Apply the answer to the Pion `RTCPeerConnection`; media starts.

### Inbound call

1. A signed `calls` webhook contains the SDP offer.
2. Pion creates the SDP answer.
3. Call `POST /{phone-number-id}/calls` with `pre_accept`:

```json
{
  "messaging_product": "whatsapp",
  "call_id": "wacid...",
  "action": "pre_accept",
  "session": {
    "sdp_type": "answer",
    "sdp": "<RFC 8866 SDP>"
  }
}
```

4. Send `accept` after the media connection is ready.
5. Media flows immediately after accept.

The webhook may expose SDP under `session.sdp` and/or
`connection.webrtc.sdp`; parse both defensively.

### Control rules

- Do not accept before the answer is ready.
- `pre_accept` is recommended before `accept` to avoid audio clipping.
- Media actions are not ordinary message sends; do not route them through
  `sendPayload`/`sendError`.
- Never retry a call `connect` blindly. A timeout may mean the call exists.

---

## TURN / coturn

Use coturn for agent-browser and gateway media in restrictive networks.

### coturn configuration

```conf
listening-port=3478
tls-listening-port=5349
fingerprint
use-auth-secret
static-auth-secret=<shared-secret>
realm=calls.example.com
external-ip=<public-ip>
min-port=49152
max-port=65535
no-multicast-peers
```

Expose:

- UDP `3478` and TCP `3478`;
- TLS `5349`;
- relay UDP `49152–65535`;
- a valid TLS certificate for `turns:`.

### Ephemeral credentials

Do not send the static secret or long-lived username/password to the browser.
Use coturn's REST credentials:

```text
username   = <unix-expiry>:<instance-id>
credential = base64(HMAC-SHA1(shared-secret, username))
ttl        = 1 hour
```

Add:

```text
GET /cloud/calling/ice
```

returning `iceServers` with `urls: ["stun:...", "turn:...", "turns:..."]`,
username and credential. Require an instance token and scope the credential to
the instance/call.

Suggested env:

```env
CLOUD_CALLING_MEDIA_ENABLED=false
CLOUD_CALLING_PUBLIC_IP=
CLOUD_CALLING_MEDIA_PORT_MIN=49152
CLOUD_CALLING_MEDIA_PORT_MAX=65535
CLOUD_CALLING_TURN_URLS=turn:calls.example.com:3478?transport=udp,turns:calls.example.com:5349
CLOUD_CALLING_TURN_SECRET=
CLOUD_CALLING_TURN_TTL=1h
```

Keep TURN credentials out of logs, webhook payloads and manager responses.

---

## Implementation TODO

### Phase 1 — media core

- [ ] Add `github.com/pion/webrtc/v4` and `github.com/pion/rtp`.
- [ ] Create `pkg/calling/media`.
- [ ] `Gateway.CreateOffer(callID)` → local SDP.
- [ ] `Gateway.AcceptOffer(callID, offer)` → SDP answer.
- [ ] `Gateway.AcceptAnswer(callID, answer)`.
- [ ] `Gateway.AddAgent(callID)` / browser peer connection.
- [ ] Bridge Opus RTP between Meta and agent peer connections.
- [ ] Set ICE controlling role; configure public IP/port range.
- [ ] Close/cleanup sessions on `terminate`, timeout and shutdown.
- [ ] Track RTP/RTCP stats: packet loss, jitter, RTT, bitrate.
- [ ] Explicitly reject unsupported video/media layouts.

### Phase 2 — signaling and APIs

- [ ] Extend `calling.Service.Start` with a `session` offer and
      `biz_opaque_callback_data`.
- [ ] Extend `calling.Service.Action` with SDP answer payloads.
- [ ] Add `PreAccept`/`Accept` helpers.
- [ ] Parse `calls[].session.sdp` and `calls[].connection.webrtc.sdp`.
- [ ] Add `cloud_call_sessions`:
      `call_id`, `instance_id`, `direction`, `state`, `agent_id`,
      `sdp_offer`, `sdp_answer`, `started_at`, `ended_at`.
- [ ] Add `GET /cloud/calling/calls/:id/session`.
- [ ] Add `POST /cloud/calling/calls/:id/answer` for an agent answer.
- [ ] Add `POST /cloud/calling/calls/:id/offer` for outbound.
- [ ] Add `GET /cloud/calling/ice`.
- [ ] Publish call events to the existing webhook/NATS/WebSocket event system.
- [ ] Never persist raw SDP or media encryption keys longer than needed.

### Phase 3 — agent browser

- [ ] Add authenticated WebSocket signaling:
      `/ws/calling/:callId` (or a ticket-based equivalent).
- [ ] Browser `RTCPeerConnection` with coturn credentials.
- [ ] Incoming-call ringing/answer/reject UI.
- [ ] Mute, hang up, audio device selection and connection-state display.
- [ ] Handle browser refresh/network loss/idempotent answer.
- [ ] Add a manager call panel/toast; do not auto-answer.
- [ ] Redact phone numbers/BSUIDs from non-essential logs.

### Phase 4 — production media service

- [ ] Prefer a separate `cmd/wamux-call-media` process for UDP ports and scale.
- [ ] Sticky call routing or a shared session broker (Redis/NATS).
- [ ] Co-locate TURN/gateway with low-latency network paths.
- [ ] Health/readiness endpoints for TURN and gateway.
- [ ] Graceful drain of active calls before deploy.
- [ ] Load test with concurrent calls and packet loss.
- [ ] Runbook: no audio, one-way audio, ICE failure, TURN auth failure, codec
      mismatch, call stuck in pre_accept.

### Phase 5 — optional SFU/IVR

- [ ] Choose LiveKit (fastest to operate) or Janus/FreeSWITCH (SIP/IVR).
- [ ] Use Pion/Janus/FreeSWITCH gateway to terminate Meta WebRTC.
- [ ] Recording consent, retention and encryption.
- [ ] Transfer, queue, supervisor and DTMF as separate features.

---

## WABA vs whatsmeow

This media implementation is **WABA-specific at the signaling layer** and
**channel-neutral at the media layer**.

### Reusable for whatsmeow

- coturn/TURN credentials;
- Pion media gateway;
- browser WebRTC client and manager call UI;
- call-session store, metrics, recording and RTP handling.

### Not reusable

- Graph API call endpoints and `calls` webhook parsing;
- Meta SDP offer/answer contract;
- WABA call permissions/settings;
- WABA call IDs and error mappings.

### Current whatsmeow reality

In the installed whatsmeow version
(`v0.0.0-20261116172437-19d61352436f`), WaMux has:

- `CallOffer`, `CallAccept`, `CallPreAccept`, `CallTransport`, `CallTerminate`,
  `CallReject`, `CallOfferNotice`, `CallRelayLatency` events;
- raw `waBinary.Node` data on events;
- `RejectCall(...)`;
- no supported `AcceptCall`/media engine API in the installed module.

So the Pion gateway can be reused later, but **whatsmeow calls will not start
working automatically**. A whatsmeow adapter needs one of:

1. upstream whatsmeow support for accepting calls and exposing media/session
   state; or
2. a separate protocol-level implementation parsing raw call nodes, deriving
   call keys and handling WhatsApp relay/transport; or
3. keeping whatsmeow at notification/reject only.

Options 1–2 are a separate R&D program. Option 2 has breakage, encryption,
relay and account-risk concerns and must not be bundled with WABA media.

### Future abstraction

```go
type MediaBackend interface {
    CreatePeer(callID string) (Peer, error)
    AttachAgent(callID, agentID string) error
    Close(callID string) error
}

type WABASignaler interface { /* Graph + calls webhooks */ }
type WhatsmeowSignaler interface { /* raw call nodes, if supported */ }
```

Keep the browser/media code free of `pkg/cloud` imports if it is expected to
serve multiple channels later.

---

## Security and operations

- Serve TURN over TLS; rotate the shared secret.
- Restrict relay ports and use least-privilege firewall rules.
- Rate-limit call start and answer endpoints.
- Require instance authentication on ICE/signaling routes.
- Bind media sessions to one instance and one call ID.
- Do not expose SDP, call keys, TURN secrets or raw phone numbers in logs.
- Enforce call recording consent and retention.
- Metrics/alerts: setup latency, active calls, ICE failures, packet loss,
  jitter, RTT, one-way audio, TURN auth failures.
- Keep the existing `CLOUD_CALLING` flag off by default; add
  `CLOUD_CALLING_MEDIA_ENABLED` for the media rollout.

---

## Testing checklist

- [ ] Unit test SDP offer/answer mapping with fixtures.
- [ ] Pion loopback/PCMA/Opus gateway test.
- [ ] Fake Graph call fixture for outbound connect and inbound pre_accept.
- [ ] Signed `calls` webhook fixture (valid + invalid signature).
- [ ] Answer idempotency and late/duplicate webhooks.
- [ ] coturn credential HMAC/TTL tests.
- [ ] Browser signaling integration test.
- [ ] NAT/firewall test with TURN forced.
- [ ] Concurrent call load test.
- [ ] Call termination/shutdown cleanup test.
- [ ] Verify whatsmeow remains event/reject-only while its flag is off.

---

## References

- Meta Calling API overview:
  <https://developers.facebook.com/docs/whatsapp/cloud-api/calling>
- Business-initiated calls:
  <https://developers.facebook.com/documentation/business-messaging/whatsapp/calling/business-initiated-calls>
- User-initiated calls:
  <https://developers.facebook.com/docs/whatsapp/cloud-api/calling/user-initiated-calls/>
- Calling API reference/SDP:
  <https://developers.facebook.com/docs/whatsapp/cloud-api/calling/reference/>
- Calling FAQ (ICE-lite, no Meta TURN):
  <https://developers.facebook.com/documentation/business-messaging/whatsapp/calling/faq>
- Integration examples (FreeSWITCH + Janus):
  <https://developers.facebook.com/docs/whatsapp/cloud-api/calling/integration-examples/>

## Current code pointers

- `pkg/cloud/calling/service.go` — control plane and webhook persistence.
- `pkg/cloud/calling/handler.go` — `/cloud/calling/*` routes.
- `pkg/cloud/service.go` — `GraphRequest`, phone limiter and webhook hooks.
- `pkg/cloud/webhook.go` — signed WABA webhook and the `calls` field.
- `pkg/whatsmeow/service/whatsmeow.go` — call events and auto-reject.
- `pkg/call/service` — current `RejectCall`-only call API.
