import type { HistoryMessage } from '@/types/messages';

// These change or react to a message, and never count as a new one. The server skips
// the same types in `unread_count`.
const NOT_NEW = new Set(['edit', 'delete', 'deleted', 'reaction', 'poll_update', 'unknown']);

/** The messages after the read position, oldest first, from a list in that order. */
export function unreadMessages(ordered: HistoryMessage[], readAt: string): HistoryMessage[] {
  return ordered.filter(
    (m) => !m.is_from_me && !NOT_NEW.has(m.message_type) && m.timestamp > readAt
  );
}

/** What a count badge shows: WhatsApp stops counting at 99. */
export function unreadLabel(count: number): string {
  return count > 99 ? '99+' : String(count);
}
