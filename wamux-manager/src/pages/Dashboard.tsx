import { useEffect, useMemo, useState } from 'react';
import {
  Activity,
  Cpu,
  HardDrive,
  Layers,
  Mail,
  RefreshCw,
  Server,
  Smartphone,
  Users,
} from 'lucide-react';
import useServerStats from '@/hooks/useServerStats';
import useInstances from '@/hooks/useInstances';
import ConnectionChart from '@/components/dashboard/ConnectionChart';
import InstanceLogs from '@/components/dashboard/InstanceLogs';
import InstancesTable from '@/components/dashboard/InstancesTable';
import TopConversations from '@/components/dashboard/TopConversations';
import { Button, Skeleton } from '@/components/ui';
import GithubIcon from '@/components/base/GithubIcon';
import { REPO } from '@/constants/branding';
import { useI18n } from '@/i18n/I18nContext';

// (the repository link comes from @/constants/branding)

const REFRESH_SECONDS = [0, 10, 15, 30, 60];
const DEFAULT_REFRESH_SECONDS = 15;

const fmtMB = (mb?: number) => {
  if (mb === undefined || mb === null || Number.isNaN(mb)) return '—';
  return mb >= 1024 ? `${(mb / 1024).toFixed(1)} GB` : `${Math.round(mb)} MB`;
};

const fmtUptime = (seconds?: number) => {
  const s = Math.floor(seconds || 0);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d) return `${d}d ${h}h`;
  if (h) return `${h}h ${m}m`;
  return `${m}m`;
};

function Kpi({
  icon: Icon,
  label,
  value,
  sub,
  tone = 'default',
  title,
  loading = false,
}: {
  icon: typeof Activity;
  label: string;
  value: string;
  sub?: string;
  tone?: 'default' | 'green' | 'red';
  title?: string;
  loading?: boolean;
}) {
  const valueColor =
    tone === 'green'
      ? 'text-green-500'
      : tone === 'red'
        ? 'text-red-500'
        : 'text-foreground';
  return (
    <div
      className="rounded-xl border border-sidebar-border bg-sidebar p-4"
      title={title}
    >
      <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        <Icon className="h-3.5 w-3.5" />
        {label}
      </div>
      {loading ? (
        <Skeleton className="mt-2 h-9 w-20" />
      ) : (
        <div className={`mt-2 text-3xl font-bold ${valueColor}`}>{value}</div>
      )}
      {sub && !loading && <div className="mt-1 text-xs text-muted-foreground">{sub}</div>}
    </div>
  );
}

function StorageRow({
  label,
  value,
  hint,
}: {
  label: string;
  value?: string;
  hint?: string;
}) {
  return (
    <div className="min-w-0">
      <dt className="text-xs uppercase tracking-wide text-muted-foreground">
        {label}
      </dt>
      <dd className="font-mono text-xs text-foreground">{value || '—'}</dd>
      {hint && (
        <dd className="truncate text-[11px] text-muted-foreground" title={hint}>
          {hint}
        </dd>
      )}
    </div>
  );
}

