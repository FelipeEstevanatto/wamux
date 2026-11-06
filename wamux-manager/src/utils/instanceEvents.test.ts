import { describe, expect, it } from 'vitest';

import type { InstanceEvent } from '@/types/messages';
import {
  eventCategory,
  matchesEvent,
  prependEvents,
  sendsToWebSocket,
  toFeedEvent,
  type FeedEvent,
} from './instanceEvents';

const source = (event: string, data: Record<string, unknown>, extra = {}): InstanceEvent => {
  const raw = { event, data, instanceId: 'i1', instanceName: 'One', ...extra };
  return { queue: 'q', event, data, raw };
};

const feed = (event: string, data: Record<string, unknown>, extra = {}) =>
  toFeedEvent(source(event, data, extra), 1, 1000);

const item = (id: number, overrides: Partial<FeedEvent> = {}): FeedEvent => ({
  id,
  receivedAt: id,
  event: 'Message',
  instanceId: 'i1',
  instanceName: 'One',
  summary: '',
  json: '{"text":"Hello"}',
  ...overrides,
});

describe('toFeedEvent', () => {
  it('hides tokens and keys at any depth', () => {
    const { json } = feed(
      'Message',
      { Message: { imageMessage: { mediaKey: 'media-secret-value' } }, apikey: 'api-secret-value' },
      { instanceToken: 'token-secret-value' }
    );

    expect(json).not.toContain('secret-value');
    expect(json).toContain('[hidden]');
  });

  it('cuts long strings such as base64', () => {
    const { json } = feed('Message', { Base64: 'A'.repeat(50_000) });

    expect(json.length).toBeLessThan(2_000);
    expect(json).toContain('49000 characters omitted');
  });

  it('drops a payload that stays too large after sanitizing', () => {
    const conversations = Array.from({ length: 5_000 }, (_, i) => ({ id: `chat-${i}` }));
    const { json, event } = feed('HistorySync', { conversations });

    expect(event).toBe('HistorySync');
    expect(json.length).toBeLessThan(1_000);
    expect(json).toContain('too large');
  });

  it('survives a payload nested too deeply', () => {
    let deep: Record<string, unknown> = { leaf: true };
    for (let i = 0; i < 100; i++) deep = { deep };

    expect(feed('Other', deep).json).toContain('[too deep]');
  });

  it.each([
    ['text', { conversation: 'hello' }, 'Ana: hello'],
    ['reply', { extendedTextMessage: { text: 'reply' } }, 'Ana: reply'],
    ['caption', { imageMessage: { caption: 'a photo' } }, 'Ana: [image] a photo'],
    ['no caption', { audioMessage: {} }, 'Ana: [audio]'],
    ['unknown body', {}, 'Ana'],
  ])('summarizes a message: %s', (_, message, summary) => {
    expect(feed('Message', { Info: { PushName: 'Ana' }, Message: message }).summary).toBe(summary);
  });

  it('falls back to the number for an unnamed sender', () => {
    const data = { Info: { Sender: '100:7@s.whatsapp.net' }, Message: { conversation: 'hi' } };
    expect(feed('Message', data).summary).toBe('100: hi');
  });

  it('summarizes receipts, presence and connections', () => {
    const chat = '100@s.whatsapp.net';
    expect(feed('Receipt', { Type: 'read', Chat: chat, MessageIDs: ['A', 'B'] }).summary).toBe(
      'read · 100 · ×2'
    );
    expect(feed('Receipt', { Type: '', Chat: chat, MessageIDs: ['A'] }).summary).toBe(
      'delivered · 100'
    );
    expect(feed('Presence', { From: chat, Unavailable: true }).summary).toBe('100 · offline');
    expect(feed('Connected', { pushName: 'Bot', status: 'open' }).summary).toBe('Bot · open');
    expect(feed('Disconnected', {}).summary).toBe('');
  });

  it('keeps summaries on one short line', () => {
    const { summary } = feed('Message', {
      Info: { PushName: 'Ana' },
      Message: { conversation: `line one\n\nline two ${'x'.repeat(500)}` },
    });

    expect(summary).toContain('line one line two');
    expect(summary.length).toBeLessThanOrEqual(161);
  });

  it('tolerates missing and malformed fields', () => {
    expect(toFeedEvent({ queue: '', event: '', data: {}, raw: {} }, 1, 1).event).toBe('Unknown');
    expect(feed('Message', { Info: 'bad', Message: 7 }).summary).toBe('');
    expect(feed('Receipt', { MessageIDs: 'bad' }).summary).toBe('delivered');
  });
});

describe('prependEvents', () => {
  it('puts the newest first and caps the list', () => {
    const merged = prependEvents([item(2), item(1)], [item(3), item(4)], 3);
    expect(merged.map((e) => e.id)).toEqual([4, 3, 2]);
  });
});

describe('eventCategory', () => {
  it.each([
    ['Message', 'message'],
    ['SendMessage', 'message'],
    ['Receipt', 'receipt'],
    ['Connected', 'connection'],
    ['PairSuccess', 'connection'],
    ['Disconnected', 'problem'],
    ['LoggedOut', 'problem'],
    ['ConnectFailure', 'problem'],
    ['PairError', 'problem'],
    ['CallOffer', 'call'],
    ['Presence', 'other'],
  ])('%s is %s', (event, category) => {
    expect(eventCategory(event)).toBe(category);
  });
});

describe('matchesEvent', () => {
  const none = { instanceId: '', types: new Set<string>(), search: '' };

  it('matches everything with no filters', () => {
    expect(matchesEvent(item(1), none)).toBe(true);
  });

  it('filters by instance, type and text together', () => {
    const target = item(1);
    expect(matchesEvent(target, { ...none, instanceId: 'i2' })).toBe(false);
    expect(matchesEvent(target, { ...none, types: new Set(['Receipt']) })).toBe(false);
    expect(matchesEvent(target, { ...none, search: ' hello ' })).toBe(true);
    expect(matchesEvent(target, { ...none, search: 'nope' })).toBe(false);
  });
});

describe('sendsToWebSocket', () => {
  it('accepts the two values the server treats as on', () => {
    expect(sendsToWebSocket('enabled')).toBe(true);
    expect(sendsToWebSocket('true')).toBe(true);
    expect(sendsToWebSocket('')).toBe(false);
    expect(sendsToWebSocket(undefined)).toBe(false);
  });
});
