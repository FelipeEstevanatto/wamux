import { Loader2, WifiOff } from 'lucide-react';
import useOnlineStatus from '@/hooks/useOnlineStatus';
import useConnectionStore from '@/store/connectionStore';
import { useI18n } from '@/i18n/I18nContext';

/**
 * Thin status strip shown when the browser is offline or the API is
 * unreachable, so the operator knows the UI may be showing stale data.
 */
export default function ConnectionBanner() {
  const { t } = useI18n();
  const online = useOnlineStatus();
  const reconnecting = useConnectionStore((s) => s.reconnecting);

  if (online && !reconnecting) return null;

  return (
    <div
      role="status"
      aria-live="polite"
      className="flex items-center justify-center gap-2 border-b border-amber-500/30 bg-amber-500/10 px-4 py-1.5 text-xs font-medium text-amber-600 dark:text-amber-400"
    >
      {online ? (
        <Loader2 className="h-3.5 w-3.5 animate-spin" />
      ) : (
        <WifiOff className="h-3.5 w-3.5" />
      )}
      {online ? t('banner.reconnecting') : t('banner.offline')}
    </div>
  );
}
