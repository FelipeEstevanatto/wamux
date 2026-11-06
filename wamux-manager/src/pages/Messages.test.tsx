import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/i18n/I18nContext';
import * as messagesApi from '@/services/api/messages';
import * as serverApi from '@/services/api/server';
import type { ChatSummary, HistoryMessage } from '@/types/messages';
import type { WsStatus } from '@/hooks/useInstanceEvents';
import Messages from './Messages';

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

const CHAT = '100@s.whatsapp.net';

const live = vi.hoisted(() => ({
  status: 'open' as WsStatus,
  websocketEnable: 'enabled',
  onEvent: (() => {}) as (event: { event: string }) => void,
}));
const toastInfo = vi.hoisted(() => vi.fn());
const toastError = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useInstanceEvents', () => ({
  default: (_id: string, onEvent: (event: { event: string }) => void) => {
    live.onEvent = onEvent;
    return live.status;
  },
}));

vi.mock('@/hooks/useInstances', () => ({
  default: () => ({
    instances: [
      {
        id: 'inst',
        instanceName: 'One',
        owner: '',
        apikey: 'instance-token',
        connected: true,
        websocketEnable: live.websocketEnable,
      },
    ],
    isLoading: false,
    fetchInstances: () => Promise.resolve(),
  }),
}));

vi.mock('sonner', () => ({
  toast: { info: toastInfo, success: vi.fn(), error: toastError },
}));

vi.mock('@/services/api/messages', async (original) => ({
  ...(await original<typeof messagesApi>()),
  listChats: vi.fn(),
  getHistory: vi.fn(),
  searchHistory: vi.fn(),
  getSenderNames: vi.fn(),
  sendMedia: vi.fn(),
  fetchMediaObjectUrl: vi.fn(),
  getMediaDownloadUrl: vi.fn(),
}));

vi.mock('@/services/api/server', async (original) => ({
  ...(await original<typeof serverApi>()),
  fetchServerStats: vi.fn(),
}));

const chat: ChatSummary = {
  chat_jid: CHAT,
  last_message_id: 'm2',
  last_text_content: 'second',
  last_message_type: 'text',
  last_timestamp: '2026-01-02 10:00:00',
  message_count: 2,
} as ChatSummary;

const row = (id: string, text: string, overrides: Partial<HistoryMessage> = {}): HistoryMessage =>
  ({
    id,
    message_id: id,
    timestamp: `2026-01-02 10:00:0${id.length % 10}`,
    status: 'Read',
    chat_jid: CHAT,
    sender_jid: CHAT,
    message_type: 'text',
    text_content: text,
    media_url: '',
    media_mimetype: '',
    quoted_message_id: '',
    is_from_me: false,
    ...overrides,
  }) as HistoryMessage;

// A file of any size without allocating it.
const bigFile = (name: string, type: string, size: number) => {
  const file = new File(['x'], name, { type });
  Object.defineProperty(file, 'size', { value: size });
  return file;
};

const flush = async (ms = 0) => {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
};

const button = (name: string) =>
  [...document.querySelectorAll('button')].find(
    (b) => b.textContent?.includes(name) || b.getAttribute('aria-label') === name
  ) as HTMLButtonElement;

