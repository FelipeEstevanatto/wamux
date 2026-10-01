import { useEffect, useMemo } from 'react';
import {
  Activity,
  Cpu,
  ExternalLink,
  HardDrive,
  Layers,
  Mail,
  RefreshCw,
  Server,
  Smartphone,
  Users,
} from 'lucide-react';
import useServerStats from '@/hooks/useServerStats';
import useInstancesStore from '@/store/instancesStore';
import useAuth from '@/hooks/useAuth';
import { useDarkMode } from '@/hooks/useDarkMode';
import GithubIcon from '@/components/base/GithubIcon';
import {
  FORK_REPO,
  UPSTREAM_REPO,
} from '@/constants/branding';

// (fork/upstream links come from @/constants/branding)

const fmtNumber = (n?: number) =>
  n === undefined || n === null ? '—' : n.toLocaleString('pt-BR');

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
}: {
  icon: typeof Activity;
  label: string;
  value: string;
  sub?: string;
  tone?: 'default' | 'green' | 'red';
  title?: string;
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
      <div className={`mt-2 text-3xl font-bold ${valueColor}`}>{value}</div>
      {sub && <div className="mt-1 text-xs text-muted-foreground">{sub}</div>}
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
  const { stats, error, loading } = useServerStats(15000);
  const { instances, fetchInstances, overviews, fetchOverviews } =
    useInstancesStore();
  const { apiUrl, apiKey } = useAuth();
  const { theme } = useDarkMode();

  // The embedded dashboard only makes sense once credentials are available.
  // Derived rather than stored, so the effect below has no state to set.
  const embedReady = Boolean(apiUrl && apiKey);

  // The static self-hosted dashboard at /dashboard has its own charts, top
  // sources and per-instance logs; rather than duplicate it, it is embedded
  // below. It reads its credentials from its own localStorage keys, so seed
  // them here (same origin) before mounting the iframe to avoid a second login.
  useEffect(() => {
    if (!apiKey) return;
    const sameOrigin =
      !apiUrl || apiUrl.replace(/\/+$/, '') === window.location.origin;
    localStorage.setItem('egogo_dash_key', apiKey);
    localStorage.setItem(
      'egogo_dash_base',
      sameOrigin ? '' : apiUrl.replace(/\/+$/, '')
    );
  }, [apiUrl, apiKey]);

  useEffect(() => {
    fetchInstances();
  }, [fetchInstances]);

  useEffect(() => {
    fetchOverviews(instances);
  }, [instances, fetchOverviews]);

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
      ? `Load average (1 min): média de tarefas executáveis (rodando ou na fila). ${load1.toFixed(2)}` +
        (loadPct !== undefined
          ? ` em ${cpus} CPUs ≈ ${loadPct}% da capacidade`
          : '') +
        (system.loadAvg5 !== undefined
          ? ` · 5m ${system.loadAvg5.toFixed(2)}`
          : '') +
        (system.loadAvg15 !== undefined
          ? ` · 15m ${system.loadAvg15.toFixed(2)}`
          : '')
      : undefined;

  return (
    <div className="h-full overflow-y-auto p-4 sm:p-6">
      <div className="mx-auto max-w-6xl space-y-6">
        {/* Header */}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-2xl font-bold text-foreground">Dashboard</h1>
            <p className="text-sm text-muted-foreground">
              Visão geral do sistema e das instâncias
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {deviceName && (
              <span
                className="inline-flex items-center gap-2 rounded-md border border-sidebar-border bg-sidebar px-3 py-2 text-xs text-muted-foreground"
                title="Nome exibido como aparelho conectado no WhatsApp (OS_NAME)"
              >
                <Smartphone className="h-3.5 w-3.5" />
                {deviceName}
              </span>
            )}
            <span className="inline-flex items-center gap-2 rounded-md border border-sidebar-border bg-sidebar px-3 py-2 text-xs text-muted-foreground">
              <Server className="h-3.5 w-3.5" />
              {system.version ? `versão ${system.version}` : 'versão —'}
            </span>
            <a
              href={FORK_REPO}
              target="_blank"
              rel="noreferrer noopener"
              className="inline-flex items-center gap-2 rounded-md border border-sidebar-border bg-sidebar px-3 py-2 text-xs text-muted-foreground transition-colors hover:text-foreground"
              title="Repositório deste fork"
            >
              <GithubIcon className="h-3.5 w-3.5" />
              Fork
            </a>
            <a
              href={UPSTREAM_REPO}
              target="_blank"
              rel="noreferrer noopener"
              className="inline-flex items-center gap-2 rounded-md border border-sidebar-border bg-sidebar px-3 py-2 text-xs text-muted-foreground transition-colors hover:text-foreground"
              title="Projeto upstream (Evolution Foundation)"
            >
              <GithubIcon className="h-3.5 w-3.5" />
              Upstream
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
            label="Instâncias"
            value={fmtNumber(total)}
            sub={`${connected} conectada(s)`}
          />
          <Kpi
            icon={Activity}
            label="Conectadas"
            value={fmtNumber(connected)}
            sub={total ? `${Math.round((connected / total) * 100)}% do total` : '—'}
            tone="green"
          />
          <Kpi
            icon={Mail}
            label="Mensagens"
            value={fmtNumber(messages.total)}
            sub="salvas no banco"
          />
          <Kpi
            icon={Users}
            label="Contatos"
            value={fmtNumber(contacts)}
            sub="somando instâncias conectadas"
          />
        </div>

        {/* Host / runtime */}
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <Kpi
            icon={HardDrive}
            label="RAM do host"
            value={
              system.hostMemUsedPct !== undefined
                ? `${Math.round(system.hostMemUsedPct)}%`
                : '—'
            }
            sub={
              system.hostMemTotalMB !== undefined
                ? `${fmtMB((system.hostMemTotalMB || 0) - (system.hostMemAvailableMB || 0))} / ${fmtMB(system.hostMemTotalMB)}`
                : 'host indisponível'
            }
          />
          <Kpi
            icon={Cpu}
            label="Carga (1m)"
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
                : 'load indisponível'
            }
            title={loadTitle}
          />
          <Kpi
            icon={Activity}
            label="Goroutines"
            value={fmtNumber(system.goroutines)}
            sub={`heap ${fmtMB(system.heapInuseMB)}`}
          />
          <Kpi
            icon={Server}
            label="Uptime"
            value={fmtUptime(system.uptimeSeconds)}
            sub={system.goVersion || '—'}
          />
        </div>

        {/* Storage */}
        <div className="rounded-xl border border-sidebar-border bg-sidebar p-4">
          <div className="mb-4 flex items-center justify-between">
            <h2 className="text-sm font-semibold text-foreground">
              Armazenamento
            </h2>
            <HardDrive className="h-3.5 w-3.5 text-muted-foreground" />
          </div>
          <div className="space-y-4">
            {storage.diskTotalMB !== undefined && (
              <div>
                <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
                  <span className="text-muted-foreground">
                    Disco{' '}
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
                label="Dados"
                hint={storage.dataDir}
                value={
                  storage.dataUsedMB !== undefined
                    ? `${fmtMB(storage.dataUsedMB)} · ${fmtNumber(storage.dataFiles)} arquivo(s)`
                    : undefined
                }
              />
              <StorageRow
                label="Banco de dados"
                hint="PostgreSQL"
                value={
                  storage.dbTotalMB !== undefined
                    ? `${fmtMB(storage.dbTotalMB)}${
                        storage.dbMessagesMB
                          ? ` · messages ${fmtMB(storage.dbMessagesMB)}`
                          : ''
                      }`
                    : undefined
                }
              />
              <StorageRow
                label="Mídia"
                value={storage.mediaEnabled ? storage.mediaBackend : 'não configurada'}
                hint={
                  storage.mediaEnabled
                    ? 'arquivos enviados/recebidos'
                    : 'mídia não é gravada localmente'
                }
              />
            </dl>
          </div>
        </div>

        {/* Messages per day */}
        <div className="rounded-xl border border-sidebar-border bg-sidebar p-4">
          <div className="mb-4 flex items-center justify-between">
            <h2 className="text-sm font-semibold text-foreground">
              Mensagens por dia
            </h2>
            {loading && (
              <RefreshCw className="h-3.5 w-3.5 animate-spin text-muted-foreground" />
            )}
          </div>
          {byDay.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              Sem dados. Ative DATABASE_SAVE_MESSAGES para registrar mensagens.
            </p>
          ) : (
            <div className="overflow-x-auto">
              {/* 2.25rem per bar keeps the labels readable on a phone, where the
                  chart scrolls horizontally instead of squashing. */}
              <div
                className="flex h-40 items-end gap-1"
                style={{ minWidth: `${byDay.length * 2.25}rem` }}
              >
                {byDay.map((d) => (
                  <div
                    key={d.key}
                    className="flex flex-1 flex-col items-center gap-1"
                    title={`${d.key}: ${d.count}`}
                  >
                    {/* The bar's height is a percentage, so its parent must have a
                        definite height — otherwise the percentage is unresolved and
                        every bar collapses to 0px (the chart looked empty). */}
                    <div className="flex h-32 w-full items-end">
                      <div
                        className="w-full rounded-t bg-primary/70"
                        style={{
                          height: `${Math.max(2, (d.count / maxDay) * 100)}%`,
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

        {/* Full self-hosted dashboard (charts, top sources, per-instance logs).
            Embedded instead of duplicated; it hides its own KPI grid when
            embedded (see dashboard.html). On a phone a nested-scrolling iframe
            is unusable, so small screens get a link to the full page instead. */}
        {embedReady && (
          <>
            <div className="rounded-xl border border-sidebar-border bg-sidebar p-4 md:hidden">
              <h2 className="text-sm font-semibold text-foreground">
                Dashboard completo
              </h2>
              <p className="mt-1 text-xs text-muted-foreground">
                Gráficos, conversas mais ativas, instâncias e logs.
              </p>
              <a
                href={`/dashboard?theme=${theme}`}
                target="_blank"
                rel="noreferrer noopener"
                className="mt-3 inline-flex items-center gap-2 rounded-md border border-sidebar-border px-3 py-2 text-xs text-foreground transition-colors hover:bg-sidebar-accent"
              >
                <ExternalLink className="h-3.5 w-3.5" />
                Abrir dashboard completo
              </a>
            </div>
            <div className="hidden overflow-hidden rounded-xl border border-sidebar-border bg-sidebar md:block">
              <iframe
                src={`/dashboard?embed=1&theme=${theme}`}
                title="Dashboard completo"
                className="h-[70vh] w-full border-0"
              />
            </div>
          </>
        )}
      </div>
    </div>
  );
}
