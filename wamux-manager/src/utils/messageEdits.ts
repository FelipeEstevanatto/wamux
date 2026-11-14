import type { HistoryMessage } from '@/types/messages';

/**
 * Folds edits into the messages they change, as WhatsApp shows them. History keeps
 * an edit as its own row pointing at the original through quoted_message_id, so the
 * row disappears here and the original takes its text and an `edited` mark.
 *
 * An edit whose original is not loaded (an older page, or never stored) stays
 * visible on its own, so the new text is not lost.
 *
 * A revoke is also its own row. The message it removed already shows as deleted, so
 * the row has nothing to add, and an edit cannot bring that message's text back.
 */
export function applyEdits(rows: HistoryMessage[]): HistoryMessage[] {
  const messages = rows.some((m) => m.message_type === 'delete')
    ? rows.filter((m) => m.message_type !== 'delete')
    : rows;
  if (!messages.some((m) => m.message_type === 'edit')) return messages;
  const byId = new Map(messages.map((m) => [m.message_id, m]));
  // Only the original author may change a message. A protocol edit carries its
  // sender; applying it to another author's target would let a contact rewrite
  // text that looks like it came from someone else.
  const sameAuthor = (edit: HistoryMessage, target: HistoryMessage) =>
    edit.is_from_me === target.is_from_me &&
    (edit.is_from_me || edit.sender_jid === target.sender_jid);
  const latest = new Map<string, HistoryMessage>();
  for (const row of messages) {
    if (row.message_type !== 'edit' || !row.quoted_message_id) continue;
    const target = byId.get(row.quoted_message_id);
    if (target && !sameAuthor(row, target)) continue;
    const seen = latest.get(row.quoted_message_id);
    if (!seen || row.timestamp >= seen.timestamp) latest.set(row.quoted_message_id, row);
  }
  const present = new Set(messages.map((m) => m.message_id));
  const authorized = new Set(Array.from(latest.values(), (m) => m.message_id));
  const out: HistoryMessage[] = [];
  for (const row of messages) {
    if (row.message_type === 'edit') {
      // A set target always owns the display; a rejected or superseded edit
      // must not reappear as a standalone bubble.
      if (row.quoted_message_id && present.has(row.quoted_message_id)) continue;
      if (!authorized.has(row.message_id)) continue;
      if (!row.text_content) continue; // nothing to show: saved before edits decrypted
      out.push({ ...row, message_type: 'text', quoted_message_id: '', edited: true });
      continue;
    }
    const edit = row.message_type === 'deleted' ? undefined : latest.get(row.message_id);
    out.push(
      edit
        ? { ...row, text_content: edit.text_content || row.text_content, edited: true }
        : row
    );
  }
  return out;
}
