import { useI18n } from '@/i18n/I18nContext';

// A radius giving a circumference of 100, so a dash length is a percentage.
const RADIUS = 100 / (2 * Math.PI);

function ConnectionChart({ connected, total }: { connected: number; total: number }) {
  const { t } = useI18n();
  const disconnected = total - connected;
  const pct = total > 0 ? Math.round((connected / total) * 100) : 0;
  const share = total > 0 ? (connected / total) * 100 : 0;

  return (
    <div className="flex items-center gap-6">
      <svg
        viewBox="0 0 36 36"
        role="img"
        aria-label={t('dashboard.connectionSummary', { connected, total })}
        className="h-32 w-32 shrink-0 -rotate-90"
      >
        <circle
          cx="18"
          cy="18"
          r={RADIUS}
          fill="none"
          strokeWidth="4"
          className={total > 0 ? 'stroke-red-500' : 'stroke-muted'}
        />
        {connected > 0 && (
          <circle
            cx="18"
            cy="18"
            r={RADIUS}
            fill="none"
            strokeWidth="4"
            pathLength={100}
            strokeDasharray={`${share} ${100 - share}`}
            className="stroke-green-500"
          />
        )}
        <text
          x="18"
          y="18"
          textAnchor="middle"
          dominantBaseline="central"
          transform="rotate(90 18 18)"
          className="fill-foreground text-[7px] font-bold"
        >
          {total > 0 ? `${pct}%` : '—'}
        </text>
      </svg>
      <dl className="space-y-2 text-sm">
        <div className="flex items-center gap-2">
          <span className="h-2.5 w-2.5 rounded-full bg-green-500" />
          <dt className="text-muted-foreground">{t('dashboard.kpiConnected')}</dt>
          <dd className="font-mono text-foreground">{connected}</dd>
        </div>
        <div className="flex items-center gap-2">
          <span className="h-2.5 w-2.5 rounded-full bg-red-500" />
          <dt className="text-muted-foreground">{t('dashboard.kpiDisconnected')}</dt>
          <dd className="font-mono text-foreground">{disconnected}</dd>
        </div>
      </dl>
    </div>
  );
}

export default ConnectionChart;
