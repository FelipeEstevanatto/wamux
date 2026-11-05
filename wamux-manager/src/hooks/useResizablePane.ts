import {
  type KeyboardEvent,
  type PointerEvent,
  type RefObject,
  useCallback,
  useRef,
  useState,
} from 'react';

export const PANE_MIN = 260;
export const PANE_MAX = 640;
export const PANE_DEFAULT = 340;
/** Room the neighbouring pane keeps, however far the divider is dragged. */
export const PANE_NEIGHBOUR_MIN = 360;
const KEY_STEP = 24;

/** Limits a pane width to the allowed range for a container of this width. */
export function clampPaneWidth(width: number, containerWidth: number): number {
  const max = Math.max(PANE_MIN, Math.min(PANE_MAX, containerWidth - PANE_NEIGHBOUR_MIN));
  return Math.round(Math.min(max, Math.max(PANE_MIN, width)));
}

function read(key: string): number {
  try {
    const saved = Number(window.localStorage.getItem(key));
    return saved >= PANE_MIN && saved <= PANE_MAX ? saved : PANE_DEFAULT;
  } catch {
    return PANE_DEFAULT;
  }
}

function save(key: string, width: number) {
  try {
    window.localStorage.setItem(key, String(width));
  } catch {
    // Private windows can refuse storage; the width just lasts this visit.
  }
}

/**
 * A pane resized by dragging a divider, or with the arrow keys when it has focus.
 * The width is remembered in this browser, and a double click resets it.
 */
export function useResizablePane(
  storageKey: string,
  containerRef: RefObject<HTMLElement | null>
) {
  const [width, setWidth] = useState(() => read(storageKey));
  const [dragging, setDragging] = useState(false);
  const widthRef = useRef(width);

  const apply = useCallback(
    (next: number) => {
      const container = containerRef.current?.clientWidth ?? Number.MAX_SAFE_INTEGER;
      const clamped = clampPaneWidth(next, container);
      widthRef.current = clamped;
      setWidth(clamped);
    },
    [containerRef]
  );

  const onPointerDown = (e: PointerEvent<HTMLElement>) => {
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    setDragging(true);
  };

  const onPointerMove = (e: PointerEvent<HTMLElement>) => {
    if (!dragging) return;
    const left = containerRef.current?.getBoundingClientRect().left ?? 0;
    apply(e.clientX - left);
  };

  const onPointerUp = () => {
    if (!dragging) return;
    setDragging(false);
    save(storageKey, widthRef.current);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
    e.preventDefault();
    apply(widthRef.current + (e.key === 'ArrowRight' ? KEY_STEP : -KEY_STEP));
    save(storageKey, widthRef.current);
  };

  const onDoubleClick = () => {
    apply(PANE_DEFAULT);
    save(storageKey, widthRef.current);
  };

  return {
    width,
    dragging,
    handleProps: { onPointerDown, onPointerMove, onPointerUp, onKeyDown, onDoubleClick },
  };
}
