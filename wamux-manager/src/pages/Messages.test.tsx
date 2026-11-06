import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/i18n/I18nContext';
import * as messagesApi from '@/services/api/messages';
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
  toast: { info: toastInfo, success: vi.fn(), error: vi.fn() },
}));

vi.mock('@/services/api/messages', async (original) => ({
  ...(await original<typeof messagesApi>()),
  listChats: vi.fn(),
  getHistory: vi.fn(),
  searchHistory: vi.fn(),
  getSenderNames: vi.fn(),
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

  it('closes the search when another conversation opens', async () => {
    await render();
    await openChat();
    await search('text');
    expect(document.querySelector('input[aria-label="Buscar nesta conversa"]')).not.toBeNull();

    await openChat();

    expect(document.querySelector('input[aria-label="Buscar nesta conversa"]')).toBeNull();
  });
});
