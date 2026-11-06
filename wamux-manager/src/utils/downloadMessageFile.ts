import * as messagesApi from '@/services/api/messages';
import type { HistoryMessage } from '@/types/messages';

/** Hands a stored file to the browser's downloader, which saves it straight to disk. */
export async function downloadMessageFile(token: string, message: HistoryMessage): Promise<void> {
  const url = await messagesApi.getMediaDownloadUrl(token, message);
  const link = document.createElement('a');
  link.href = url;
  // Empty, so the name comes from the server, which adds what the file is missing.
  link.setAttribute('download', '');
  link.rel = 'noreferrer noopener';
  document.body.appendChild(link);
  link.click();
  link.remove();
}
