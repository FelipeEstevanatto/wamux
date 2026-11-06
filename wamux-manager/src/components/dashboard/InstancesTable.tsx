import { useState } from 'react';

import { useI18n } from '@/i18n/I18nContext';
import type { Instance, InstanceOverview } from '@/types/instance';
import { cn } from '@/utils/cn';

function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return '?';
  const first = words[0][0] ?? '';
  const last = words.length > 1 ? (words[words.length - 1][0] ?? '') : '';
  return (first + last).toUpperCase();
}

function Avatar({ name, url }: { name: string; url?: string }) {
  const [failedUrl, setFailedUrl] = useState('');

  return (
    <span className="flex h-7 w-7 shrink-0 items-center justify-center overflow-hidden rounded-full bg-muted text-[10px] font-semibold text-muted-foreground">
      {url && url !== failedUrl ? (
        <img
          src={url}
          alt=""
          referrerPolicy="no-referrer"
          onError={() => setFailedUrl(url)}
          className="h-full w-full object-cover"
        />
      ) : (
        initials(name)
      )}
    </span>
  );
}

function InstancesTable({
  instances,
  overviews,
  selectedId,
  onSelect,
}: {
  instances: Instance[];
  overviews: Record<string, InstanceOverview>;
  selectedId: string;
  onSelect: (id: string) => void;
}) {
  const { t, lang } = useI18n();
  const locale = lang === 'pt' ? 'pt-BR' : 'en-US';

  const formatDate = (value?: string) => {
    if (!value) return '—';
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString(locale);
  };

  if (instances.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('dashboard.noInstances')}</p>;
  }

  const head = [
    t('dashboard.colStatus'),
    t('dashboard.colName'),
    t('dashboard.colNumber'),
    t('dashboard.colOs'),
    t('dashboard.colClient'),
    t('dashboard.kpiContacts'),
    t('dashboard.colCreated'),
    t('dashboard.colReason'),
  ];

  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[56rem] text-left text-sm">
        <thead>
          <tr className="border-b border-sidebar-border text-xs uppercase tracking-wide text-muted-foreground">
            {head.map((label) => (
              <th key={label} scope="col" className="px-2 py-2 font-semibold">
                {label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {instances.map((instance) => {
            const overview = overviews[instance.id];
            const selected = instance.id === selectedId;
            return (
              <tr
                key={instance.id}
                onClick={() => onSelect(instance.id)}
                className={cn(
                  'cursor-pointer border-b border-sidebar-border last:border-b-0 hover:bg-sidebar-accent',
                  selected && 'bg-primary/10'
                )}
              >
                <td className="px-2 py-2">
                  <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs">
                    <span
                      className={cn(
                        'h-2 w-2 rounded-full',
                        instance.connected ? 'bg-green-500' : 'bg-red-500'
                      )}
                    />
                    {instance.connected
                      ? t('dashboard.statusConnected')
                      : t('dashboard.statusDisconnected')}
                  </span>
                </td>
                <td className="px-2 py-2">
                  {/* Keyboard access: the click bubbles to the row. */}
                  <button
                    type="button"
                    aria-pressed={selected}
                    className="flex items-center gap-2 text-left font-medium text-foreground"
                  >
                    {instance.connected && (
                      <Avatar
                        name={overview?.profileName || instance.instanceName}
                        url={overview?.profilePicUrl}
                      />
                    )}
                    {instance.instanceName}
                  </button>
                </td>
                <td className="px-2 py-2 font-mono text-xs">
                  {instance.owner ? `+${instance.owner}` : '—'}
                </td>
                <td className="px-2 py-2">{instance.osName || '—'}</td>
                <td className="px-2 py-2">{instance.clientName || '—'}</td>
                <td className="px-2 py-2 font-mono text-xs">
                  {instance.connected ? (overview?.contactsCount ?? '…') : '—'}
                </td>
                <td className="px-2 py-2 font-mono text-xs whitespace-nowrap">
                  {formatDate(instance.createdAt)}
                </td>
                <td className="px-2 py-2 text-muted-foreground">
                  {instance.disconnectReason || '—'}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

export default InstancesTable;
