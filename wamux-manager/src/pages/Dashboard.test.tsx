import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/i18n/I18nContext';
import * as instancesApi from '@/services/api/instances';
import * as serverApi from '@/services/api/server';
import type { Instance, InstanceOverview } from '@/types/instance';
import type { LogEntry } from '@/utils/instanceLogs';
import Dashboard from './Dashboard';

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

vi.mock('@/services/api/server', async (original) => ({
  ...(await original<typeof serverApi>()),
  fetchServerStats: vi.fn(),
}));

vi.mock('@/services/api/instances', async (original) => ({
  ...(await original<typeof instancesApi>()),
  fetchInstances: vi.fn(),
  fetchInstanceOverview: vi.fn(),
  fetchInstanceLogs: vi.fn(),
}));

const online = {
  id: 'a',
  instanceName: 'Sales',
  status: 'open',
  owner: '5511999990000',
  connected: true,
  osName: 'WaMux',
  clientName: 'wamux',
  createdAt: '2026-01-02T03:04:05Z',
} as Instance;

const offline = {
  id: 'b',
  instanceName: 'Support',
  status: 'close',
  owner: '',
  connected: false,
  disconnectReason: 'logged out',
} as Instance;

const stats: serverApi.ServerStats = {
  system: { version: '1.2.3' },
  messages: {
    total: 30,
    byDay: [{ key: '2026-01-02', count: 30 }],
    topSources: [
      { key: '100', count: 20, name: 'Ana', phone: '5511999990001' },
      { key: '5511999990002', count: 10 },
      { key: 'status', count: 5 },
    ],
  },
};

const log = (level: string, message: string): LogEntry => ({
  timestamp: '2026-01-02T03:04:05Z',
  level,
  instance_id: 'a',
  message: `[a] ${message}`,
});

const flush = async (ms = 0) => {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
};

const text = () => document.body.textContent ?? '';
const byLabel = <T extends Element>(label: string) =>
  document.querySelector<T>(`[aria-label="${label}"]`) as T;
const button = (name: string) =>
  [...document.querySelectorAll('button')].find((b) =>
    b.textContent?.includes(name)
  ) as HTMLButtonElement;

const logsApi = () => vi.mocked(instancesApi.fetchInstanceLogs);