const typeInto = async (input: HTMLInputElement, value: string) => {
  await act(async () => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
};

describe('Messages page', () => {
  let root: Root;
  let host: HTMLDivElement;

  const openChat = async () => {
    const entry = [...document.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('100')
    );
    await act(async () => entry?.click());
    await flush();
  };

  const attach = async (file: File) => {
    const input = document.querySelector<HTMLInputElement>('input[type="file"]');
    Object.defineProperty(input, 'files', { value: [file], configurable: true });
    await act(async () => input?.dispatchEvent(new Event('change', { bubbles: true })));
    await flush();
  };

  const search = async (text: string) => {
    await act(async () => button('Buscar nesta conversa').click());
    const input = document.querySelector<HTMLInputElement>(
      'input[aria-label="Buscar nesta conversa"]'
    );
    await typeInto(input as HTMLInputElement, text);
    await flush(350); // The debounce, which starts the query.
    await flush();
  };

  beforeEach(async () => {
    vi.useFakeTimers();
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        observe() {}
        disconnect() {}
        unobserve() {}
      }
    );
    vi.stubGlobal('CSS', { escape: (value: string) => value });
    Element.prototype.scrollIntoView = vi.fn();
    localStorage.setItem('wamux-lang', 'pt');
    live.status = 'open';
    live.websocketEnable = 'enabled';
    toastInfo.mockClear();
    toastError.mockClear();
    vi.mocked(messagesApi.sendMedia).mockReset();
    vi.mocked(messagesApi.fetchMediaObjectUrl).mockReset().mockResolvedValue('blob:x');
    vi.mocked(messagesApi.getMediaDownloadUrl).mockReset();
    vi.mocked(serverApi.fetchServerStats).mockReset().mockResolvedValue({ system: {}, messages: {} });
    vi.mocked(messagesApi.listChats).mockReset().mockResolvedValue([chat]);
    vi.mocked(messagesApi.getHistory)
      .mockReset()
      .mockResolvedValue([row('m2', 'second'), row('m1', 'first')]);
    vi.mocked(messagesApi.searchHistory).mockReset().mockResolvedValue([]);
    vi.mocked(messagesApi.getSenderNames).mockReset().mockResolvedValue({});

    host = document.createElement('div');
    document.body.appendChild(host);
    root = createRoot(host);
  });

  const render = async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => {
      root.render(
        <QueryClientProvider client={client}>
          <I18nProvider>
            <MemoryRouter>
              <Messages />
            </MemoryRouter>
          </I18nProvider>
        </QueryClientProvider>
      );
    });
    await flush();
  };

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    document.body.innerHTML = '';
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('lists conversations and opens a thread', async () => {
    await render();
    await openChat();

    expect(messagesApi.getHistory).toHaveBeenCalledWith('instance-token', CHAT, 50, undefined);
    expect(document.body.textContent).toContain('first');
    expect(document.body.textContent).toContain('second');
  });

  it('shows an edit as the new text of the original message', async () => {
    vi.mocked(messagesApi.getHistory).mockResolvedValue([
      row('edit-row', 'corrected', { message_type: 'edit', quoted_message_id: 'm1' }),
      row('m1', 'typo'),
    ]);
    await render();
    await openChat();

    expect(document.body.textContent).toContain('corrected');
    expect(document.body.textContent).not.toContain('typo');
    expect(document.querySelectorAll('[data-message-id]')).toHaveLength(1);
  });

  it.each([
    ['pt', 'jan'],
    ['en', 'Jan'],
  ])('shows dates in the chosen language: %s', async (lang, month) => {
    localStorage.setItem('wamux-lang', lang);
    await render();
    await openChat();

    expect(document.body.textContent).toContain(month);
    expect(document.body.textContent).toContain('2026');
    if (lang === 'en') expect(document.body.textContent).toMatch(/\d{1,2}:\d{2}\s[AP]M/);
  });

  it('polls when the instance publishes no events, even with the socket open', async () => {
    live.websocketEnable = 'disabled';
    await render();
    await openChat();
    vi.mocked(messagesApi.listChats).mockClear();
    vi.mocked(messagesApi.getHistory).mockClear();

    await flush(16_000);

    expect(messagesApi.listChats).toHaveBeenCalled();
    expect(messagesApi.getHistory).toHaveBeenCalled();
  });

  it('polls while the socket is down', async () => {
    live.status = 'connecting';
    await render();
    vi.mocked(messagesApi.listChats).mockClear();

    await flush(16_000);

    expect(messagesApi.listChats).toHaveBeenCalled();
  });

  it('does not poll with live events', async () => {
    await render();
    await openChat();
    vi.mocked(messagesApi.listChats).mockClear();
    vi.mocked(messagesApi.getHistory).mockClear();

    await flush(60_000);

    expect(messagesApi.listChats).not.toHaveBeenCalled();
    expect(messagesApi.getHistory).not.toHaveBeenCalled();
  });

  it('searches the open conversation, highlighting the match', async () => {
    vi.mocked(messagesApi.searchHistory).mockResolvedValue([row('m1', 'first hit')]);
    await render();
    await openChat();
    await search('hit');

    expect(messagesApi.searchHistory).toHaveBeenCalledWith(
      'instance-token',
      CHAT,
      'hit',
      30,
      undefined
    );
    expect(document.querySelector('mark')?.textContent).toBe('hit');
  });

  it('does not search a single character', async () => {
    await render();
    await openChat();
    await search('h');

    expect(messagesApi.searchHistory).not.toHaveBeenCalled();
  });

  it('says so when nothing matches', async () => {
    await render();
    await openChat();
    await search('nothing');

    expect(document.body.textContent).toContain('Nenhuma mensagem encontrada.');
  });

  it('loads older pages to open a hit that is not loaded yet', async () => {
    const full = Array.from({ length: 50 }, (_, i) => row(`p${i}`, `row ${i}`));
    vi.mocked(messagesApi.getHistory)
      .mockReset()
      .mockResolvedValueOnce(full)
      .mockResolvedValue([row('old', 'older hit', { timestamp: '2025-01-01 10:00:00' })]);
    vi.mocked(messagesApi.searchHistory).mockResolvedValue([
      row('old', 'older hit', { timestamp: '2025-01-01 10:00:00' }),
    ]);
    await render();
    await openChat();
    await search('older');

    await act(async () => document.querySelector('mark')?.closest('button')?.click());
    await flush();

    expect(messagesApi.getHistory).toHaveBeenCalledTimes(2);
    const bubble = document.querySelector('[data-message-id="old"]');
    expect(bubble?.querySelector('.ring-2')).not.toBeNull();
    expect(Element.prototype.scrollIntoView).toHaveBeenCalled();

    await flush(3000);
    expect(bubble?.querySelector('.ring-2')).toBeNull();
  });

  it('opens the original when the hit is an edit', async () => {
    vi.mocked(messagesApi.searchHistory).mockResolvedValue([
      row('edit-row', 'new text', { message_type: 'edit', quoted_message_id: 'm1' }),
    ]);
    await render();
    await openChat();
    await search('new');

    await act(async () => document.querySelector('mark')?.closest('button')?.click());
    await flush();

    expect(document.querySelector('[data-message-id="m1"] .ring-2')).not.toBeNull();
  });

  it('explains when a hit is too far back to open', async () => {
    let page = 0;
    vi.mocked(messagesApi.getHistory)
      .mockReset()
      .mockImplementation(async () => {
        page += 1;
        return Array.from({ length: 50 }, (_, i) =>
          row(`p${page}-${i}`, 'x', {
            timestamp: `20${String(50 - page).padStart(2, '0')}-01-01 10:00:00`,
          })
        );
      });
    vi.mocked(messagesApi.searchHistory).mockResolvedValue([row('ancient', 'ancient hit')]);
    await render();
    await openChat();
    await search('ancient');

    await act(async () => document.querySelector('mark')?.closest('button')?.click());
    await flush();

    expect(page).toBe(41);
    expect(toastInfo).toHaveBeenCalledOnce();
  });

  it.each([
    ['a large MP4 stays a video', 'clip.mp4', 'video/mp4', 'video'],
    ['a large MKV goes as a document', 'clip.mkv', 'video/x-matroska', 'document'],
    ['a large image goes as a document', 'scan.png', 'image/png', 'document'],
  ])('%s', async (_, name, type, kind) => {
    vi.mocked(messagesApi.sendMedia).mockResolvedValue({});
    await render();
    await openChat();
    await attach(bigFile(name, type, 100 * 1024 * 1024));

    await act(async () => button('Enviar').click());
    await flush();

    expect(vi.mocked(messagesApi.sendMedia).mock.calls[0][1].type).toBe(kind);
  });

  it('refuses a file over the server limit before uploading it', async () => {
    vi.mocked(serverApi.fetchServerStats).mockResolvedValue({
      system: {},
      messages: {},
      storage: { maxUploadBytes: 1024 * 1024 },
    });
    await render();
    await openChat();
    await attach(bigFile('big.bin', 'application/zip', 2 * 1024 * 1024));

    await act(async () => button('Enviar').click());
    await flush();

    expect(messagesApi.sendMedia).not.toHaveBeenCalled();
    expect(toastError).toHaveBeenCalledWith('Este arquivo passa do limite de 1.0 MB.');
  });

  const fileRow = (overrides: Partial<HistoryMessage> = {}) =>
    row('doc', '', {
      message_type: 'document',
      media_url: '/chat/media/doc',
      media_mimetype: 'application/pdf',
      file_name: 'report.final.pdf',
      file_size: 2 * 1024 * 1024,
      ...overrides,
    });

  it('shows a file with its name, type, size and a download button', async () => {
    vi.mocked(messagesApi.getHistory).mockResolvedValue([fileRow()]);
    await render();
    await openChat();

    expect(document.body.textContent).toContain('report.final.pdf');
    expect(document.body.textContent).toContain('PDF · 2.0 MB');
    expect(document.querySelector('[aria-label="Baixar report.final.pdf"]')).not.toBeNull();
  });

  // A 1.9 GB document must not reach the browser just because its chat was opened.
  it('does not fetch a file until it is downloaded', async () => {
    vi.mocked(messagesApi.getHistory).mockResolvedValue([
      fileRow(),
      row('big', '', {
        message_type: 'video',
        media_url: '/chat/media/big',
        media_mimetype: 'video/mp4',
        file_size: 500 * 1024 * 1024,
      }),
    ]);
    await render();
    await openChat();

    expect(messagesApi.fetchMediaObjectUrl).not.toHaveBeenCalled();
  });

  it('downloads through a link the browser can open', async () => {
    vi.mocked(messagesApi.getHistory).mockResolvedValue([fileRow()]);
    vi.mocked(messagesApi.getMediaDownloadUrl).mockResolvedValue('http://api.test/download/doc?sig=x');
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
    await render();
    await openChat();

    await act(async () =>
      document.querySelector<HTMLButtonElement>('[aria-label="Baixar report.final.pdf"]')?.click()
    );
    await flush();

    expect(messagesApi.getMediaDownloadUrl).toHaveBeenCalledWith('instance-token', expect.anything());
    expect(click).toHaveBeenCalledOnce();
    click.mockRestore();
  });

  it('explains a failed download', async () => {
    vi.mocked(messagesApi.getHistory).mockResolvedValue([fileRow()]);
    vi.mocked(messagesApi.getMediaDownloadUrl).mockRejectedValue(new Error('gone'));
    await render();
    await openChat();

    await act(async () =>
      document.querySelector<HTMLButtonElement>('[aria-label="Baixar report.final.pdf"]')?.click()
    );
    await flush();

    expect(toastError).toHaveBeenCalledWith('Não foi possível baixar o arquivo.');
  });

  it('still describes a file that was not stored', async () => {
    vi.mocked(messagesApi.getHistory).mockResolvedValue([fileRow({ media_url: '' })]);
    await render();
    await openChat();

    expect(document.body.textContent).toContain('report.final.pdf');
    expect(document.body.textContent).toContain('Não salvo');
    expect(document.querySelector('[aria-label="Baixar report.final.pdf"]')).toBeNull();
  });

  it('does not repeat a caption that is just the file name', async () => {
    vi.mocked(messagesApi.getHistory).mockResolvedValue([fileRow({ text_content: 'report.final.pdf' })]);
    await render();
    await openChat();

    const bubble = document.querySelector('[data-message-id="doc"]');
    expect(bubble?.textContent?.match(/report\.final\.pdf/g)).toHaveLength(1);
  });

  it('shows the length of a document', async () => {
    vi.mocked(messagesApi.getHistory).mockResolvedValue([fileRow({ page_count: 12 })]);
    await render();
    await openChat();

    expect(document.querySelector('[data-message-id="doc"]')?.textContent).toContain(
      '12 páginas · PDF · 2.0 MB'
    );
  });

  describe('document previews', () => {
    const bubbleImage = () => document.querySelector('[data-message-id="doc"] img:not([hidden])');

    // Without an observer the card counts as on screen, so the preview loads.
    beforeEach(() => vi.stubGlobal('IntersectionObserver', undefined));

    it('loads the rendered first page of a PDF', async () => {
      vi.mocked(messagesApi.getHistory).mockResolvedValue([fileRow()]);
      vi.mocked(messagesApi.fetchMediaObjectUrl).mockResolvedValue('blob:page');
      await render();
      await openChat();

      expect(messagesApi.fetchMediaObjectUrl).toHaveBeenCalledWith('instance-token', '/chat/media/doc/preview');
      expect(bubbleImage()?.getAttribute('src')).toBe('blob:page');
    });

    it('never fetches the file itself', async () => {
      vi.mocked(messagesApi.getHistory).mockResolvedValue([fileRow()]);
      await render();
      await openChat();

      expect(messagesApi.fetchMediaObjectUrl).not.toHaveBeenCalledWith('instance-token', '/chat/media/doc');
    });

    it('asks for no render of other documents', async () => {
      vi.mocked(messagesApi.getHistory).mockResolvedValue([
        fileRow({ media_mimetype: 'application/zip', file_name: 'a.zip' }),
      ]);
      await render();
      await openChat();

      expect(messagesApi.fetchMediaObjectUrl).not.toHaveBeenCalled();
    });

    // WhatsApp's own can be 21x30, which stretched over the card is a blur.
    it.each([
      ['hides a tiny inline preview', 21, null],
      ['uses a real inline preview', 300, 'data:image/jpeg;base64,AAAA'],
    ])('%s when the render fails', async (_, width, expected) => {
      // Its own id, because a successful preview from another test stays cached.
      const id = `doc-${width}`;
      vi.mocked(messagesApi.getHistory).mockResolvedValue([
        fileRow({ message_id: id, file_thumbnail: 'AAAA' }),
      ]);
      vi.mocked(messagesApi.fetchMediaObjectUrl).mockRejectedValue(new Error('no preview'));
      await render();
      await openChat();

      const bubble = document.querySelector(`[data-message-id="${id}"]`);
      const hidden = bubble?.querySelector<HTMLImageElement>('img[hidden]');
      Object.defineProperty(hidden, 'naturalWidth', { value: width });
      await act(async () => hidden?.dispatchEvent(new Event('load')));

      expect(bubble?.querySelector('img:not([hidden])')?.getAttribute('src') ?? null).toBe(expected);
    });
  });

  it('marks forwarded messages, and only those', async () => {
    vi.mocked(messagesApi.getHistory).mockResolvedValue([
      row('m2', 'passed on', { is_forwarded: true }),
      row('m1', 'mine'),
    ]);
    await render();
    await openChat();

    expect(document.querySelector('[data-message-id="m2"]')?.textContent).toContain('Encaminhada');
    expect(document.querySelector('[data-message-id="m1"]')?.textContent).not.toContain('Encaminhada');
  });

  it('closes the search when another conversation opens', async () => {
    await render();
    await openChat();
    await search('text');
    expect(document.querySelector('input[aria-label="Buscar nesta conversa"]')).not.toBeNull();

    await openChat();

    expect(document.querySelector('input[aria-label="Buscar nesta conversa"]')).toBeNull();
  });
});
