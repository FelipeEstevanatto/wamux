import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/i18n/I18nContext';
import * as instancesApi from '@/services/api/instances';
import type { Instance } from '@/types/instance';
import InstanceSettings from './InstanceSettings';

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

vi.mock('sonner', () => ({
  toast: { info: vi.fn(), success: vi.fn(), error: vi.fn() },
}));

vi.mock('@/services/api/instances', async (original) => ({
  ...(await original<typeof instancesApi>()),
  fetchInstance: vi.fn(),
  fetchInstanceOverview: vi.fn(),
  getProxy: vi.fn(),
}));

const instance = {
  id: 'inst-1',
  instanceName: 'Desk',
  status: 'close',
  owner: '',
  apikey: 'token',
  connected: false,
  webhook: 'https://example.com/hook',
  events: 'MESSAGE',
  rabbitmqEnable: '',
  websocketEnable: 'enabled',
  natsEnable: '',
  alwaysOnline: false,
  rejectCall: false,
  readMessages: false,
  ignoreGroups: false,
  ignoreStatus: false,
} as Instance;

const section = (id: string) =>
  document.querySelector(`button[data-section="${id}"]`);

describe('Instance settings sections', () => {
  let root: Root;
  let host: HTMLDivElement;

  beforeEach(() => {
    localStorage.setItem('wamux-lang', 'pt');
    vi.mocked(instancesApi.fetchInstance).mockResolvedValue(instance);
    vi.mocked(instancesApi.fetchInstanceOverview).mockResolvedValue({
      connected: false,
      contactsCount: 0,
      chatsCount: 0,
      messagesCount: 0,
    });
    vi.mocked(instancesApi.getProxy).mockResolvedValue(null);
    host = document.createElement('div');
    document.body.appendChild(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    localStorage.clear();
  });

  const render = async () => {
    await act(async () => {
      root.render(
        <I18nProvider>
          <MemoryRouter initialEntries={['/manager/instances/inst-1/settings']}>
            <Routes>
              <Route
                path="/manager/instances/:instanceId/settings"
                element={<InstanceSettings />}
              />
            </Routes>
          </MemoryRouter>
        </I18nProvider>
      );
    });
    await act(async () => {
      await Promise.resolve();
    });
  };

  const typeInto = async (input: HTMLInputElement, value: string) => {
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(
        input,
        value
      );
      input.dispatchEvent(new Event('input', { bubbles: true }));
      input.dispatchEvent(new Event('change', { bubbles: true }));
    });
  };

  it('lists the sections and marks only the form that changed', async () => {
    await render();

    for (const id of ['connection', 'webhooks', 'events', 'proxy', 'behavior', 'danger']) {
      expect(section(id)?.textContent).toBeTruthy();
      expect(section(id)?.getAttribute('data-unsaved')).toBe('false');
    }

    const url = document.querySelector<HTMLInputElement>('#webhookUrl');
    await typeInto(url as HTMLInputElement, 'https://example.com/other');

    expect(section('webhooks')?.getAttribute('data-unsaved')).toBe('true');
    expect(document.getElementById('webhooks')?.textContent).toContain('Não salvo');
    expect(section('events')?.getAttribute('data-unsaved')).toBe('false');
    expect(section('proxy')?.getAttribute('data-unsaved')).toBe('false');
    expect(section('behavior')?.getAttribute('data-unsaved')).toBe('false');

    const save = [...document.querySelectorAll('button')].find((button) =>
      button.textContent?.includes('Salvar Webhook')
    ) as HTMLButtonElement;
    const saveEvents = [...document.querySelectorAll('button')].find((button) =>
      button.textContent?.includes('Salvar eventos')
    ) as HTMLButtonElement;
    expect(save.disabled).toBe(false);
    expect(saveEvents.disabled).toBe(true);

    const call = [...document.querySelectorAll('label')].find((label) =>
      label.textContent?.includes('Chamada')
    );
    await act(async () => {
      call?.querySelector('input')?.click();
    });
    expect(section('events')?.getAttribute('data-unsaved')).toBe('true');
    expect(document.getElementById('events')?.textContent).toContain('Não salvo');
    expect(saveEvents.disabled).toBe(false);
  });
});
