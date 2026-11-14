# Chat state from WhatsApp

`chat_states` keeps the account's own chat state, separate from this manager's
read position (`chat_reads`):

- `archived`
- `pinned`
- `muted_until` (Unix seconds; `-1` means muted forever, `0` means unmuted)
- `marked_unread`

Sources:

- `Archive`, `Pin`, `Mute` and `MarkChatAsRead` app-state events;
- history-sync `Conversation` fields;
- the pin/archive/mute/unmark API actions, after WhatsApp accepts them.

Updates are partial: an archive event only writes `archived`, and a mute event
only writes `muted_until`. `GET /chat/chats` returns the fields and the manager
shows pin/archive/mute badges. A stale `muted_until` timestamp is ignored by the
UI; it does not need a cleanup job.

The table is additive (migration `0015`). Existing chats start as not archived,
not pinned, unmuted and not explicitly unread.
