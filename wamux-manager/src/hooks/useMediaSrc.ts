import { useEffect, useState } from 'react';
import * as messagesApi from '@/services/api/messages';

// Resolved object URLs are cached so re-renders and re-mounts do not refetch.
// A cached value is reused across every message that points at the same media.
const mediaObjectUrlCache = new Map<string, string>();

function isAbsoluteUrl(url: string): boolean {
  return url.startsWith('http://') || url.startsWith('https://');
}

export interface MediaSrcState {
  src: string;
  loading: boolean;
  failed: boolean;
}

function initialState(mediaUrl: string, token: string): MediaSrcState {
  if (!mediaUrl) return { src: '', loading: false, failed: false };
  if (isAbsoluteUrl(mediaUrl)) return { src: mediaUrl, loading: false, failed: false };
  if (!token) return { src: '', loading: false, failed: true };

  const cached = mediaObjectUrlCache.get(`${token}|${mediaUrl}`);
  if (cached !== undefined) return { src: cached, loading: false, failed: false };
  return { src: '', loading: true, failed: false };
}

/**
 * Resolve a message's `media_url` into something an <img>/<video>/<a> can use.
 * Absolute object-store URLs are used as-is; API paths (`/chat/media/<id>`) are
 * fetched with the instance token and turned into a blob URL.
 */
export function useMediaSrc(mediaUrl: string, token: string): MediaSrcState {
  const cacheKey = `${token}|${mediaUrl}`;
  const absolute = isAbsoluteUrl(mediaUrl);
  const [state, setState] = useState<MediaSrcState>(() =>
    initialState(mediaUrl, token)
  );
  const [lastKey, setLastKey] = useState(cacheKey);

  // Re-resolve during render when the target changes, so a cached image paints
  // immediately instead of waiting for an effect round-trip.
  if (cacheKey !== lastKey) {
    setLastKey(cacheKey);
    setState(initialState(mediaUrl, token));
  }

  useEffect(() => {
    if (!mediaUrl || absolute || !token) return;
    if (mediaObjectUrlCache.has(cacheKey)) return;

    let active = true;
    messagesApi
      .fetchMediaObjectUrl(token, mediaUrl)
      .then((url) => {
        if (!active) {
          URL.revokeObjectURL(url);
          return;
        }
        mediaObjectUrlCache.set(cacheKey, url);
        setState({ src: url, loading: false, failed: false });
      })
      .catch(() => {
        if (active) setState({ src: '', loading: false, failed: true });
      });

    return () => {
      active = false;
    };
  }, [cacheKey, mediaUrl, token, absolute]);

  return state;
}
