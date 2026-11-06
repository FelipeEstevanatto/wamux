import { useState } from 'react';
import { Download, Loader2 } from 'lucide-react';
import { toast } from 'sonner';

import { useInView } from '@/hooks/useInView';
import { useMediaSrc } from '@/hooks/useMediaSrc';
import { useI18n } from '@/i18n/I18nContext';
import type { HistoryMessage } from '@/types/messages';
import { cn } from '@/utils/cn';
import { downloadMessageFile } from '@/utils/downloadMessageFile';
import { fileExtension, fileTypeColor, humanSize } from '@/utils/fileInfo';

// WhatsApp's inline previews can be 21x30 pixels: a blur at the width of this card.
const MIN_PREVIEW_WIDTH = 100;

function DocumentPreview({ message, token }: { message: HistoryMessage; token: string }) {
  const [ref, inView] = useInView<HTMLDivElement>();
  const [inlineUsable, setInlineUsable] = useState(false);

  // Only a file on this server has a rendered preview, and it is asked for once the
  // card scrolls into view.
  const rendered =
    message.media_mimetype === 'application/pdf' && message.media_url.startsWith('/');
  const { src, loading } = useMediaSrc(
    rendered && inView ? `/chat/media/${encodeURIComponent(message.message_id)}/preview` : '',
    token
  );
  const inline = message.file_thumbnail
    ? `data:image/jpeg;base64,${message.file_thumbnail}`
    : '';
  const waiting = rendered && (!inView || loading);
  const shown = src || (inlineUsable ? inline : '');

  return (
    <div ref={ref}>
      {shown ? (
        <img src={shown} alt="" className="h-44 w-full bg-white object-cover object-top" />
      ) : (
        waiting && <div className="h-44 w-full animate-pulse bg-black/10" />
      )}
      {inline && !src && !inlineUsable && (
        <img
          src={inline}
          alt=""
          hidden
          onLoad={(event) => setInlineUsable(event.currentTarget.naturalWidth >= MIN_PREVIEW_WIDTH)}
        />
      )}
    </div>
  );
}

/**
 * A file in the chat, laid out like WhatsApp's: a preview when there is one, then its
 * type, name, details and a download button. The file is only requested
 * on the click, by the browser's own downloader, so a large one goes straight to disk.
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
  const extension = fileExtension(message.file_name ?? '');
  const pages = message.page_count ?? 0;
  const details = [
    pages === 1 ? t('messages.onePage') : pages > 1 ? t('messages.pageCount', { count: pages }) : '',
    extension,
    message.file_size ? humanSize(message.file_size) : '',
  ]
    .filter(Boolean)
    .join(' · ');

  const download = async () => {
    setPreparing(true);
    try {
      await downloadMessageFile(token, message);
    } catch {
      toast.error(t('messages.downloadError'));
    } finally {
      setPreparing(false);
    }
  };

  return (
    <div
      className={cn(
        'mb-1 w-80 max-w-full overflow-hidden rounded-lg',
        mine ? 'bg-primary-foreground/15' : 'bg-background/60'
      )}
    >
      <DocumentPreview message={message} token={token} />
      <div className="flex items-center gap-3 p-3">
        <span
          className={cn(
            'flex h-11 w-9 shrink-0 items-center justify-center rounded text-[9px] font-bold text-white',
            fileTypeColor(extension)
          )}
          aria-hidden="true"
        >
          {extension.slice(0, 4) || 'FILE'}
        </span>
        <div className="min-w-0 flex-1">
          <p className="line-clamp-2 font-medium break-words" title={name}>
            {name}
          </p>
          {details && <p className="mt-0.5 text-xs opacity-70">{details}</p>}
        </div>
        {message.media_url ? (
          <button
            type="button"
            onClick={() => void download()}
            disabled={preparing}
            title={t('messages.downloadMedia')}
            aria-label={t('messages.downloadFile', { name })}
            className="shrink-0 rounded-full border border-current/30 p-2 transition-colors hover:bg-black/10 disabled:opacity-60"
          >
            {preparing ? <Loader2 className="h-5 w-5 animate-spin" /> : <Download className="h-5 w-5" />}
          </button>
        ) : (
          <span className="shrink-0 text-[11px] opacity-70">{t('messages.fileNotStored')}</span>
        )}
      </div>
    </div>
  );
}

export default FileCard;