export default function Dashboard() {
  const [refreshSeconds, setRefreshSeconds] = useState(DEFAULT_REFRESH_SECONDS);
  const [selectedId, setSelectedId] = useState('');
  const { stats, error, loading, updatedAt, refetch } = useServerStats(refreshSeconds * 1000);
  const {
    instances,
    isLoading: instancesLoading,
    fetchInstances,
    overviews,
  } = useInstances(refreshSeconds * 1000);
  const { t, locale } = useI18n();
  const fmtNumber = (n?: number) =>
    n === undefined || n === null ? '—' : n.toLocaleString(locale);
  const selected = instances.find((i) => i.id === selectedId);

  useEffect(() => {
    fetchInstances();
  }, [fetchInstances]);

  const connected = instances.filter((i) => i.connected).length;
  const total = instances.length;

  // The name WhatsApp shows as the linked device (DeviceProps.Os). Only surface
  // it when every instance agrees, otherwise a single chip would be misleading.
  const deviceNames = useMemo(
    () =>
      Array.from(
        new Set(
          instances.map((i) => i.osName).filter((v): v is string => !!v)
        )
      ),
    [instances]
  );
  const deviceName = deviceNames.length === 1 ? deviceNames[0] : undefined;

  // Sum of the per-instance contact counts we already fetched for the cards.
  const contacts = useMemo(
    () =>
      Object.values(overviews).reduce(
        (sum, o) => sum + (o.contactsCount || 0),
        0
      ),
    [overviews]
  );

  const system = stats?.system || {};
  const messages = stats?.messages || {};
  const storage = stats?.storage || {};
  const diskPct = Math.min(100, Math.max(0, storage.diskUsedPct ?? 0));
  const byDay = useMemo(
    () => (messages.byDay || []).slice().reverse(),
    [messages.byDay]
  );
  const maxDay = Math.max(1, ...byDay.map((d) => d.count));

  // Load average is the average number of runnable tasks (not a percentage of a
  // single core). Express it against the CPU count so the number is meaningful.
  const cpus = system.numCpu || 0;
  const load1 = system.loadAvg1;
  const loadPct =
    cpus > 0 && load1 !== undefined
      ? Math.round((load1 / cpus) * 100)
      : undefined;
  const loadTitle =
    load1 !== undefined
      ? t('dashboard.loadTitle', { load1: load1.toFixed(2) }) +
        (loadPct !== undefined
          ? t('dashboard.loadTitleCapacity', { cpus, loadPct })
          : '') +
        (system.loadAvg5 !== undefined
          ? t('dashboard.loadTitle5m', { load5: system.loadAvg5.toFixed(2) })
          : '') +
        (system.loadAvg15 !== undefined
          ? t('dashboard.loadTitle15m', { load15: system.loadAvg15.toFixed(2) })
          : '')
      : undefined;

  return (
    <div className="h-full overflow-y-auto p-4 sm:p-6">
      <div className="mx-auto max-w-6xl space-y-6">
        {/* Header */}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-2xl font-bold text-foreground">
              {t('sidebar.dashboard')}
            </h1>
            <p className="text-sm text-muted-foreground">
              {t('dashboard.subtitle')}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {deviceName && (
              <span
                className="inline-flex items-center gap-2 rounded-md border border-sidebar-border bg-sidebar px-3 py-2 text-xs text-muted-foreground"
                title={t('dashboard.deviceNameTitle')}
              >
                <Smartphone className="h-3.5 w-3.5" />
                {deviceName}
              </span>
            )}
            <span className="inline-flex items-center gap-2 rounded-md border border-sidebar-border bg-sidebar px-3 py-2 text-xs text-muted-foreground">
              <Server className="h-3.5 w-3.5" />
              {system.version
                ? t('sidebar.version', { version: system.version })
                : t('sidebar.versionEmpty')}
            </span>
            <span
              role="status"
              className="inline-flex items-center gap-2 rounded-md border border-sidebar-border bg-sidebar px-3 py-2 text-xs text-muted-foreground"
            >
              <span className={`h-2 w-2 rounded-full ${error ? 'bg-red-500' : 'bg-green-500'}`} />
              {error ? t('dashboard.serverOffline') : t('dashboard.serverOnline')}
              {updatedAt > 0 &&
                ` · ${new Date(updatedAt).toLocaleTimeString(locale)}`}
            </span>
            <select
              value={refreshSeconds}
              onChange={(e) => setRefreshSeconds(Number(e.target.value))}
              aria-label={t('dashboard.refreshInterval')}
              className="h-9 rounded-md border border-sidebar-border bg-sidebar px-2 text-xs text-muted-foreground"
            >
              {REFRESH_SECONDS.map((seconds) => (
                <option key={seconds} value={seconds}>
                  {seconds === 0
                    ? t('dashboard.refreshManual')
                    : t('dashboard.refreshEvery', { seconds })}
                </option>
              ))}
            </select>
            <Button
              variant="outline"
              size="icon"
              onClick={() => {
                void refetch();
                void fetchInstances();
              }}
              title={t('dashboard.refresh')}
              aria-label={t('dashboard.refresh')}
            >
              <RefreshCw />
            </Button>
            <a
              href={REPO}
              target="_blank"
              rel="noreferrer noopener"
              className="inline-flex items-center gap-2 rounded-md border border-sidebar-border bg-sidebar px-3 py-2 text-xs text-muted-foreground transition-colors hover:text-foreground"
              title={t('sidebar.repoTitle')}
            >
              <GithubIcon className="h-3.5 w-3.5" />
              {t('sidebar.github')}
            </a>
          </div>
        </div>

        {error && (
          <div className="rounded-md border border-red-500/40 bg-red-500/10 p-3 text-sm text-red-400">
            {error}
          </div>
        )}

        {/* Instances + traffic */}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Kpi
            icon={Layers}
            loading={instancesLoading}
            label={t('sidebar.instances')}
            value={fmtNumber(total)}
            sub={t('dashboard.connectedCount', { count: connected })}
          />
          <Kpi
            icon={Activity}
            loading={instancesLoading}
            label={t('dashboard.kpiConnected')}
            value={fmtNumber(connected)}
            sub={
              total
                ? t('dashboard.percentOfTotal', {
                    pct: Math.round((connected / total) * 100),
                  })
                : '—'
            }
            tone="green"
          />
          <Kpi
            icon={Mail}
            loading={loading}
            label={t('sidebar.messages')}
            value={fmtNumber(messages.total)}
            sub={t('dashboard.savedInDb')}
          />
          <Kpi
            icon={Users}
            loading={instancesLoading}
            label={t('dashboard.kpiContacts')}
            value={fmtNumber(contacts)}
            sub={t('dashboard.summingConnected')}
          />
        </div>

        {/* Host / runtime */}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Kpi
            icon={HardDrive}
            loading={loading}
            label={t('dashboard.kpiHostRam')}
            value={
              system.hostMemUsedPct !== undefined
                ? `${Math.round(system.hostMemUsedPct)}%`
                : '—'
            }
            sub={
              system.hostMemTotalMB !== undefined
                ? `${fmtMB((system.hostMemTotalMB || 0) - (system.hostMemAvailableMB || 0))} / ${fmtMB(system.hostMemTotalMB)}`
                : t('dashboard.hostUnavailable')
            }
          />
          <Kpi
            icon={Cpu}
            loading={loading}
            label={t('dashboard.kpiLoad')}
            value={
              loadPct !== undefined
                ? `${loadPct}%`
                : load1 !== undefined
                  ? load1.toFixed(2)
                  : '—'
            }
            sub={
              load1 !== undefined
                ? `${load1.toFixed(2)} / ${fmtNumber(system.numCpu)} CPUs`
                : t('dashboard.loadUnavailable')
            }
            title={loadTitle}
          />
          <Kpi
            icon={Activity}
            loading={loading}
            label={t('dashboard.kpiGoroutines')}
            value={fmtNumber(system.goroutines)}
            sub={t('dashboard.heapValue', { size: fmtMB(system.heapInuseMB) })}
          />
          <Kpi
            icon={Server}
            loading={loading}
            label={t('dashboard.kpiUptime')}
            value={fmtUptime(system.uptimeSeconds)}
            sub={system.goVersion || '—'}
          />
        </div>

        {/* Storage */}
        <div className="rounded-xl border border-sidebar-border bg-sidebar p-4">
          <div className="mb-4 flex items-center justify-between">
            <h2 className="text-sm font-semibold text-foreground">
              {t('dashboard.storageTitle')}
            </h2>
            <HardDrive className="h-3.5 w-3.5 text-muted-foreground" />
          </div>
          <div className="space-y-4">
            {storage.diskTotalMB !== undefined && (
              <div>
                <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
                  <span className="text-muted-foreground">
                    {t('dashboard.disk')}{' '}
                    <span className="font-mono text-xs">
                      {storage.diskPath}
                    </span>
                  </span>
                  <span className="font-mono text-xs">
                    {fmtMB(storage.diskUsedMB)} / {fmtMB(storage.diskTotalMB)}{' '}
                    · {Math.round(storage.diskUsedPct ?? 0)}%
                  </span>
                </div>
                <div className="mt-1.5 h-2 w-full overflow-hidden rounded-full bg-muted">
                  <div
                    className={`h-full rounded-full ${diskPct >= 90 ? 'bg-red-500' : 'bg-primary'}`}
                    style={{ width: `${diskPct}%` }}
                  />
                </div>
              </div>
            )}
            <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
              <StorageRow
                label={t('dashboard.storageData')}
                hint={storage.dataDir}
                value={
                  storage.dataUsedMB !== undefined
                    ? t('dashboard.dataFilesValue', {
                        size: fmtMB(storage.dataUsedMB),
                        count: fmtNumber(storage.dataFiles),
                      })
                    : undefined
                }
              />
              <StorageRow
                label={t('dashboard.storageDatabase')}
                hint={t('dashboard.postgresql')}
                value={
                  storage.dbTotalMB !== undefined
                    ? `${fmtMB(storage.dbTotalMB)}${
                        storage.dbMessagesMB
                          ? ` · ${t('dashboard.dbMessagesLabel')} ${fmtMB(storage.dbMessagesMB)}`
                          : ''
                      }`
                    : undefined
                }
              />
              <StorageRow
                label={t('dashboard.storageMedia')}
                value={
                  storage.mediaEnabled
                    ? storage.mediaBackend
                    : t('dashboard.mediaNotConfigured')
                }
                hint={
                  storage.mediaEnabled
                    ? t('dashboard.mediaFilesHint')
                    : t('dashboard.mediaDisabledHint')
                }
              />
            </dl>
          </div>
        </div>

        <div className="grid gap-4 lg:grid-cols-3">
          <div className="rounded-xl border border-sidebar-border bg-sidebar p-4">
            <h2 className="mb-4 text-sm font-semibold text-foreground">
              {t('dashboard.connectionStatus')}
            </h2>
            <ConnectionChart connected={connected} total={total} />
          </div>

        {/* Messages per day */}
        <div className="min-w-0 rounded-xl border border-sidebar-border bg-sidebar p-4 lg:col-span-2">
          <div className="mb-4 flex items-center justify-between">
            <h2 className="text-sm font-semibold text-foreground">
              {t('dashboard.messagesPerDay')}
            </h2>
            {loading && (
              <RefreshCw className="h-3.5 w-3.5 animate-spin text-muted-foreground" />
            )}
          </div>
          {byDay.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              {t('dashboard.noMessagesData')}
            </p>
          ) : (
            <div className="overflow-x-auto">
              {/* 2.25rem per bar keeps the labels readable on a phone, where the
                  chart scrolls horizontally instead of squashing. */}
              <div
                className="flex h-44 items-end gap-1"
                style={{ minWidth: `${byDay.length * 2.25}rem` }}
              >
                {byDay.map((d) => (
                  <div
                    key={d.key}
                    className="flex flex-1 flex-col items-center gap-1"
                    title={`${d.key}: ${d.count}`}
                  >
                    {/* The bar's height is a share of what the count leaves free. Its
                        parent needs a definite height, or the percentage has nothing
                        to resolve against and every bar collapses to 0px. */}
                    <div className="flex h-36 w-full flex-col items-center justify-end">
                      <span className="mb-0.5 text-xs font-semibold text-foreground">
                        {fmtNumber(d.count)}
                      </span>
                      <div
                        className="w-full max-w-16 rounded-t bg-primary/70"
                        style={{
                          height: `max(2px, calc((100% - 1.25rem) * ${d.count / maxDay}))`,
                        }}
                      />
                    </div>
                    <span className="text-[10px] text-muted-foreground">
                      {d.key.slice(5)}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
        </div>

        <div className="rounded-xl border border-sidebar-border bg-sidebar p-4">
          <h2 className="mb-4 text-sm font-semibold text-foreground">
            {t('dashboard.topConversations')}
          </h2>
          <TopConversations sources={messages.topSources ?? []} />
        </div>

        <div className="rounded-xl border border-sidebar-border bg-sidebar p-4">
          <h2 className="mb-3 text-sm font-semibold text-foreground">
            {t('sidebar.instances')}
          </h2>
          {instancesLoading ? (
            <Skeleton className="h-12 w-full" />
          ) : (
            <InstancesTable
            instances={instances}
            overviews={overviews}
            selectedId={selectedId}
            onSelect={setSelectedId}
          />
          )}
          {selected ? (
            <InstanceLogs
              instanceId={selected.id}
              instanceName={selected.instanceName}
              onClose={() => setSelectedId('')}
            />
          ) : (
            instances.length > 0 && (
              <p className="mt-3 text-xs text-muted-foreground">{t('dashboard.logsHint')}</p>
            )
          )}
        </div>
      </div>
    </div>
  );
}
