import { useState } from 'react';
import { Download, FileText, Loader2 } from 'lucide-react';
import { toast } from 'sonner';

import { useI18n } from '@/i18n/I18nContext';
import * as messagesApi from '@/services/api/messages';
import type { HistoryMessage } from '@/types/messages';
import { cn } from '@/utils/cn';
import { fileExtension, humanSize } from '@/utils/fileInfo';

/**
 * A file in the chat: its name, type, size and a download button. The file is only
 * requested on the click, and then by the browser's own downloader, so a large one
 * goes straight to disk and never through this page's memory.
 */
function FileCard({
  message,
  token,
  mine,
}: {
  message: HistoryMessage;
  token: string;
  mine: boolean;
}) {
  const { t } = useI18n();
  const [preparing, setPreparing] = useState(false);

  const name = message.file_name || t('messages.fileFallback');
  const details = [
    fileExtension(message.file_name ?? ''),
    message.file_size ? humanSize(message.file_size) : '',
  ]
    .filter(Boolean)
    .join(' · ');

  const download = async () => {
    setPreparing(true);
    try {
      const url = await messagesApi.getMediaDownloadUrl(token, message);
      const link = document.createElement('a');
      link.href = url;
      link.download = message.file_name ?? '';
      link.rel = 'noreferrer noopener';
      document.body.appendChild(link);
      link.click();
      link.remove();
    } catch {
      toast.error(t('messages.downloadError'));
    } finally {
      setPreparing(false);
    }
  };

  return (
    <div
      className={cn(
        'mb-1 flex w-72 max-w-full items-center gap-2.5 rounded-md p-2',
        mine ? 'bg-primary-foreground/15' : 'bg-background/60'
      )}
    >
      <FileText className="h-8 w-8 shrink-0 opacity-80" />
      <div className="min-w-0 flex-1">
        <p className="truncate font-medium" title={name}>
          {name}
        </p>
        {details && <p className="text-xs opacity-70">{details}</p>}
      </div>
      {message.media_url ? (
        <button
          type="button"
          onClick={() => void download()}
          disabled={preparing}
          title={t('messages.downloadMedia')}
          aria-label={t('messages.downloadFile', { name })}
          className="shrink-0 rounded-full p-2 transition-colors hover:bg-black/10 disabled:opacity-60"
        >
          {preparing ? <Loader2 className="h-5 w-5 animate-spin" /> : <Download className="h-5 w-5" />}
        </button>
      ) : (
        <span className="shrink-0 text-[11px] opacity-70">{t('messages.fileNotStored')}</span>
      )}
    </div>
  );
}

export default FileCard;
