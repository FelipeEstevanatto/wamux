import { useEffect, useRef, useState } from 'react';
import useAuth from '@/hooks/useAuth';
import apiClient from '@/services/api/client';
import type { InstanceEvent } from '@/types/messages';

export type WsStatus = 'connecting' | 'open' | 'closed';

/** Pass as the instance to receive the events of every instance. */
export const ALL_INSTANCES = '*';

/**
 * Parses one /ws frame. The server sends `{ queue, payload }` where `payload`
 * is the event JSON serialized as a string.
 */
export function parseFrame(raw: string): InstanceEvent | null {
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
 * Subscribes to one instance's events over /ws, or to all of them with
 * ALL_INSTANCES, and reports the connection status.
 *
 * The socket opens with a short-lived ticket, because a browser can only send
 * credentials in the URL and the global API key must not end up in proxy logs.
 * Each attempt gets a new ticket. It reconnects with a capped backoff. `onEvent` is held in a ref so a
 * changing callback never forces a reconnect. The returned status lets the UI
 * warn the operator when live updates are down.
 */
export default function useInstanceEvents(
  instanceId: string | undefined,
  onEvent: (event: InstanceEvent) => void
): WsStatus {
  const { apiUrl, apiKey } = useAuth();
  const handler = useRef(onEvent);
  // The only stored connection state is whether the socket for a given target
  // is open. Tagging it with the target key means a switch of instance/URL is
  // reported as "connecting" immediately, with no state written during the
  // effect body or its cleanup.
  const targetKey = `${apiUrl ?? ''}|${instanceId ?? ''}|${apiKey ?? ''}`;
  const [conn, setConn] = useState<{ key: string; open: boolean }>({
    key: '',
    open: false,
  });

  // Keep the latest callback in the ref on every render so a changing callback
  // never forces the socket to reconnect.
  useEffect(() => {
    handler.current = onEvent;
  });

  useEffect(() => {
    if (!instanceId || !apiKey || !apiUrl) return;

    const scope =
      instanceId === ALL_INSTANCES ? '' : `&instanceId=${encodeURIComponent(instanceId)}`;

    let stopped = false;
    let attempt = 0;
    let retryTimer: number | undefined;
    let ws: WebSocket | null = null;

    const retry = () => {
      attempt = Math.min(attempt + 1, 6);
      retryTimer = window.setTimeout(() => void connect(), attempt * 1000);
    };

    const connect = async () => {
      if (stopped) return;

      let ticket: string;
      try {
        const response = await apiClient.post<{ ticket: string }>('/ws/ticket', null, {
          baseURL: apiUrl,
          headers: { apikey: apiKey },
        });
        ticket = response.data.ticket;
      } catch {
        if (!stopped) retry();
        return;
      }
      if (stopped) return;

      ws = new WebSocket(
        `${apiUrl.replace(/^http/, 'ws')}/ws?ticket=${encodeURIComponent(ticket)}${scope}`
      );

      // State is written only from the socket's own callbacks, never
      // synchronously in this effect body. Between attempts the public status is
      // the derived "connecting" below, so no extra render is needed to leave
      // "closed".
      ws.onopen = () => {
        attempt = 0;
        setConn({ key: targetKey, open: true });
      };

      ws.onmessage = (ev: MessageEvent<string>) => {
        const parsed = parseFrame(ev.data);
        if (parsed) handler.current(parsed);
      };

      ws.onclose = () => {
        if (stopped) return;
        setConn({ key: targetKey, open: false });
        retry();
      };

      ws.onerror = () => {
        // onclose fires next and schedules the retry.
        ws?.close();
      };
    };

    void connect();

    return () => {
      stopped = true;
      if (retryTimer) window.clearTimeout(retryTimer);
      // Detach onclose before closing so the cleanup itself does not schedule a
      // retry that outlives this effect.
      if (ws) {
        ws.onclose = null;
        ws.close();
      }
    };
  }, [apiUrl, apiKey, instanceId, targetKey]);

  // No credentials to connect with: there is no socket, so the connection is
  // simply closed (and not a pending attempt).
  if (!instanceId || !apiKey || !apiUrl) return 'closed';
  // A connection record for a different target belongs to the previous socket;
  // this one is already "connecting".
  return conn.key === targetKey && conn.open ? 'open' : 'connecting';
}
