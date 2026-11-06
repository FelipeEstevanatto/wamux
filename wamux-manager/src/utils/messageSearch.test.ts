import { describe, expect, it } from 'vitest';

import type { HistoryMessage } from '@/types/messages';
import { highlightParts, searchTarget, snippetAround } from './messageSearch';

describe('snippetAround', () => {
  it('keeps short text whole', () => {
    expect(snippetAround('hello  there\nfriend', 'there')).toBe('hello there friend');
  });

  it('centers on a match deep in a long message', () => {
    const text = `${'a'.repeat(300)} needle ${'b'.repeat(300)}`;
    const snippet = snippetAround(text, 'NEEDLE');

    expect(snippet).toContain('needle');
    expect(snippet.startsWith('…')).toBe(true);
    expect(snippet.endsWith('…')).toBe(true);
    expect(snippet.length).toBeLessThan(150);
  });

  it('falls back to the start when the text does not match', () => {
    expect(snippetAround('x'.repeat(500), 'nope')).toHaveLength(120);
  });

  it('treats regex characters as text', () => {
    expect(snippetAround('cost (usd) 5', '(usd)')).toBe('cost (usd) 5');
  });
});

describe('highlightParts', () => {
  it('puts matches at odd positions, ignoring case', () => {
    expect(highlightParts('Hello hello', 'hello')).toEqual(['', 'Hello', ' ', 'hello', '']);
  });

  it('treats regex characters as text', () => {
    expect(highlightParts('a.b axb', 'a.b')).toEqual(['', 'a.b', ' axb']);
  });

  it('returns the text alone for an empty query', () => {
    expect(highlightParts('text', '  ')).toEqual(['text']);
  });
});

describe('searchTarget', () => {
  const message = (overrides: Partial<HistoryMessage>) =>
    ({ message_id: 'm1', message_type: 'text', quoted_message_id: '', ...overrides }) as HistoryMessage;

  it('opens a normal message', () => {
    expect(searchTarget(message({}))).toBe('m1');
  });

  it('opens a reply, not what it quotes', () => {
    expect(searchTarget(message({ quoted_message_id: 'm0' }))).toBe('m1');
  });

  it('opens the original of an edit', () => {
    expect(searchTarget(message({ message_type: 'edit', quoted_message_id: 'm0' }))).toBe('m0');
  });

  it('opens an edit whose original is unknown', () => {
    expect(searchTarget(message({ message_type: 'edit' }))).toBe('m1');
  });
});
