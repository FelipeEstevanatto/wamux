import { describe, expect, it } from 'vitest';

import type { HistoryMessage } from '@/types/messages';
import { captionOf, fileExtension, fileTypeColor, humanSize, showsInline } from './fileInfo';

const message = (overrides: Partial<HistoryMessage>) => overrides as HistoryMessage;
const MB = 1024 * 1024;

describe('humanSize', () => {
  it.each([
    [500, '500 B'],
    [2048, '2 KB'],
    [3 * MB, '3.0 MB'],
    [1900 * MB, '1.86 GB'],
  ])('%d is %s', (bytes, text) => {
    expect(humanSize(bytes)).toBe(text);
  });
});

describe('fileExtension', () => {
  it.each([
    ['report.pdf', 'PDF'],
    ['archive.tar.gz', 'GZ'],
    ['no-extension', ''],
    ['.hidden', ''],
    ['trailing.', ''],
    ['', ''],
  ])('%s gives "%s"', (name, extension) => {
    expect(fileExtension(name)).toBe(extension);
  });
});

describe('showsInline', () => {
  it('shows images, videos and audio', () => {
    for (const media_mimetype of ['image/png', 'video/mp4', 'audio/ogg']) {
      expect(showsInline(message({ media_mimetype, file_size: MB }))).toBe(true);
    }
  });

  it('leaves documents as cards', () => {
    expect(showsInline(message({ media_mimetype: 'application/pdf' }))).toBe(false);
    expect(showsInline(message({ media_mimetype: '' }))).toBe(false);
  });

  // Showing one would pull the whole file into the page just to draw the chat.
  it('leaves large media as cards', () => {
    expect(showsInline(message({ media_mimetype: 'video/mp4', file_size: 77 * MB }))).toBe(false);
  });

  it('shows media of unknown size, from before sizes were stored', () => {
    expect(showsInline(message({ media_mimetype: 'image/png' }))).toBe(true);
  });
});

describe('captionOf', () => {
  it('hides a caption that repeats the file name', () => {
    expect(captionOf(message({ text_content: 'a.pdf', file_name: 'a.pdf' }))).toBe('');
  });

  it('keeps a real caption', () => {
    expect(captionOf(message({ text_content: 'Invoice', file_name: 'a.pdf' }))).toBe('Invoice');
  });

  it('keeps text on messages without a file', () => {
    expect(captionOf(message({ text_content: 'hello' }))).toBe('hello');
  });
});

describe('fileTypeColor', () => {
  it.each([
    ['PDF', 'bg-red-600'],
    ['DOCX', 'bg-blue-600'],
    ['XLS', 'bg-green-700'],
    ['ZIP', 'bg-amber-700'],
    ['BIN', 'bg-slate-500'],
    ['', 'bg-slate-500'],
  ])('%s is %s', (extension, color) => {
    expect(fileTypeColor(extension)).toBe(color);
  });
});
