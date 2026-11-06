import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/i18n/I18nContext';
import * as instancesApi from '@/services/api/instances';
import CreateInstanceModal from './CreateInstanceModal';

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

vi.mock('sonner', () => ({ toast: { info: vi.fn(), success: vi.fn(), error: vi.fn() } }));

vi.mock('@/services/api/instances', async (original) => ({
  ...(await original<typeof instancesApi>()),
  createInstance: vi.fn(),
  fetchInstances: vi.fn(),
}));

describe('CreateInstanceModal', () => {
  let root: Root;

  beforeEach(() => {
    localStorage.setItem('wamux-lang', 'pt');
    vi.mocked(instancesApi.createInstance).mockReset().mockResolvedValue({} as never);
    vi.mocked(instancesApi.fetchInstances).mockReset().mockResolvedValue([]);
    root = createRoot(document.body.appendChild(document.createElement('div')));
  });

  afterEach(() => {
    act(() => root.unmount());
    document.body.innerHTML = '';
  });

  // The Events and Messages pages read this feed, and a new instance has it off.
  it('creates instances that publish to the WebSocket', async () => {
    await act(async () => {
      root.render(
        <QueryClientProvider client={new QueryClient()}>
          <I18nProvider>
            <CreateInstanceModal open onOpenChange={() => {}} />
          </I18nProvider>
        </QueryClientProvider>
      );
    });

    const name = document.querySelector<HTMLInputElement>('input[name="instanceName"]');
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set?.call(name, 'sales');
      name?.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await act(async () => {
      document.querySelector('form')?.requestSubmit();
    });
    await vi.waitFor(() => expect(instancesApi.createInstance).toHaveBeenCalled());

    expect(vi.mocked(instancesApi.createInstance).mock.calls[0][0]).toMatchObject({
      name: 'sales',
      websocketEnable: 'enabled',
    });
  });
});
