import { useCallback, useState } from 'react';
import {
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  Download,
  Loader2,
  Maximize,
  RotateCw,
  X,
  ZoomIn,
  ZoomOut,
} from 'lucide-react';
import { toast } from 'sonner';

import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui';
import ZoomableImage from '@/components/messages/ZoomableImage';
import { useI18n } from '@/i18n/I18nContext';
import { useMediaSrc } from '@/hooks/useMediaSrc';
import type { HistoryMessage } from '@/types/messages';
import { downloadMessageFile } from '@/utils/downloadMessageFile';
import { humanSize } from '@/utils/fileInfo';
import { MAX_SCALE, MIN_SCALE, NO_ZOOM, type Size, type ZoomState } from '@/utils/zoom';

const ZOOM_STEP = 1.5;

interface MediaViewerProps {
  open: boolean;
  items: HistoryMessage[];
  initialIndex: number;
  token: string;
  onClose: () => void;
}

function ToolButton({
  label,
  onClick,
  disabled,
  children,
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={label}
      aria-label={label}
      className="rounded-full p-2 text-white/90 transition-colors hover:bg-white/15 disabled:opacity-40 disabled:hover:bg-transparent"
    >
      {children}
    </button>
  );
}

/**
 * Full-screen viewer for the conversation's images and videos. Size, type and date
 * come from the stored message, and the dimensions from the file already on screen,
 * so showing them costs no extra download.
 */
