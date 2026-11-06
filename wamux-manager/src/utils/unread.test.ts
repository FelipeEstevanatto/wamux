import { describe, expect, it } from 'vitest';
import type { HistoryMessage } from '@/types/messages';
import { unreadLabel, unreadMessages } from './unread';

const row = (over: Partial<HistoryMessage>) =>
  ({
    message_id: 'A',
    timestamp: '2026-01-01 10:00:00',
    message_type: 'text',
    is_from_me: false,
    ...over,
  }) as HistoryMessage;

describe('unreadMessages', () => {
  const list = [
    row({ message_id: 'read', timestamp: '2026-01-01 09:00:00' }),
    row({ message_id: 'mine', is_from_me: true }),
    row({ message_id: 'reaction', message_type: 'reaction' }),
    row({ message_id: 'gone', message_type: 'deleted' }),
    row({ message_id: 'first', timestamp: '2026-01-01 10:01:00' }),
    row({ message_id: 'second', timestamp: '2026-01-01 10:02:00', message_type: 'image' }),
  ];

  it('keeps only new inbound messages', () => {
    expect(unreadMessages(list, '2026-01-01 09:30:00').map((m) => m.message_id)).toEqual([
      'first',
      'second',
    ]);
  });

  it('treats a chat never read as all unread', () => {
    expect(unreadMessages(list, '')[0].message_id).toBe('read');
  });

  it('finds nothing once everything is read', () => {
    expect(unreadMessages(list, '2026-01-02 00:00:00')).toEqual([]);
  });
});

describe('unreadLabel', () => {
  it('caps the count', () => {
    expect(unreadLabel(7)).toBe('7');
    expect(unreadLabel(99)).toBe('99');
    expect(unreadLabel(100)).toBe('99+');
  });
});
