import type { InstanceEvent } from '@/types/messages';

/** The newest events kept in memory. */
export const MAX_EVENTS = 500;

const MAX_STRING_LENGTH = 1000;
const MAX_JSON_LENGTH = 32 * 1024;
const MAX_DEPTH = 32;
const MAX_SUMMARY_LENGTH = 160;

// Payloads carry the instance token and media decryption keys, and the JSON can
// be copied out of the page.
const SECRET_KEY = /token|api_?key|secret|password|mediaKey/i;

export interface FeedEvent {
  id: number;
  receivedAt: number;
  event: string;
  instanceId: string;
  instanceName: string;
  summary: string;
  json: string;
}

export type EventCategory = 'message' | 'receipt' | 'connection' | 'problem' | 'call' | 'other';

function sanitize(value: unknown, depth = 0): unknown {
  if (typeof value === 'string') {
    if (value.length <= MAX_STRING_LENGTH) return value;
    const omitted = value.length - MAX_STRING_LENGTH;
    return `${value.slice(0, MAX_STRING_LENGTH)}… [${omitted} characters omitted]`;
  }
  if (value === null || typeof value !== 'object') return value;
  if (depth >= MAX_DEPTH) return '[too deep]';
  if (Array.isArray(value)) return value.map((item) => sanitize(item, depth + 1));

  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => [
      key,
      SECRET_KEY.test(key) && item !== null && item !== ''
        ? '[hidden]'
        : sanitize(item, depth + 1),
    ])
  );
}

const asRecord = (value: unknown): Record<string, unknown> =>
  value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};

const asText = (value: unknown): string => (typeof value === 'string' ? value : '');

/** "5511...@s.whatsapp.net" and "5511...:7@s.whatsapp.net" read as "5511...". */
const shortJid = (value: unknown): string => asText(value).split(/[@:]/)[0];

function messageText(message: Record<string, unknown>): string {
  const plain = asText(message.conversation);
  if (plain) return plain;

  const extended = asText(asRecord(message.extendedTextMessage).text);
  if (extended) return extended;

  const media = Object.keys(message).find((key) => key.endsWith('Message'));
  if (!media) return '';
  const kind = media.replace(/Message$/, '');
  const caption = asText(asRecord(message[media]).caption);
  return caption ? `[${kind}] ${caption}` : `[${kind}]`;
}

function summarize(event: string, data: Record<string, unknown>): string {
  if (event === 'Message' || event === 'SendMessage') {
    const info = asRecord(data.Info);
    const who = asText(info.PushName) || shortJid(info.Sender) || shortJid(info.Chat);
    return [who, messageText(asRecord(data.Message))].filter(Boolean).join(': ');
  }
  if (event === 'Receipt') {
    const ids = Array.isArray(data.MessageIDs) ? data.MessageIDs.length : 0;
    return [asText(data.Type) || 'delivered', shortJid(data.Chat), ids > 1 ? `×${ids}` : '']
      .filter(Boolean)
      .join(' · ');
  }
  if (event === 'Presence') {
    return `${shortJid(data.From)} · ${data.Unavailable ? 'offline' : 'online'}`;
  }

  const label = asText(data.pushName) || shortJid(data.jid) || shortJid(data.From);
  return [label, shortJid(data.Chat), asText(data.status), asText(data.Reason)]
    .filter(Boolean)
    .join(' · ');
}

/** Reduces a /ws event to what the feed keeps: small, and free of secrets. */
export function toFeedEvent(source: InstanceEvent, id: number, receivedAt: number): FeedEvent {
  const raw = asRecord(sanitize(source.raw));
  const event = source.event || 'Unknown';
  const instanceId = asText(raw.instanceId);
  const instanceName = asText(raw.instanceName);

  let json = JSON.stringify(raw, null, 2);
  if (json.length > MAX_JSON_LENGTH) {
    const omitted = `Payload of ${Math.round(json.length / 1024)} KB is too large to keep`;
    json = JSON.stringify({ event, instanceId, instanceName, omitted }, null, 2);
  }

  const summary = summarize(event, asRecord(raw.data)).replace(/\s+/g, ' ');
  return {
    id,
    receivedAt,
    event,
    instanceId,
    instanceName,
    summary:
      summary.length > MAX_SUMMARY_LENGTH ? `${summary.slice(0, MAX_SUMMARY_LENGTH)}…` : summary,
    json,
  };
}

/** Adds events, given oldest first, to a newest-first list of at most `max`. */
export function prependEvents(
  current: FeedEvent[],
  incoming: FeedEvent[],
  max = MAX_EVENTS
): FeedEvent[] {
  return [...incoming].reverse().concat(current).slice(0, max);
}

export function eventCategory(event: string): EventCategory {
  if (/fail|error|loggedout|disconnected|banned|temporary/i.test(event)) return 'problem';
  if (event === 'Message' || event === 'SendMessage') return 'message';
  if (event === 'Receipt') return 'receipt';
  if (event.startsWith('Call')) return 'call';
  if (/connected|pairsuccess|qrcode/i.test(event)) return 'connection';
  return 'other';
}

export function matchesEvent(
  item: FeedEvent,
  filters: { instanceId: string; types: ReadonlySet<string>; search: string }
): boolean {
  if (filters.instanceId && item.instanceId !== filters.instanceId) return false;
  if (filters.types.size > 0 && !filters.types.has(item.event)) return false;
  const search = filters.search.trim().toLowerCase();
  return !search || item.json.toLowerCase().includes(search);
}

export const sendsToWebSocket = (websocketEnable?: string): boolean =>
  websocketEnable === 'enabled' || websocketEnable === 'true';