describe('Dashboard page', () => {
  let root: Root;
  let host: HTMLDivElement;

  const render = async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    await act(async () => {
      root.render(
        <QueryClientProvider client={client}>
          <I18nProvider>
            <MemoryRouter>
              <Dashboard />
            </MemoryRouter>
          </I18nProvider>
        </QueryClientProvider>
      );
    });
    await flush();
    await flush(1); // The overviews start once the instances arrive.
  };

  const openLogs = async () => {
    await render();
    await act(async () => button('Sales').click());
    await flush();
  };

  const type = async (input: HTMLInputElement, value: string) => {
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(input, value);
      input.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await flush();
  };

  const change = async (select: HTMLSelectElement, value: string) => {
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')?.set?.call(select, value);
      select.dispatchEvent(new Event('change', { bubbles: true }));
    });
    await flush();
  };

  beforeEach(() => {
    vi.useFakeTimers();
    localStorage.clear();
    localStorage.setItem('wamux-lang', 'pt');
    vi.mocked(serverApi.fetchServerStats).mockReset().mockResolvedValue(stats);
    vi.mocked(instancesApi.fetchInstances).mockReset().mockResolvedValue([online, offline]);
    vi.mocked(instancesApi.fetchInstanceOverview)
      .mockReset()
      .mockResolvedValue({ connected: true, contactsCount: 42 } as InstanceOverview);
    logsApi().mockReset().mockResolvedValue([log('INFO', 'Connected'), log('ERROR', 'Boom')]);

    host = document.createElement('div');
    document.body.appendChild(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    document.body.innerHTML = '';
    vi.useRealTimers();
  });

  it('has no iframe and copies no credentials', async () => {
    localStorage.setItem('wamux-auth', '{"state":{"apiKey":"k","apiUrl":"http://a"}}');
    await render();

    expect(document.querySelector('iframe')).toBeNull();
    expect(localStorage.getItem('egogo_dash_key')).toBeNull();
  });

  it('formats numbers in the chosen language', async () => {
    const big = { ...stats, messages: { ...stats.messages, total: 1234567 } };
    vi.mocked(serverApi.fetchServerStats).mockResolvedValue(big);

    await render();
    expect(text()).toContain('1.234.567');

    act(() => root.unmount());
    localStorage.setItem('wamux-lang', 'en');
    root = createRoot(host);
    await render();
    expect(text()).toContain('1,234,567');
  });

  it('shows the connection split', async () => {
    await render();

    expect(byLabel('1 de 2 instâncias conectadas').textContent).toBe('50%');
    expect(text()).toContain('Desconectadas');
  });

  it('labels the top conversations', async () => {
    await render();

    expect(text()).toContain('Ana');
    expect(text()).toContain('+5511999990001');
    expect(text()).toContain('+5511999990002');
    expect(text()).toContain('Status');
  });

  it('lists instances with what the overview adds', async () => {
    await render();
    const rows = [...document.querySelectorAll('tbody tr')];

    expect(rows).toHaveLength(2);
    expect(rows[0].textContent).toContain('+5511999990000');
    expect(rows[0].textContent).toContain('42');
    expect(rows[1].textContent).toContain('logged out');
    expect(instancesApi.fetchInstanceOverview).toHaveBeenCalledTimes(1);
  });

  it('asks for no logs until an instance is selected', async () => {
    await render();

    expect(logsApi()).not.toHaveBeenCalled();
    expect(text()).toContain('Clique numa instância');
  });

  it('opens logs inside the instances card, and closes them', async () => {
    await openLogs();

    expect(logsApi()).toHaveBeenCalledWith('a', { limit: 100, level: undefined });
    expect(text()).toContain('Logs de Sales');
    expect(text()).toContain('Boom');
    expect(text()).not.toContain('[a]');
    expect(button('Sales').getAttribute('aria-pressed')).toBe('true');

    await act(async () => byLabel<HTMLButtonElement>('Fechar').click());

    expect(text()).not.toContain('Logs de Sales');
  });

  it('filters levels on the server', async () => {
    await openLogs();
    logsApi().mockClear();

    for (const level of ['INFO', 'WARN', 'DEBUG']) {
      await act(async () => button(level).click());
      await flush();
    }

    expect(logsApi()).toHaveBeenLastCalledWith('a', { limit: 100, level: 'ERROR' });
  });

  it('treats no level selected as all of them', async () => {
    await openLogs();

    for (const level of ['INFO', 'WARN', 'DEBUG', 'ERROR']) {
      await act(async () => button(level).click());
      await flush();
    }

    expect(button('ERROR').getAttribute('aria-pressed')).toBe('true');
    expect(logsApi()).toHaveBeenLastCalledWith('a', { limit: 100, level: undefined });
  });

  it('counts levels on their chips', async () => {
    await openLogs();

    expect(button('ERROR').textContent).toContain('1');
    expect(button('INFO').textContent).toContain('1');
  });

  it('loads more lines on request', async () => {
    await openLogs();

    await change(byLabel<HTMLSelectElement>('Quantidade de linhas'), '500');

    expect(logsApi()).toHaveBeenLastCalledWith('a', { limit: 500, level: undefined });
  });

  it('filters text on the loaded lines, highlighting matches', async () => {
    await openLogs();
    logsApi().mockClear();

    await type(byLabel<HTMLInputElement>('Filtrar texto…'), 'boom');

    expect(logsApi()).not.toHaveBeenCalled();
    expect(document.querySelector('[role="log"] mark')?.textContent).toBe('Boom');
    expect(document.querySelector('[role="log"]')?.textContent).not.toContain('Connected');
    expect(text()).toContain('1 de 2 entradas');

    await type(byLabel<HTMLInputElement>('Filtrar texto…'), 'absent');
    expect(text()).toContain('Nenhum log com esse texto.');
  });

  it('refreshes every 5 seconds only while live', async () => {
    await openLogs();
    logsApi().mockClear();

    await flush(20_000);
    expect(logsApi()).not.toHaveBeenCalled();

    await act(async () => document.querySelector<HTMLInputElement>('[type=checkbox]')?.click());
    await flush(5_100);
    expect(logsApi()).toHaveBeenCalledTimes(1);

    await act(async () => byLabel<HTMLButtonElement>('Fechar').click());
    logsApi().mockClear();
    await flush(20_000);
    expect(logsApi()).not.toHaveBeenCalled();
  });

  it('renders log text as text, never as HTML', async () => {
    logsApi().mockResolvedValue([log('INFO', '<img src=x onerror="window.hacked=1">')]);
    await openLogs();

    const panel = document.querySelector('[role="log"]') as HTMLElement;
    expect(panel.querySelector('img')).toBeNull();
    expect(panel.textContent).toContain('<img src=x');
  });

  it('shows headings without hiding their level', async () => {
    logsApi().mockResolvedValue([log('INFO', '===== SENT ====='), log('WARN', '===== SKIPPED =====')]);
    await openLogs();
    const line = (name: string) =>
      [...document.querySelectorAll('[role="log"] span')].find((el) => el.textContent === name);

    expect(line('SENT')?.className).toContain('text-primary');
    expect(line('SKIPPED')?.className).toContain('text-amber');
    expect(line('SKIPPED')?.className).not.toContain('text-primary');
  });

  it('collapses a long message until it is clicked', async () => {
    logsApi().mockResolvedValue([log('INFO', 'x'.repeat(500))]);
    await openLogs();
    const line = document.querySelector('[role="log"] button') as HTMLButtonElement;

    expect(line.getAttribute('aria-expanded')).toBe('false');
    expect(line.className).toContain('line-clamp-3');

    await act(async () => line.click());

    expect(line.getAttribute('aria-expanded')).toBe('true');
    expect(line.className).not.toContain('line-clamp-3');
  });

  it('keeps a message open across a refresh that adds lines', async () => {
    const long = log('INFO', 'x'.repeat(500));
    logsApi().mockResolvedValue([long]);
    await openLogs();
    await act(async () => (document.querySelector('[role="log"] button') as HTMLElement).click());

    logsApi().mockResolvedValue([log('INFO', 'newer'), long]);
    await act(async () => byLabel<HTMLButtonElement>('Atualizar').click());
    await flush();

    const lines = document.querySelectorAll('[role="log"] button[aria-expanded]');
    expect(lines).toHaveLength(1);
    expect(lines[0].getAttribute('aria-expanded')).toBe('true');
  });

  it('never shows one instance\'s logs under another', async () => {
    await openLogs();
    let release: (rows: LogEntry[]) => void = () => {};
    logsApi().mockReturnValue(new Promise((resolve) => (release = resolve)));

    await act(async () => button('Support').click());
    await flush();

    expect(text()).toContain('Logs de Support');
    expect(text()).not.toContain('Boom');

    await act(async () => release([]));
    await flush();
  });

  it('says so when logs fail or are empty', async () => {
    logsApi().mockRejectedValue({ message: 'server down' });
    await openLogs();
    expect(text()).toContain('Erro ao carregar logs: server down');
  });

  it('says so when there are no logs', async () => {
    logsApi().mockResolvedValue([]);
    await openLogs();
    expect(text()).toContain('Sem logs.');
  });

  it('polls on the chosen interval, and not in manual mode', async () => {
    await render();
    const stat = vi.mocked(serverApi.fetchServerStats);

    await change(byLabel<HTMLSelectElement>('Atualização automática'), '0');
    stat.mockClear();
    await flush(120_000);
    expect(stat).not.toHaveBeenCalled();

    await act(async () => byLabel<HTMLButtonElement>('Atualizar').click());
    await flush();
    expect(stat).toHaveBeenCalledTimes(1);

    await change(byLabel<HTMLSelectElement>('Atualização automática'), '10');
    stat.mockClear();
    await flush(10_100);
    expect(stat).toHaveBeenCalled();
  });

  it('shows the server as offline when stats fail', async () => {
    vi.mocked(serverApi.fetchServerStats).mockRejectedValue(new Error('down'));
    await render();

    expect(document.querySelector('[role="status"]')?.textContent).toContain('servidor offline');
  });
});
