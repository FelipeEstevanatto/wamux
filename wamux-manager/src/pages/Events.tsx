import { memo, useDeferredValue, useEffect, useMemo, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { Check, ChevronRight, Copy, Pause, Play, Trash2 } from 'lucide-react';
import { toast } from 'sonner';

import { Alert, AlertDescription, AlertTitle, Button, Input } from '@/components/ui';
import useInstanceEvents, { ALL_INSTANCES } from '@/hooks/useInstanceEvents';
import useInstances from '@/hooks/useInstances';
import { useI18n } from '@/i18n/I18nContext';
import { cn } from '@/utils/cn';
import {
  eventCategory,
  matchesEvent,
  MAX_EVENTS,
  prependEvents,
  sendsToWebSocket,
  toFeedEvent,
  type EventCategory,
  type FeedEvent,
} from '@/utils/instanceEvents';

// A busy instance can send many events a second, so they are drawn in batches.
const FLUSH_INTERVAL_MS = 250;

const categoryClass: Record<EventCategory, string> = {
  message: 'bg-primary/10 text-primary',
  receipt: 'bg-muted text-muted-foreground',
  connection: 'bg-green-500/10 text-green-700 dark:text-green-400',
  problem: 'bg-destructive/10 text-red-700 dark:text-red-400',
  call: 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400',
  other: 'bg-accent text-accent-foreground',
};

const EventRow = memo(function EventRow({
  item,
  showInstance,
}: {
  item: FeedEvent;
  showInstance: boolean;
}) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(item.json);
      setCopied(true);
      toast.success(t('events.copied'));
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error(t('events.copyError'));
    }
  };

  return (
    <li className="border-b border-border last:border-b-0">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-accent/50"
      >
        <ChevronRight
          className={cn(
            'h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform',
            open && 'rotate-90'
          )}
        />
        <time className="shrink-0 font-mono text-xs text-muted-foreground">
          {new Date(item.receivedAt).toLocaleTimeString([], { hour12: false })}
        </time>
        <span
          className={cn(
            'shrink-0 rounded px-1.5 py-0.5 text-xs font-medium',
            categoryClass[eventCategory(item.event)]
          )}
        >
          {item.event}
        </span>
        {showInstance && item.instanceName && (
          <span className="hidden max-w-40 shrink-0 truncate text-xs text-muted-foreground sm:block">
            {item.instanceName}
          </span>
        )}
        <span className="min-w-0 flex-1 truncate text-foreground">{item.summary}</span>
      </button>
      {open && (
        <div className="relative border-t border-border bg-muted/30">
          <Button
            variant="outline"
            size="sm"
            onClick={copy}
            className="absolute top-2 right-3 gap-1.5"
          >
            {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
            {t('events.copy')}
          </Button>
          <pre className="max-h-96 overflow-auto p-3 pr-32 font-mono text-xs break-words whitespace-pre-wrap text-foreground">
            {item.json}
          </pre>
        </div>
      )}
    </li>
  );
});

