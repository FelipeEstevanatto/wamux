import { useEffect, useMemo, useState } from 'react';
import {
  Activity,
  Cpu,
  HardDrive,
  Layers,
  Mail,
  RefreshCw,
  Server,
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
}: {
  icon: typeof Activity;
  label: string;
  value: string;
  sub?: string;
  tone?: 'default' | 'green' | 'red';
}) {
  const valueColor =
    tone === 'green'
      ? 'text-green-500'
      : tone === 'red'
        ? 'text-red-500'
        : 'text-foreground';
  return (
    <div className="rounded-xl border border-sidebar-border bg-sidebar p-4">
      <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        <Icon className="h-3.5 w-3.5" />
        {label}
      </div>
      <div className={`mt-2 text-3xl font-bold ${valueColor}`}>{value}</div>
      {sub && <div className="mt-1 text-xs text-muted-foreground">{sub}</div>}
    </div>
  );
}

export default function Dashboard() {
  const { stats, error, loading } = useServerStats(15000);
  const { instances, fetchInstances, overviews, fetchOverviews } =
    useInstancesStore();
  const { apiUrl, apiKey } = useAuth();
  const { theme } = useDarkMode();

  // The static self-hosted dashboard at /dashboard has its own charts, top
  // sources and per-instance logs; rather than duplicate it, it is embedded
  // below. It reads its credentials from its own localStorage keys, so seed
  // them here (same origin) before mounting the iframe to avoid a second login.
  const [embedReady, setEmbedReady] = useState(false);
  useEffect(() => {
    if (!apiKey) return;
    const sameOrigin =
      !apiUrl || apiUrl.replace(/\/+$/, '') === window.location.origin;
    localStorage.setItem('egogo_dash_key', apiKey);
    localStorage.setItem(
      'egogo_dash_base',
      sameOrigin ? '' : apiUrl.replace(/\/+$/, '')
    );
    setEmbedReady(true);
  }, [apiUrl, apiKey]);

  useEffect(() => {
    fetchInstances();
  }, [fetchInstances]);

  useEffect(() => {
    fetchOverviews(instances);
  }, [instances, fetchOverviews]);

  const connected = instances.filter((i) => i.connected).length;
  const total = instances.length;

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
  const byDay = useMemo(
    () => (messages.byDay || []).slice().reverse(),
    [messages.byDay]
  );
  const maxDay = Math.max(1, ...byDay.map((d) => d.count));

  return (
    <div className="h-full overflow-y-auto p-6">
      <div className="mx-auto max-w-6xl space-y-6">
        {/* Header */}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-2xl font-bold text-foreground">Dashboard</h1>
            <p className="text-sm text-muted-foreground">
              Visão geral do sistema e das instâncias
            </p>
          </div>
          <div className="flex items-center gap-2">
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
            label="Load 1m"
            value={system.loadAvg1 !== undefined ? system.loadAvg1.toFixed(2) : '—'}
            sub={`CPUs: ${fmtNumber(system.numCpu)}`}
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
            <div className="flex h-40 items-end gap-1">
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
          )}
        </div>

        {/* Full self-hosted dashboard (charts, top sources, per-instance logs).
            Embedded instead of duplicated; it hides its own KPI grid when
            embedded (see dashboard.html). */}
        {embedReady && (
          <div className="overflow-hidden rounded-xl border border-sidebar-border bg-sidebar">
            <iframe
              src={`/dashboard?embed=1&theme=${theme}`}
              title="Dashboard completo"
              className="h-[70vh] w-full border-0"
            />
          </div>
        )}
      </div>
    </div>
  );
}
