# Inbound media integrity

When an inbound message carries `FileLength` and/or `FileSHA256`, WaMux checks the
downloaded plaintext against both before storage:

- size must equal `FileLength` (when present);
- SHA-256 must equal `FileSHA256` (when present).

Sticker bytes are checked **before** the WebP-to-PNG conversion, so the digest
covers what WhatsApp actually sent.

On a mismatch the event carries `mediaError`, the bytes are not written to local
or S3 storage, and no `mediaUrl` is advertised. The message row still exists, so
history shows the message without a broken attachment. This prevents a truncated
or mismatched file from being served forever by `/chat/media/:id` and previews.

Metadata that the sender omitted is not treated as a mismatch; only the checks
that can be performed are applied.
