import { useEffect, useRef, useState } from 'react';
import useAuth from '@/hooks/useAuth';
import type { InstanceEvent } from '@/types/messages';

export type WsStatus = 'connecting' | 'open' | 'closed';

/**
 * Parses one /ws frame. The server sends `{ queue, payload }` where `payload`
 * is the event JSON serialized as a string.
 */
function parseFrame(raw: string): InstanceEvent | null {
  try {
    const frame = JSON.parse(raw) as { queue?: unknown; payload?: unknown };
    const payload =
      typeof frame.payload === 'string' ? JSON.parse(frame.payload) : frame.payload;
    if (!payload || typeof payload !== 'object') return null;

    const event = payload as Record<string, unknown>;
    const data =
      event.data && typeof event.data === 'object'
        ? (event.data as Record<string, unknown>)
        : {};

    return {
      queue: typeof frame.queue === 'string' ? frame.queue : '',
      event: typeof event.event === 'string' ? event.event : '',
      data,
      raw: event,
    };
  } catch {
    return null;
  }
}

/**
 * Subscribes to one instance's events over /ws and reports the connection
 * status.
 *
 * The socket is authenticated with the global API key (the manager's stored
 * credential) and scoped to `instanceId`, so it only receives that instance's
 * events. It reconnects with a capped backoff. `onEvent` is held in a ref so a
 * changing callback never forces a reconnect. The returned status lets the UI
 * warn the operator when live updates are down.
 */
export default function useInstanceEvents(
  instanceId: string | undefined,
  onEvent: (event: InstanceEvent) => void
): WsStatus {
  const { apiUrl, apiKey } = useAuth();
  const handler = useRef(onEvent);
  const [status, setStatus] = useState<WsStatus>('closed');

  // Keep the latest callback in the ref on every render so a changing callback
  // never forces the socket to reconnect.
  useEffect(() => {
    handler.current = onEvent;
  });

  useEffect(() => {
    if (!instanceId || !apiKey || !apiUrl) {
      setStatus('closed');
      return;
    }

    const wsUrl = `${apiUrl.replace(/^http/, 'ws')}/ws?token=${encodeURIComponent(
      apiKey
    )}&instanceId=${encodeURIComponent(instanceId)}`;

    let socket: WebSocket | null = null;
    let stopped = false;
    let attempt = 0;
    let retryTimer: number | undefined;

    const connect = () => {
      setStatus('connecting');
      socket = new WebSocket(wsUrl);

      socket.onopen = () => {
        attempt = 0;
        setStatus('open');
      };

      socket.onmessage = (ev: MessageEvent<string>) => {
        const parsed = parseFrame(ev.data);
        if (parsed) handler.current(parsed);
      };

      socket.onclose = () => {
        if (stopped) return;
        setStatus('connecting');
        attempt = Math.min(attempt + 1, 6);
        retryTimer = window.setTimeout(connect, attempt * 1000);
      };

      socket.onerror = () => {
        // onclose fires next and schedules the retry.
        socket?.close();
      };
    };

    connect();

    return () => {
      stopped = true;
      if (retryTimer) window.clearTimeout(retryTimer);
      socket?.close();
    };
  }, [apiUrl, apiKey, instanceId]);

  return status;
}
