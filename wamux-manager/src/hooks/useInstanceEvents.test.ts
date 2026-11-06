import { act, createElement } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import apiClient from '@/services/api/client';
import useInstanceEvents, { ALL_INSTANCES, type WsStatus } from './useInstanceEvents';

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT =
  true;

const API_KEY = 'global-key-value';

vi.mock('@/hooks/useAuth', () => ({
  default: () => ({ apiUrl: 'http://api.test', apiKey: API_KEY }),
}));

class FakeSocket {
  static opened: FakeSocket[] = [];
  onopen?: () => void;
  onclose?: (() => void) | null;
  onmessage?: (event: { data: string }) => void;
  onerror?: () => void;
  close = vi.fn();
  constructor(public url: string) {
    FakeSocket.opened.push(this);
  }
}

describe('useInstanceEvents', () => {
  let root: Root;
  let status: WsStatus;
  const post = vi.spyOn(apiClient, 'post');

  const Probe = ({ id }: { id: string }) => {
    status = useInstanceEvents(id, () => {});
    return null;
  };

  const mount = async (id: string) => {
    await act(async () => {
      root.render(createElement(Probe, { id }));
    });
  };

  beforeEach(() => {
    vi.useFakeTimers();
    FakeSocket.opened = [];
    vi.stubGlobal('WebSocket', FakeSocket);
    post.mockReset();
    post.mockResolvedValue({ data: { ticket: 'ticket-1' } });
    root = createRoot(document.createElement('div'));
  });

  afterEach(() => {
    act(() => root.unmount());
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('opens with a ticket and keeps the API key out of the URL', async () => {
    await mount('inst-1');

    expect(post).toHaveBeenCalledOnce();
    expect(post.mock.calls[0][2]?.headers).toEqual({ apikey: API_KEY });
    expect(FakeSocket.opened).toHaveLength(1);
    const { url } = FakeSocket.opened[0];
    expect(url).toBe('ws://api.test/ws?ticket=ticket-1&instanceId=inst-1');
    expect(url).not.toContain(API_KEY);
  });

  it('leaves the instance out of the URL for all instances', async () => {
    await mount(ALL_INSTANCES);
    expect(FakeSocket.opened[0].url).toBe('ws://api.test/ws?ticket=ticket-1');
  });

  it('gets a new ticket for each reconnect', async () => {
    await mount('inst-1');
    post.mockResolvedValue({ data: { ticket: 'ticket-2' } });

    await act(async () => {
      FakeSocket.opened[0].onclose?.();
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(post).toHaveBeenCalledTimes(2);
    expect(FakeSocket.opened[1].url).toContain('ticket=ticket-2');
  });

  it('retries when a ticket cannot be fetched', async () => {
    post.mockRejectedValueOnce(new Error('offline'));
    await mount('inst-1');
    expect(FakeSocket.opened).toHaveLength(0);
    expect(status).toBe('connecting');

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(FakeSocket.opened).toHaveLength(1);
  });

  it('does not open a socket once unmounted', async () => {
    let release: (value: unknown) => void = () => {};
    post.mockReturnValue(new Promise((resolve) => (release = resolve)));
    await mount('inst-1');

    await act(async () => {
      root.unmount();
      release({ data: { ticket: 'late' } });
    });
    root = createRoot(document.createElement('div'));

    expect(FakeSocket.opened).toHaveLength(0);
  });
});
