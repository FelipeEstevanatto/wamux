import { describe, expect, it } from 'vitest';
import type { HistoryMessage } from '@/types/messages';
import { applyEdits } from './messageEdits';

const row = (over: Partial<HistoryMessage>): HistoryMessage =>
  ({
    id: over.message_id ?? 'id',
    message_id: 'A',
    instance_id: 'i',
    timestamp: '2026-01-01 10:00:00',
    status: 'Received',
    source: '1',
    chat_jid: 'c',
    sender_jid: 's',
    message_type: 'text',
    text_content: 'original',
    media_url: '',
    media_mimetype: '',
    quoted_message_id: '',
    is_from_me: false,
    ...over,
  }) as HistoryMessage;

const edit = (id: string, target: string, text: string, timestamp = '2026-01-01 10:01:00') =>
  row({ message_id: id, message_type: 'edit', text_content: text, quoted_message_id: target, timestamp });

describe('applyEdits', () => {
  it('returns the same list when nothing was edited', () => {
    const list = [row({})];
    expect(applyEdits(list)).toBe(list);
  });

  it('moves the new text onto the original and hides the edit row', () => {
    const got = applyEdits([edit('E', 'A', 'new'), row({})]);
    expect(got).toHaveLength(1);
    expect(got[0]).toMatchObject({ message_id: 'A', text_content: 'new', edited: true });
  });

  it('uses the latest of several edits, in any order', () => {
    const got = applyEdits([
      edit('E2', 'A', 'second', '2026-01-01 10:02:00'),
      edit('E1', 'A', 'first', '2026-01-01 10:01:00'),
      row({}),
    ]);
    expect(got[0].text_content).toBe('second');
  });

  it('keeps an edit visible while its original is not loaded', () => {
    const got = applyEdits([edit('E', 'missing', 'new')]);
    expect(got[0]).toMatchObject({
      message_id: 'E',
      message_type: 'text',
      text_content: 'new',
      quoted_message_id: '',
      edited: true,
    });
  });

  it('drops an empty edit but keeps the original text', () => {
    const got = applyEdits([edit('E', 'A', ''), row({})]);
    expect(got[0]).toMatchObject({ text_content: 'original', edited: true });
    expect(applyEdits([edit('E', 'missing', '')])).toHaveLength(0);
  });

  it('leaves other messages alone', () => {
    const other = row({ message_id: 'B', text_content: 'other' });
    const got = applyEdits([edit('E', 'A', 'new'), row({}), other]);
    expect(got.find((m) => m.message_id === 'B')).toBe(other);
  });
});
