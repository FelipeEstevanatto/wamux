import type { HistoryMessage } from '@/types/messages';

const SNIPPET_RADIUS = 60;

const matcher = (query: string, flags: string) =>
  new RegExp(`(${query.trim().replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, flags);

/** The part of `text` around the first match, so a long message shows why it hit. */
export function snippetAround(text: string, query: string): string {
  const flat = text.replace(/\s+/g, ' ').trim();
  const at = flat.search(matcher(query, 'i'));
  if (at < 0) return flat.slice(0, SNIPPET_RADIUS * 2);

  const start = Math.max(0, at - SNIPPET_RADIUS);
  const end = Math.min(flat.length, at + query.trim().length + SNIPPET_RADIUS);
  return `${start > 0 ? '…' : ''}${flat.slice(start, end)}${end < flat.length ? '…' : ''}`;
}

/** Splits `text` around matches. Odd positions are the matches. */
export const highlightParts = (text: string, query: string): string[] =>
  query.trim() ? text.split(matcher(query, 'gi')) : [text];

/** An edit is stored as its own row, but it shows up as the message it changed. */
export const searchTarget = (message: HistoryMessage): string =>
  message.message_type === 'edit' && message.quoted_message_id
    ? message.quoted_message_id
    : message.message_id;
