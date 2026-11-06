import type { HistoryMessage } from '@/types/messages';

// Above this a file is not fetched just to draw the chat: it stays a card, and the
// browser only gets it when asked to download.
const MAX_INLINE_BYTES = 32 * 1024 * 1024;

export function humanSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

/** "report.final.pdf" gives "PDF". A name without one has none. */
export function fileExtension(name: string): string {
  const dot = name.lastIndexOf('.');
  if (dot <= 0 || dot === name.length - 1) return '';
  return name.slice(dot + 1).toUpperCase();
}

/** Whether the chat shows the file itself. The rest show as a file card. */
export function showsInline(message: HistoryMessage): boolean {
  if ((message.file_size ?? 0) > MAX_INLINE_BYTES) return false;
  const mime = message.media_mimetype || '';
  return mime.startsWith('image/') || mime.startsWith('video/') || mime.startsWith('audio/');
}

/** A document's caption often just repeats its name, which the card already shows. */
export const captionOf = (message: HistoryMessage): string =>
  message.text_content && message.text_content === message.file_name ? '' : message.text_content;
