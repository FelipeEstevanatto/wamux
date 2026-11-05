import type { HistoryMessage } from '@/types/messages';

/**
 * Folds edits into the messages they change, as WhatsApp shows them. History keeps
 * an edit as its own row pointing at the original through quoted_message_id, so the
 * row disappears here and the original takes its text and an `edited` mark.
 *
 * An edit whose original is not loaded (an older page, or never stored) stays
 * visible on its own, so the new text is not lost.
 */
export function applyEdits(messages: HistoryMessage[]): HistoryMessage[] {
  const latest = new Map<string, HistoryMessage>();
  for (const row of messages) {
    if (row.message_type !== 'edit' || !row.quoted_message_id) continue;
    const seen = latest.get(row.quoted_message_id);
    if (!seen || row.timestamp >= seen.timestamp) latest.set(row.quoted_message_id, row);
  }
  if (latest.size === 0) return messages;

  const present = new Set(messages.map((m) => m.message_id));
  const out: HistoryMessage[] = [];
  for (const row of messages) {
    if (row.message_type === 'edit') {
      if (row.quoted_message_id && present.has(row.quoted_message_id)) continue;
      if (!row.text_content) continue; // nothing to show: saved before edits decrypted
      out.push({ ...row, message_type: 'text', quoted_message_id: '', edited: true });
      continue;
    }
    const edit = latest.get(row.message_id);
    out.push(
      edit
        ? { ...row, text_content: edit.text_content || row.text_content, edited: true }
        : row
    );
  }
  return out;
}
