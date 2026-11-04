import { useState } from 'react';
import {
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  Download,
  FileText,
  Loader2,
} from 'lucide-react';

import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from '@/components/ui';
import { useI18n } from '@/i18n/I18nContext';
import { useMediaSrc } from '@/hooks/useMediaSrc';
import type { HistoryMessage } from '@/types/messages';

interface MediaViewerProps {
  open: boolean;
  items: HistoryMessage[];
  initialIndex: number;
  token: string;
  onClose: () => void;
}

function mimeOf(message: HistoryMessage): string {
  return message.media_mimetype || '';
}

/**
 * Full-screen media lightbox for images/videos, with previous/next navigation
 * and a download action. Documents/audio open in a simple card so the viewer
 * never shows a broken <img> for a non-visual attachment.
 */
export default function MediaViewer({
  open,
  items,
  initialIndex,
  token,
  onClose,
}: MediaViewerProps) {
  const { t } = useI18n();
  const [current, setCurrent] = useState(initialIndex);
  const [lastInitial, setLastInitial] = useState(initialIndex);
  const [lastOpen, setLastOpen] = useState(open);

  if (initialIndex !== lastInitial) {
    setLastInitial(initialIndex);
    setCurrent(initialIndex);
  }
  if (open !== lastOpen) {
    setLastOpen(open);
    if (open) setCurrent(initialIndex);
  }

  const index = items.length === 0 ? 0 : Math.min(current, items.length - 1);
  const item = items[index];
  const { src, loading, failed } = useMediaSrc(item?.media_url ?? '', token);

  const go = (delta: number) => {
    if (items.length < 2) return;
    setCurrent((prev) => (prev + delta + items.length) % items.length);
  };

  if (!item) return null;

  const mime = mimeOf(item);
  const isImage = mime.startsWith('image/');
  const isVideo = mime.startsWith('video/');
  const isAudio = mime.startsWith('audio/');

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
    >
      <DialogContent
        className="max-w-5xl p-2 sm:p-4"
        onKeyDown={(e) => {
          if (e.key === 'ArrowLeft') {
            e.preventDefault();
            go(-1);
          } else if (e.key === 'ArrowRight') {
            e.preventDefault();
            go(1);
          }
        }}
      >
        <DialogTitle className="sr-only">
          {t('messages.mediaViewerTitle')}
        </DialogTitle>
        <DialogDescription className="sr-only">
          {t('messages.mediaViewerDescription')}
        </DialogDescription>

        <div className="relative flex min-h-[45vh] items-center justify-center">
          {loading ? (
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
          ) : failed || !src ? (
            <div className="flex flex-col items-center gap-2 p-8 text-center text-sm text-muted-foreground">
              <AlertCircle className="h-8 w-8 text-destructive" />
              {t('messages.mediaUnavailable')}
            </div>
          ) : isImage ? (
            <img
              src={src}
              alt={item.message_type}
              className="max-h-[75vh] max-w-full rounded-md object-contain"
            />
          ) : isVideo ? (
            <video
              src={src}
              controls
              className="max-h-[75vh] max-w-full rounded-md"
            />
          ) : isAudio ? (
            <audio src={src} controls className="w-full" />
          ) : (
            <div className="flex flex-col items-center gap-4 rounded-lg border border-border bg-muted/30 p-10 text-center">
              <FileText className="h-12 w-12 text-muted-foreground" />
              <p className="text-sm font-medium">{item.message_type}</p>
              <a
                href={src}
                download
                target="_blank"
                rel="noreferrer noopener"
                className="inline-flex items-center gap-2 text-sm text-primary underline"
              >
                <Download className="h-4 w-4" />
                {t('messages.downloadMedia')}
              </a>
            </div>
          )}

          {items.length > 1 && (
            <>
              <Button
                type="button"
                size="icon"
                variant="outline"
                onClick={() => go(-1)}
                className="absolute left-1 top-1/2 -translate-y-1/2 rounded-full bg-background/80"
                aria-label={t('messages.previousMedia')}
                title={t('messages.previousMedia')}
              >
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <Button
                type="button"
                size="icon"
                variant="outline"
                onClick={() => go(1)}
                className="absolute right-1 top-1/2 -translate-y-1/2 rounded-full bg-background/80"
                aria-label={t('messages.nextMedia')}
                title={t('messages.nextMedia')}
              >
                <ChevronRight className="h-4 w-4" />
              </Button>
            </>
          )}
        </div>

        <div className="flex items-center justify-between gap-2 px-2 pb-1 text-xs text-muted-foreground">
          <span>
            {index + 1} / {items.length}
          </span>
          {src && (
            <a
              href={src}
              download
              target="_blank"
              rel="noreferrer noopener"
              className="inline-flex items-center gap-1 transition-colors hover:text-foreground"
              title={t('messages.downloadMedia')}
            >
              <Download className="h-3.5 w-3.5" />
              {t('messages.downloadMedia')}
            </a>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