export default function MediaViewer({ open, items, initialIndex, token, onClose }: MediaViewerProps) {
  const { t, locale } = useI18n();
  const [current, setCurrent] = useState(initialIndex);
  const [lastInitial, setLastInitial] = useState(initialIndex);
  const [lastOpen, setLastOpen] = useState(open);
  const [zoom, setZoom] = useState<ZoomState>(NO_ZOOM);
  const [rotation, setRotation] = useState(0);
  const [dimensions, setDimensions] = useState<Size | null>(null);
  const [duration, setDuration] = useState(0);

  const resetView = () => {
    setZoom(NO_ZOOM);
    setRotation(0);
    setDimensions(null);
    setDuration(0);
  };

  if (initialIndex !== lastInitial) {
    setLastInitial(initialIndex);
    setCurrent(initialIndex);
    resetView();
  }
  if (open !== lastOpen) {
    setLastOpen(open);
    if (open) {
      setCurrent(initialIndex);
      resetView();
    }
  }

  const index = items.length === 0 ? 0 : Math.min(current, items.length - 1);
  const item = items[index];
  const { src, loading, failed } = useMediaSrc(item?.media_url ?? '', token);

  const onZoom = useCallback((update: (value: ZoomState) => ZoomState) => setZoom(update), []);

  if (!item) return null;

  const go = (delta: number) => {
    if (items.length < 2) return;
    setCurrent((index + delta + items.length) % items.length);
    resetView();
  };

  const isImage = (item.media_mimetype || '').startsWith('image/');
  const isVideo = (item.media_mimetype || '').startsWith('video/');

  const zoomBy = (factor: number) =>
    setZoom((value) => {
      const scale = Math.min(MAX_SCALE, Math.max(MIN_SCALE, value.scale * factor));
      return scale === 1 ? NO_ZOOM : { ...value, scale };
    });
  const rotate = () => {
    setRotation((value) => value + 90);
    setZoom(NO_ZOOM);
  };

  const download = async () => {
    try {
      await downloadMessageFile(token, item);
    } catch {
      toast.error(t('messages.downloadError'));
    }
  };

  const onKeyDown = (event: React.KeyboardEvent) => {
    const actions: Record<string, () => void> = {
      ArrowLeft: () => go(-1),
      ArrowRight: () => go(1),
    };
    if (isImage) {
      Object.assign(actions, {
        '+': () => zoomBy(ZOOM_STEP),
        '=': () => zoomBy(ZOOM_STEP),
        '-': () => zoomBy(1 / ZOOM_STEP),
        '0': () => setZoom(NO_ZOOM),
        r: rotate,
      });
    }
    const action = actions[event.key];
    if (action) {
      event.preventDefault();
      action();
    }
  };

  const kind = isImage ? t('messages.viewerImage') : t('messages.viewerVideo');
  const details = [
    item.file_name ? kind : '',
    dimensions ? `${dimensions.width} × ${dimensions.height}` : '',
    duration ? `${Math.floor(duration / 60)}:${String(Math.round(duration % 60)).padStart(2, '0')}` : '',
    item.file_size ? humanSize(item.file_size) : '',
    new Date(item.timestamp.replace(' ', 'T')).toLocaleString(locale, {
      dateStyle: 'short',
      timeStyle: 'short',
    }),
  ]
    .filter(Boolean)
    .join(' · ');

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent
        showCloseButton={false}
        onKeyDown={onKeyDown}
        className="top-0 left-0 flex h-dvh w-screen max-w-none translate-x-0 translate-y-0 flex-col gap-0 rounded-none border-0 bg-black p-0 text-white sm:max-w-none"
      >
        <DialogDescription className="sr-only">{t('messages.mediaViewerDescription')}</DialogDescription>

        <header className="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
          {/* On a phone the tools need the whole row, so this takes its own. */}
          <div className="min-w-0 basis-full sm:flex-1 sm:basis-0">
            <DialogTitle className="truncate text-sm font-medium text-white">
              {item.file_name || kind}
            </DialogTitle>
            <p className="truncate text-xs text-white/70">{details}</p>
          </div>

          {items.length > 1 && (
            <span className="mr-auto shrink-0 text-xs text-white/70 sm:mr-0">
              {index + 1} / {items.length}
            </span>
          )}

          {isImage && (
            <div className="flex shrink-0 items-center">
              <ToolButton label={t('messages.zoomOut')} onClick={() => zoomBy(1 / ZOOM_STEP)} disabled={zoom.scale <= MIN_SCALE}>
                <ZoomOut className="h-5 w-5" />
              </ToolButton>
              <span className="hidden w-12 text-center text-xs text-white/80 sm:block">
                {Math.round(zoom.scale * 100)}%
              </span>
              <ToolButton label={t('messages.zoomIn')} onClick={() => zoomBy(ZOOM_STEP)} disabled={zoom.scale >= MAX_SCALE}>
                <ZoomIn className="h-5 w-5" />
              </ToolButton>
              <ToolButton label={t('messages.zoomReset')} onClick={() => setZoom(NO_ZOOM)} disabled={zoom.scale === 1}>
                <Maximize className="h-5 w-5" />
              </ToolButton>
              <ToolButton label={t('messages.rotate')} onClick={rotate}>
                <RotateCw className="h-5 w-5" />
              </ToolButton>
            </div>
          )}

          <ToolButton label={t('messages.downloadMedia')} onClick={() => void download()}>
            <Download className="h-5 w-5" />
          </ToolButton>
          <ToolButton label={t('common.close')} onClick={onClose}>
            <X className="h-5 w-5" />
          </ToolButton>
        </header>

        <div className="relative min-h-0 flex-1">
          {loading ? (
            <div className="flex h-full items-center justify-center">
              <Loader2 className="h-8 w-8 animate-spin text-white/70" />
            </div>
          ) : failed || !src ? (
            <div className="flex h-full flex-col items-center justify-center gap-2 text-sm text-white/70">
              <AlertCircle className="h-8 w-8" />
              {t('messages.mediaUnavailable')}
            </div>
          ) : isImage ? (
            <ZoomableImage
              src={src}
              alt={item.text_content || t('messages.viewerImage')}
              zoom={zoom}
              rotation={rotation}
              onZoom={onZoom}
              onLoad={setDimensions}
            />
          ) : (
            isVideo && (
              <video
                key={item.message_id}
                src={src}
                controls
                onLoadedMetadata={(event) => {
                  setDimensions({
                    width: event.currentTarget.videoWidth,
                    height: event.currentTarget.videoHeight,
                  });
                  setDuration(event.currentTarget.duration || 0);
                }}
                className="h-full w-full object-contain"
              />
            )
          )}

          {items.length > 1 && (
            <>
              <button
                type="button"
                onClick={() => go(-1)}
                aria-label={t('messages.previousMedia')}
                title={t('messages.previousMedia')}
                className="absolute top-1/2 left-3 -translate-y-1/2 rounded-full bg-black/50 p-2.5 text-white transition-colors hover:bg-black/80"
              >
                <ChevronLeft className="h-6 w-6" />
              </button>
              <button
                type="button"
                onClick={() => go(1)}
                aria-label={t('messages.nextMedia')}
                title={t('messages.nextMedia')}
                className="absolute top-1/2 right-3 -translate-y-1/2 rounded-full bg-black/50 p-2.5 text-white transition-colors hover:bg-black/80"
              >
                <ChevronRight className="h-6 w-6" />
              </button>
            </>
          )}
        </div>

        {item.text_content && (
          <p className="max-h-24 shrink-0 overflow-y-auto bg-black/60 px-4 py-2 text-center text-sm break-words whitespace-pre-wrap">
            {item.text_content}
          </p>
        )}
      </DialogContent>
    </Dialog>
  );
}
