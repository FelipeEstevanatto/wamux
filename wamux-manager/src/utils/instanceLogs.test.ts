import { describe, expect, it } from 'vitest';

import {
  countLevels,
  isQrDrawing,
  levelFilter,
  LOG_LEVELS,
  logTime,
  toLogRows,
  type LogEntry,
  type LogLevel,
} from './instanceLogs';

const entry = (message: string, overrides: Partial<LogEntry> = {}): LogEntry => ({
  timestamp: '2026-01-02T03:04:05Z',
  level: 'INFO',
  message,
  ...overrides,
});

describe('toLogRows', () => {
  it('drops the instance prefix', () => {
    const [row] = toLogRows([entry('[inst] Connected')], 'inst');
    expect(row.text).toBe('Connected');
  });

  it('prefers the id on the entry, and keeps other prefixes', () => {
    expect(toLogRows([entry('[a] x', { instance_id: 'a' })], 'b')[0].text).toBe('x');
    expect(toLogRows([entry('[other] x')], 'inst')[0].text).toBe('[other] x');
  });

  it('turns ===== lines into headings, keeping what follows', () => {
    const [row] = toLogRows([entry('[inst] ===== WEBHOOK SENT ===== url=x')], 'inst');
    expect(row.banner).toBe(true);
    expect(row.text).toBe('WEBHOOK SENT url=x');
  });

  it('marks long and many-line messages', () => {
    const rows = toLogRows(
      [entry('short'), entry('x'.repeat(401)), entry('1\n2\n3'), entry('1\n2\n3\n4')],
      'inst'
    );
    expect(rows.map((row) => row.long)).toEqual([false, true, false, true]);
  });

  it('treats an unknown level as INFO', () => {
    expect(toLogRows([entry('x', { level: 'TRACE' })], 'i')[0].level).toBe('INFO');
  });

  it('gives identical lines different keys', () => {
    const rows = toLogRows([entry('same'), entry('same')], 'i');
    expect(rows[0].key).not.toBe(rows[1].key);
  });

  it('keeps a key when lines are added above', () => {
    const before = toLogRows([entry('old')], 'i')[0].key;
    const after = toLogRows([entry('new'), entry('old')], 'i')[1].key;
    expect(after).toBe(before);
  });

  it('survives a missing message', () => {
    const broken = { timestamp: '', level: 'INFO' } as LogEntry;
    expect(toLogRows([broken], 'i')[0].text).toBe('');
  });
});

describe('countLevels', () => {
  it('counts each level', () => {
    const rows = toLogRows([entry('a', { level: 'ERROR' }), entry('b', { level: 'ERROR' }), entry('c')], 'i');
    expect(countLevels(rows)).toEqual({ ERROR: 2, WARN: 0, INFO: 1, DEBUG: 0 });
  });
});

describe('logTime', () => {
  const now = new Date(2026, 0, 2, 12);

  it('shows only the time for today', () => {
    const value = new Date(2026, 0, 2, 3, 4, 5).toISOString();
    expect(logTime(value, 'pt-BR', now)).toBe('03:04:05');
  });

  it('adds the date for another day', () => {
    const value = new Date(2026, 0, 1, 3, 4, 5).toISOString();
    expect(logTime(value, 'pt-BR', now)).toBe('01/01 03:04:05');
  });

  it('returns an unreadable value as it is', () => {
    expect(logTime('not a date', 'pt-BR', now)).toBe('not a date');
  });
});

describe('levelFilter', () => {
  it('sends nothing when every level is on, or none', () => {
    expect(levelFilter(new Set(LOG_LEVELS))).toBeUndefined();
    expect(levelFilter(new Set())).toBeUndefined();
  });

  it('lists the chosen levels in a stable order', () => {
    expect(levelFilter(new Set<LogLevel>(['WARN', 'ERROR']))).toBe('ERROR,WARN');
  });
});

describe('isQrDrawing', () => {
  it('finds a drawing', () => {
    expect(isQrDrawing('Instance a, QR code #1\n████ ▄▄▀▀ █\n')).toBe(true);
  });

  it('leaves other lines alone', () => {
    expect(isQrDrawing('QR code generated #1')).toBe(false);
    expect(isQrDrawing('first line\nsecond line')).toBe(false);
  });
});