function Events() {
  const { t } = useI18n();
  const { instances } = useInstances();

  const [events, setEvents] = useState<FeedEvent[]>([]);
  const [paused, setPaused] = useState(false);
  const [waiting, setWaiting] = useState(0);
  const [instanceId, setInstanceId] = useState('');
  const [types, setTypes] = useState<ReadonlySet<string>>(new Set());
  const [search, setSearch] = useState('');
  const deferredSearch = useDeferredValue(search);

  const buffer = useRef<FeedEvent[]>([]);
  const nextId = useRef(1);

  const status = useInstanceEvents(ALL_INSTANCES, (event) => {
    buffer.current.push(toFeedEvent(event, nextId.current++, Date.now()));
    if (buffer.current.length > MAX_EVENTS) buffer.current.shift();
  });

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (paused) {
        setWaiting(buffer.current.length);
      } else if (buffer.current.length > 0) {
        const incoming = buffer.current;
        buffer.current = [];
        setEvents((current) => prependEvents(current, incoming));
      }
    }, FLUSH_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [paused]);

  const togglePaused = () => {
    setPaused((value) => !value);
    setWaiting(0);
  };

  const clear = () => {
    buffer.current = [];
    setEvents([]);
    setWaiting(0);
    setTypes(new Set());
  };

  const toggleType = (type: string) => {
    setTypes((current) => {
      const next = new Set(current);
      if (!next.delete(type)) next.add(type);
      return next;
    });
  };

  // Counts follow the instance filter, so a chip matches what clicking it shows.
  const typeCounts = useMemo(() => {
    const counts = new Map<string, number>();
    for (const item of events) {
      if (instanceId && item.instanceId !== instanceId) continue;
      counts.set(item.event, (counts.get(item.event) ?? 0) + 1);
    }
    return [...counts].sort(([a], [b]) => a.localeCompare(b));
  }, [events, instanceId]);

  const visible = useMemo(
    () =>
      events.filter((item) => matchesEvent(item, { instanceId, types, search: deferredSearch })),
    [events, instanceId, types, deferredSearch]
  );

  const silent = instances.filter((i) => !sendsToWebSocket(i.websocketEnable));

  return (
    <div className="flex h-full min-h-0 flex-col gap-3 p-4 sm:p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-2xl font-bold text-foreground">{t('events.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('events.subtitle')}</p>
        </div>
        <div className="flex items-center gap-2">
          <span role="status" className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span
              className={cn(
                'h-2 w-2 rounded-full',
                status === 'open' ? 'bg-green-500' : 'bg-yellow-500'
              )}
            />
            {status === 'open' ? t('events.live') : t('events.connecting')}
          </span>
          <Button variant="outline" size="sm" onClick={togglePaused} className="gap-1.5">
            {paused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
            {!paused
              ? t('events.pause')
              : waiting > 0
                ? t('events.resumeCount', { count: waiting })
                : t('events.resume')}
          </Button>
          <Button variant="outline" size="sm" onClick={clear} className="gap-1.5">
            <Trash2 className="h-3.5 w-3.5" />
            {t('events.clear')}
          </Button>
        </div>
      </div>

      {silent.length > 0 && (
        <Alert>
          <AlertTitle>{t('events.wsDisabledTitle')}</AlertTitle>
          <AlertDescription>
            <p>{t('events.wsDisabledBody')}</p>
            <ul className="mt-1 flex flex-wrap gap-x-3 gap-y-1">
              {silent.map((i) => (
                <li key={i.id}>
                  <Link
                    to={`/manager/instances/${i.id}/settings`}
                    className="text-primary underline underline-offset-2"
                  >
                    {i.instanceName}
                  </Link>
                </li>
              ))}
            </ul>
          </AlertDescription>
        </Alert>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <select
          value={instanceId}
          onChange={(e) => setInstanceId(e.target.value)}
          aria-label={t('events.instanceFilter')}
          className="h-9 min-w-0 rounded-md border border-input bg-background px-3 text-sm"
        >
          <option value="">{t('events.allInstances')}</option>
          {instances.map((i) => (
            <option key={i.id} value={i.id}>
              {i.instanceName}
            </option>
          ))}
        </select>
        <Input
          type="search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t('events.search')}
          aria-label={t('events.search')}
          className="h-9 min-w-48 flex-1 sm:max-w-sm"
        />
        <span className="text-xs text-muted-foreground">
          {t('events.showing', { shown: visible.length, total: events.length })}
        </span>
      </div>

      {typeCounts.length > 0 && (
        <div role="group" aria-label={t('events.typeFilter')} className="flex flex-wrap gap-1.5">
          {typeCounts.map(([type, count]) => (
            <button
              key={type}
              type="button"
              aria-pressed={types.has(type)}
              onClick={() => toggleType(type)}
              className={cn(
                'rounded-full border px-2.5 py-0.5 text-xs transition-colors',
                types.has(type)
                  ? 'border-primary bg-primary/10 text-primary'
                  : 'border-border text-muted-foreground hover:bg-accent'
              )}
            >
              {type} <span className="opacity-70">{count}</span>
            </button>
          ))}
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto rounded-xl border border-sidebar-border bg-sidebar">
        {visible.length === 0 ? (
          <p className="p-8 text-center text-sm text-muted-foreground">
            {events.length === 0 ? t('events.waiting') : t('events.noMatch')}
          </p>
        ) : (
          <ul>
            {visible.map((item) => (
              <EventRow key={item.id} item={item} showInstance={!instanceId} />
            ))}
          </ul>
        )}
      </div>

      <p className="text-xs text-muted-foreground">{t('events.redacted')}</p>
    </div>
  );
}

export default Events;
