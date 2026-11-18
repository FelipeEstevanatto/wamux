import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/i18n/I18nContext';
import type { Instance, InstanceOverview } from '@/types/instance';
import InstanceCard from './InstanceCard';

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

const instance: Instance = {
  id: 'desk 1',
  instanceName: 'Desk',
  status: 'open',
  owner: '',
  connected: true,
};

const overview: InstanceOverview = {
  connected: true,
  contactsCount: 1000,
  chatsCount: 2,
  messagesCount: 3,
};

describe('InstanceCard', () => {
  let root: Root;
  let host: HTMLDivElement;
  const onOpen = vi.fn();

  beforeEach(() => {
    onOpen.mockClear();
    host = document.createElement('div');
    document.body.appendChild(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    localStorage.clear();
  });

  const render = async (lang: 'pt' | 'en') => {
    localStorage.setItem('wamux-lang', lang);
    await act(async () => {
      root.render(
        <I18nProvider>
          <MemoryRouter>
            <InstanceCard
              instance={instance}
              overview={overview}
              onOpen={onOpen}
              onSettings={() => {}}
              onDelete={() => {}}
              onConnect={() => {}}
              onDisconnect={() => {}}
            />
          </MemoryRouter>
        </I18nProvider>
      );
    });
  };

  it('opens that instance’s messages and keeps the actions visible', async () => {
    await render('pt');

    const link = document.querySelector('a');
    expect(link?.getAttribute('href')).toBe(
      '/manager/messages?instance=desk%201'
    );
    expect(link?.closest('[data-slot="card"]')?.className).not.toContain('opacity-0');
    expect(document.body.textContent).toContain('Desconectar');

    const card = document.querySelector('[data-slot="card"]') as HTMLElement;
    await act(async () => card.click());
    expect(onOpen).toHaveBeenCalledWith(instance);

    onOpen.mockClear();
    const disconnect = [...document.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('Desconectar')
    );
    await act(async () => disconnect?.click());
    expect(onOpen).not.toHaveBeenCalled();
  });

  it('formats counts in the active language', async () => {
    await render('en');
    expect(document.body.textContent).toContain('1,000');

    act(() => root.unmount());
    root = createRoot(host);
    await render('pt');
    expect(document.body.textContent).toContain('1.000');
  });
});
