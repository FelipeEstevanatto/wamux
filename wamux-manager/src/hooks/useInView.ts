import { useEffect, useRef, useState } from 'react';

type InViewCallback = (visible: boolean) => void;

// One IntersectionObserver shared by every consumer. A chat list mounts dozens
// of avatars at once, and a single observer is far cheaper than one per node.
const callbacks = new WeakMap<Element, InViewCallback>();
let sharedObserver: IntersectionObserver | null = null;

function observer(): IntersectionObserver | null {
  if (typeof IntersectionObserver === 'undefined') return null;
  if (!sharedObserver) {
    sharedObserver = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          callbacks.get(entry.target)?.(entry.isIntersecting);
        }
      },
      // Start a little before the element reaches the viewport, so a fetch
      // triggered on first visibility has a head start before the row scrolls
      // fully into view.
      { rootMargin: '200px' }
    );
  }
  return sharedObserver;
}

/**
 * useInView reports whether the element attached to the returned ref has come
 * into (or within 200px of) the viewport.
 *
 * It exists so a chat-list avatar only fetches its picture when it is about to
 * be seen, instead of firing one request per row on load (an N+1 that grows
 * with the conversation list). With `once` (the default) it latches true on the
 * first intersection and stops observing. In an environment without
 * IntersectionObserver (an old browser, or jsdom in tests) it reports true
 * immediately, so a caller loads eagerly rather than never.
 */
export function useInView<T extends Element>(once = true) {
  // Fail open when there is no IntersectionObserver (old browser, jsdom in
  // tests): start "in view" so a caller loads eagerly rather than never. Set in
  // the initializer, not an effect, so no state is written during mount.
  const [inView, setInView] = useState(() => typeof IntersectionObserver === 'undefined');
  const ref = useRef<T | null>(null);

  useEffect(() => {
    if (once && inView) return;
    const el = ref.current;
    if (!el) return;

    const obs = observer();
    if (!obs) return; // fail-open already handled by the initial state

    const cb: InViewCallback = (visible) => {
      if (visible) setInView(true);
      else if (!once) setInView(false);
    };
    callbacks.set(el, cb);
    obs.observe(el);
    return () => {
      callbacks.delete(el);
      obs.unobserve(el);
    };
  }, [inView, once]);

  return [ref, inView] as const;
}