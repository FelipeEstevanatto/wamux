import { useI18n } from '@/i18n/I18nContext';
import type { StatKV } from '@/services/api/server';

/** The saved name when there is one, otherwise the number. */
function sourceLabel(source: StatKV): string {
  if (source.name) return source.name;
  const phone = source.phone || String(source.key ?? '');
  if (!phone) return '—';
  if (phone === 'status') return 'Status';
  return `+${phone.replace(/^\+/, '')}`;
}

function TopConversations({ sources }: { sources: StatKV[] }) {
  const { t } = useI18n();

  if (sources.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('dashboard.noMessagesData')}</p>;
  }

  const max = Math.max(1, ...sources.map((source) => source.count));
  return (
    <ul className="space-y-2.5">
      {sources.map((source) => (
        <li key={source.key} className="flex items-center gap-3">
          <div className="w-32 shrink-0 sm:w-48">
            <p className="truncate text-sm text-foreground" title={source.phone || source.key}>
              {sourceLabel(source)}
            </p>
            {/* Two contacts can share a saved name. */}
            {source.name && source.phone && (
              <p className="truncate font-mono text-[11px] text-muted-foreground">
                +{source.phone}
              </p>
            )}
          </div>
          <div className="h-3 min-w-0 flex-1 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-primary"
              style={{ width: `${Math.round((source.count / max) * 100)}%` }}
            />
          </div>
          <span className="w-14 text-right font-mono text-xs text-foreground">{source.count}</span>
        </li>
      ))}
    </ul>
  );
}

export default TopConversations;
