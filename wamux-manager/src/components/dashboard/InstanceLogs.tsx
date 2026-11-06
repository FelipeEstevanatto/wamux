import { memo, useDeferredValue, useEffect, useMemo, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { RefreshCw, X } from 'lucide-react';

import { Button, Input } from '@/components/ui';
import { useI18n } from '@/i18n/I18nContext';
import { fetchInstanceLogs } from '@/services/api/instances';
import { cn } from '@/utils/cn';
import {
  countLevels,
  isQrDrawing,
  levelFilter,
  LOG_LEVELS,
  logTime,
  toLogRows,
  type LogLevel,
  type LogRow,
} from '@/utils/instanceLogs';
import { highlightParts } from '@/utils/messageSearch';

const LIVE_INTERVAL_MS = 5000;
const LIMITS = [100, 500, 1000];

const levelBadge: Record<LogLevel, string> = {
  ERROR: 'text-red-600 dark:text-red-400',
  WARN: 'text-amber-600 dark:text-amber-400',
  INFO: 'text-blue-600 dark:text-blue-400',
  DEBUG: 'text-muted-foreground',
};

const levelStripe: Record<LogLevel, string> = {
  ERROR: 'border-l-red-500 bg-red-500/5',
  WARN: 'border-l-amber-500',
  INFO: 'border-l-blue-500',
  DEBUG: 'border-l-border',
};

const levelText: Partial<Record<LogLevel, string>> = {
  ERROR: 'text-red-700 dark:text-red-300',
  WARN: 'text-amber-700 dark:text-amber-300',
};

// Log text can hold contact names and message content, so it is only ever
// rendered as text.
const LogLine = memo(function LogLine({
  row,
  query,
  locale,
  expandLabel,
}: {
  row: LogRow;
  query: string;
  locale: string;
  expandLabel: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const parts = highlightParts(row.text, query).map((part, index) =>
    index % 2 === 1 ? (
      <mark key={index} className="rounded-sm bg-primary/25 text-foreground">
        {part}
      </mark>
    ) : (
      part
    )
  );

  // On a phone the message takes its own line, or it gets a sliver of the width.
  // A heading keeps the color of a warning or error.
  const messageClass = cn(
    'col-span-2 min-w-0 break-words whitespace-pre-wrap sm:col-span-1',
    levelText[row.level] ?? (row.banner && 'text-primary'),
    row.banner && 'font-semibold'
  );

  return (
    <div
      className={cn(
        'grid grid-cols-[auto_minmax(0,1fr)] gap-x-2 border-b border-l-2 border-b-border px-2 py-1 font-mono text-xs last:border-b-0 sm:grid-cols-[auto_3.25rem_minmax(0,1fr)]',
        levelStripe[row.level]
      )}
    >
      <time className="whitespace-nowrap text-muted-foreground" title={row.timestamp}>
        {logTime(row.timestamp, locale)}
      </time>
      <span className={cn('font-semibold', levelBadge[row.level])}>{row.level}</span>
      {isQrDrawing(row.text) ? (
        // Black behind the blocks and no line gaps, or a phone cannot read it.
        <pre className="col-span-2 w-fit max-w-full overflow-x-auto bg-black p-2 font-mono leading-none text-white sm:col-span-1">
          {row.text}
        </pre>
      ) : row.long ? (
        <button
          type="button"
          aria-expanded={expanded}
          title={expandLabel}
          onClick={() => setExpanded((value) => !value)}
          className={cn(
            messageClass,
            'text-left',
            !expanded && 'line-clamp-3'
          )}
        >
          {parts}
        </button>
      ) : (
        <span className={messageClass}>{parts}</span>
      )}
    </div>
  );
});

/**
 * The selected instance's logs. Levels are filtered on the server, so a rare ERROR
 * is found however far back it is; text is filtered on what was loaded.
 */
function InstanceLogs({
  instanceId,
  instanceName,
  onClose,
}: {
  instanceId: string;
  instanceName: string;
  onClose: () => void;
}) {
  const { t, locale } = useI18n();

  const [levels, setLevels] = useState<ReadonlySet<LogLevel>>(new Set(LOG_LEVELS));
  const [limit, setLimit] = useState(LIMITS[0]);
  const [live, setLive] = useState(false);
  const [text, setText] = useState('');
  const query = useDeferredValue(text.trim());
  const panel = useRef<HTMLElement>(null);

  useEffect(() => {
    panel.current?.scrollIntoView?.({ block: 'nearest', behavior: 'smooth' });
  }, [instanceId]);

  const level = levelFilter(levels);
  const logs = useQuery({
    queryKey: ['instance-logs', instanceId, limit, level],
    queryFn: () => fetchInstanceLogs(instanceId, { limit, level }),
    refetchInterval: live ? LIVE_INTERVAL_MS : false,
    // Keep lines on screen while a filter reloads, but never another instance's.
    placeholderData: (previous, previousQuery) =>
      previousQuery?.queryKey[1] === instanceId ? previous : undefined,
    gcTime: 0,
  });

  const rows = useMemo(() => toLogRows(logs.data ?? [], instanceId), [logs.data, instanceId]);
  const counts = useMemo(() => countLevels(rows), [rows]);
  const visible = useMemo(() => {
    const needle = query.toLowerCase();
    return needle ? rows.filter((row) => row.text.toLowerCase().includes(needle)) : rows;
  }, [rows, query]);

  const toggleLevel = (target: LogLevel) => {
    setLevels((current) => {
      const next = new Set(current);
      if (!next.delete(target)) next.add(target);
      return next.size === 0 ? new Set(LOG_LEVELS) : next;
    });
  };

  const error = logs.error as { message?: string } | null;
  const empty = logs.isLoading
    ? t('common.loading')
    : error
      ? t('dashboard.logsLoadError', { error: error.message ?? '' })
      : rows.length === 0
        ? t('dashboard.logsEmpty')
        : t('dashboard.logsNoMatch');

  return (
    <section
      ref={panel}
      aria-label={t('dashboard.logsOf', { name: instanceName })}
      className="mt-3 space-y-2 border-t border-sidebar-border pt-3"
    >
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="min-w-0 truncate text-sm font-semibold text-foreground">
          {t('dashboard.logsOf', { name: instanceName })}
        </h3>
        <span className="flex-1" />
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <input type="checkbox" checked={live} onChange={(e) => setLive(e.target.checked)} />
          {t('dashboard.logsLive')}
        </label>
        <select
          value={limit}
          onChange={(e) => setLimit(Number(e.target.value))}
          aria-label={t('dashboard.logsLimit')}
          className="h-8 rounded-md border border-input bg-background px-2 text-xs"
        >
          {LIMITS.map((value) => (
            <option key={value} value={value}>
              {t('dashboard.logsLines', { count: value })}
            </option>
          ))}
        </select>
        <Button
          variant="outline"
          size="icon"
          onClick={() => void logs.refetch()}
          title={t('dashboard.refresh')}
          aria-label={t('dashboard.refresh')}
        >
          <RefreshCw className={logs.isFetching ? 'animate-spin' : ''} />
        </Button>
        <Button
          variant="outline"
          size="icon"
          onClick={onClose}
          title={t('common.close')}
          aria-label={t('common.close')}
        >
          <X />
        </Button>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div role="group" aria-label={t('dashboard.logsLevels')} className="flex flex-wrap gap-1.5">
          {LOG_LEVELS.map((value) => (
            <button
              key={value}
              type="button"
              aria-pressed={levels.has(value)}
              onClick={() => toggleLevel(value)}
              className={cn(
                'rounded-full border px-2.5 py-0.5 text-xs font-medium transition-opacity',
                levelBadge[value],
                levels.has(value) ? 'border-current' : 'border-border opacity-50'
              )}
            >
              {value} {counts[value] > 0 && <span className="opacity-70">{counts[value]}</span>}
            </button>
          ))}
        </div>
        <Input
          type="search"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder={t('dashboard.logsSearch')}
          aria-label={t('dashboard.logsSearch')}
          className="h-8 min-w-40 flex-1 sm:max-w-xs"
        />
        <span className="text-xs text-muted-foreground">
          {visible.length === rows.length
            ? t('dashboard.logsCount', { count: rows.length })
            : t('dashboard.logsCountFiltered', { shown: visible.length, total: rows.length })}
        </span>
      </div>

      <div
        role="log"
        className="max-h-[32rem] overflow-y-auto rounded-md border border-sidebar-border bg-background"
      >
        {visible.length === 0 ? (
          <p className="p-6 text-center text-sm text-muted-foreground">{empty}</p>
        ) : (
          visible.map((row) => (
            <LogLine
              key={row.key}
              row={row}
              query={query}
              locale={locale}
              expandLabel={t('dashboard.logsExpand')}
            />
          ))
        )}
      </div>
    </section>
  );
}

export default InstanceLogs;
