export const LOG_LEVELS = ['ERROR', 'WARN', 'INFO', 'DEBUG'] as const;
export type LogLevel = (typeof LOG_LEVELS)[number];

export interface LogEntry {
  timestamp: string;
  level: string;
  instance_id?: string;
  message: string;
}

export interface LogRow {
  key: string;
  level: LogLevel;
  timestamp: string;
  text: string;
  /** A "===== TITLE =====" line, shown as a heading. */
  banner: boolean;
  /** Long enough to start collapsed. */
  long: boolean;
}

const MAX_SHORT_LENGTH = 400;
const MAX_SHORT_LINES = 3;

const levelOf = (level: string): LogLevel =>
  LOG_LEVELS.includes(level as LogLevel) ? (level as LogLevel) : 'INFO';

/**
 * Prepares log entries for display. Every message starts with "[<instance id>] ",
 * which says nothing inside that instance's panel. Keys come from the content, so
 * an expanded row stays expanded when a refresh adds lines above it.
 */
export function toLogRows(entries: LogEntry[], instanceId: string): LogRow[] {
  const seen = new Map<string, number>();
  return entries.map((entry) => {
    const raw = String(entry.message ?? '');
    const prefix = `[${entry.instance_id || instanceId}] `;
    const message = raw.startsWith(prefix) ? raw.slice(prefix.length) : raw;
    const heading = message.match(/^=+\s*(.*?)\s*=+(.*)$/s);
    const text = heading ? heading[1] + heading[2] : message;

    const identity = `${entry.timestamp}|${raw}`;
    const repeat = seen.get(identity) ?? 0;
    seen.set(identity, repeat + 1);

    return {
      key: `${identity}|${repeat}`,
      level: levelOf(entry.level),
      timestamp: entry.timestamp,
      text,
      banner: Boolean(heading),
      long: text.length > MAX_SHORT_LENGTH || text.split('\n').length > MAX_SHORT_LINES,
    };
  });
}

export function countLevels(rows: LogRow[]): Record<LogLevel, number> {
  const counts: Record<LogLevel, number> = { ERROR: 0, WARN: 0, INFO: 0, DEBUG: 0 };
  for (const row of rows) counts[row.level] += 1;
  return counts;
}

/** The time alone for today, with the date for anything older. */
export function logTime(value: string, locale: string, now = new Date()): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value ?? '');
  const time = date.toLocaleTimeString(locale);
  if (date.toDateString() === now.toDateString()) return time;
  return `${date.toLocaleDateString(locale, { day: '2-digit', month: '2-digit' })} ${time}`;
}

/** The server's `level` value. Every level or none selected needs no filter. */
export function levelFilter(levels: ReadonlySet<LogLevel>): string | undefined {
  if (levels.size === 0 || levels.size === LOG_LEVELS.length) return undefined;
  return LOG_LEVELS.filter((level) => levels.has(level)).join(',');
}
